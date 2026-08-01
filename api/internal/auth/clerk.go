// Package auth verifies Clerk identities and carries verified principals.
package auth

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

var ErrUnauthenticated = errors.New("unauthenticated")

// Principal is an identity verified by Clerk. It intentionally excludes roles.
type Principal struct {
	ClerkUserID string
}

// Verifier validates a bearer token and returns only verified identity data.
type Verifier interface {
	Verify(context.Context, string) (Principal, error)
}

// ClerkConfig configures session-token validation.
type ClerkConfig struct {
	Issuer            string
	AuthorizedParties []string
	JWKSURL           string
}

const (
	jwksRequestTimeout  = 5 * time.Second
	jwksRefreshInterval = time.Hour
	maxJWKSResponseSize = 1 << 20
)

// JWTVerifier validates Clerk session tokens with a cached JWKS key source.
type JWTVerifier struct {
	issuer            string
	authorizedParties map[string]struct{}
	keyfunc           func(context.Context) jwt.Keyfunc
}

type sessionClaims struct {
	jwt.RegisteredClaims
	AuthorizedParty string `json:"azp"`
}

// NewJWTVerifier constructs a verifier. lifecycleCtx owns periodic key refresh.
// initialCtx bounds the first network request before the verifier is exposed.
func NewJWTVerifier(lifecycleCtx context.Context, initialCtx context.Context, config ClerkConfig) (*JWTVerifier, error) {
	if config.Issuer == "" {
		return nil, fmt.Errorf("CLERK_ISSUER_URL is required")
	}
	if len(config.AuthorizedParties) == 0 {
		return nil, fmt.Errorf("at least one CLERK_AUTHORIZED_PARTY is required")
	}
	jwksURL := config.JWKSURL
	if jwksURL == "" {
		issuerURL, err := url.Parse(config.Issuer)
		if err != nil || issuerURL.Scheme == "" || issuerURL.Host == "" {
			return nil, fmt.Errorf("parse CLERK_ISSUER_URL: %w", err)
		}
		jwksURL = strings.TrimRight(issuerURL.String(), "/") + "/.well-known/jwks.json"
	}
	cache, err := newJWKSCache(lifecycleCtx, initialCtx, jwksURL)
	if err != nil {
		return nil, err
	}
	parties := make(map[string]struct{}, len(config.AuthorizedParties))
	for _, party := range config.AuthorizedParties {
		party = strings.TrimSpace(party)
		if party != "" {
			parties[party] = struct{}{}
		}
	}
	if len(parties) == 0 {
		return nil, fmt.Errorf("at least one CLERK_AUTHORIZED_PARTY is required")
	}
	return &JWTVerifier{issuer: config.Issuer, authorizedParties: parties, keyfunc: cache.keyfunc}, nil
}

// Verify validates signature, issuer, expiration, not-before, and azp.
func (v *JWTVerifier) Verify(ctx context.Context, raw string) (Principal, error) {
	if raw == "" {
		return Principal{}, ErrUnauthenticated
	}
	claims := new(sessionClaims)
	token, err := jwt.ParseWithClaims(raw, claims, v.keyfunc(ctx), jwt.WithIssuer(v.issuer), jwt.WithExpirationRequired(), jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()}))
	if err != nil || !token.Valid || claims.Subject == "" {
		return Principal{}, ErrUnauthenticated
	}
	if _, ok := v.authorizedParties[claims.AuthorizedParty]; !ok {
		return Principal{}, ErrUnauthenticated
	}
	return Principal{ClerkUserID: claims.Subject}, nil
}

type jwksCache struct {
	url    string
	client *http.Client

	mu   sync.RWMutex
	keys keyfunc.Keyfunc
}

func newJWKSCache(lifecycleCtx context.Context, initialCtx context.Context, jwksURL string) (*jwksCache, error) {
	cache := &jwksCache{url: jwksURL, client: &http.Client{}}
	if err := cache.refresh(initialCtx); err != nil {
		return nil, fmt.Errorf("load initial Clerk JWKS: %w", err)
	}
	go cache.refreshPeriodically(lifecycleCtx)
	return cache, nil
}

func (cache *jwksCache) refreshPeriodically(ctx context.Context) {
	ticker := time.NewTicker(jwksRefreshInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			refreshCtx, cancel := context.WithTimeout(ctx, jwksRequestTimeout)
			_ = cache.refresh(refreshCtx)
			cancel()
		}
	}
}

func (cache *jwksCache) keyfunc(ctx context.Context) jwt.Keyfunc {
	return func(token *jwt.Token) (any, error) {
		keys := cache.snapshot()
		key, err := keys.KeyfuncCtx(ctx)(token)
		if err == nil {
			return key, nil
		}
		refreshCtx, cancel := context.WithTimeout(ctx, jwksRequestTimeout)
		defer cancel()
		if refreshErr := cache.refresh(refreshCtx); refreshErr != nil {
			return nil, err
		}
		return cache.snapshot().KeyfuncCtx(ctx)(token)
	}
}

func (cache *jwksCache) refresh(ctx context.Context) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, cache.url, nil)
	if err != nil {
		return fmt.Errorf("build JWKS request: %w", err)
	}
	response, err := cache.client.Do(request)
	if err != nil {
		return fmt.Errorf("fetch JWKS: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("fetch JWKS: unexpected status %d", response.StatusCode)
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, maxJWKSResponseSize+1))
	if err != nil {
		return fmt.Errorf("read JWKS: %w", err)
	}
	if len(raw) > maxJWKSResponseSize {
		return fmt.Errorf("JWKS response exceeds %d bytes", maxJWKSResponseSize)
	}
	keys, err := keyfunc.NewJWKSetJSON(raw)
	if err != nil {
		return fmt.Errorf("parse JWKS: %w", err)
	}
	keySet, err := keys.VerificationKeySet(ctx)
	if err != nil {
		return fmt.Errorf("validate JWKS: %w", err)
	}
	if len(keySet.Keys) == 0 {
		return fmt.Errorf("JWKS contains no verification keys")
	}
	cache.mu.Lock()
	cache.keys = keys
	cache.mu.Unlock()
	return nil
}

func (cache *jwksCache) snapshot() keyfunc.Keyfunc {
	cache.mu.RLock()
	defer cache.mu.RUnlock()
	return cache.keys
}

// RejectingVerifier safely denies protected routes until Clerk is configured.
type RejectingVerifier struct{}

func (RejectingVerifier) Verify(context.Context, string) (Principal, error) {
	return Principal{}, ErrUnauthenticated
}
