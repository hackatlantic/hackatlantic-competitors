package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const loadTestTokenPrefix = "hat_load_v1."

type loadTestClaims struct {
	Subject   string `json:"sub"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// LoadTestVerifier accepts short-lived synthetic identities in staging while
// delegating every normal bearer token to Clerk's verifier.
type LoadTestVerifier struct {
	delegate Verifier
	secret   []byte
	now      func() time.Time
}

// NewLoadTestVerifier creates a verifier from a standard-base64 secret with at
// least 32 random bytes. Callers must gate this verifier to staging.
func NewLoadTestVerifier(delegate Verifier, encodedSecret string) (*LoadTestVerifier, error) {
	if delegate == nil {
		return nil, fmt.Errorf("load-test verifier requires a Clerk delegate")
	}
	secret, err := base64.StdEncoding.DecodeString(strings.TrimSpace(encodedSecret))
	if err != nil || len(secret) < 32 {
		return nil, fmt.Errorf("LOAD_TEST_AUTH_SECRET must be standard base64 containing at least 32 bytes")
	}
	return &LoadTestVerifier{delegate: delegate, secret: secret, now: time.Now}, nil
}

func (v *LoadTestVerifier) Verify(ctx context.Context, raw string) (Principal, error) {
	if !strings.HasPrefix(raw, loadTestTokenPrefix) {
		return v.delegate.Verify(ctx, raw)
	}
	parts := strings.Split(strings.TrimPrefix(raw, loadTestTokenPrefix), ".")
	if len(parts) != 2 {
		return Principal{}, ErrUnauthenticated
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Principal{}, ErrUnauthenticated
	}
	mac := hmac.New(sha256.New, v.secret)
	_, _ = mac.Write(payload)
	if !hmac.Equal(signature, mac.Sum(nil)) {
		return Principal{}, ErrUnauthenticated
	}
	var claims loadTestClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return Principal{}, ErrUnauthenticated
	}
	now := v.now().Unix()
	if claims.Subject == "" || claims.IssuedAt > now+30 || claims.ExpiresAt <= now ||
		claims.ExpiresAt <= claims.IssuedAt || claims.ExpiresAt-claims.IssuedAt > int64(10*time.Minute/time.Second) {
		return Principal{}, ErrUnauthenticated
	}
	return Principal{ClerkUserID: claims.Subject}, nil
}

var _ Verifier = (*LoadTestVerifier)(nil)
