package auth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"
)

type loadTestDelegate struct {
	principal Principal
	err       error
	seen      string
}

func (d *loadTestDelegate) Verify(_ context.Context, raw string) (Principal, error) {
	d.seen = raw
	return d.principal, d.err
}

func TestLoadTestVerifierAcceptsOnlyValidShortLivedTokens(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	secret := []byte("01234567890123456789012345678901")
	verifier, err := NewLoadTestVerifier(&loadTestDelegate{}, base64.StdEncoding.EncodeToString(secret))
	if err != nil {
		t.Fatal(err)
	}
	verifier.now = func() time.Time { return now }

	cases := []struct {
		name   string
		claims loadTestClaims
		valid  bool
	}{
		{name: "valid", claims: loadTestClaims{Subject: "user_synthetic", IssuedAt: now.Unix(), ExpiresAt: now.Add(5 * time.Minute).Unix()}, valid: true},
		{name: "expired", claims: loadTestClaims{Subject: "user_synthetic", IssuedAt: now.Add(-10 * time.Minute).Unix(), ExpiresAt: now.Unix()}},
		{name: "too long", claims: loadTestClaims{Subject: "user_synthetic", IssuedAt: now.Unix(), ExpiresAt: now.Add(11 * time.Minute).Unix()}},
		{name: "future", claims: loadTestClaims{Subject: "user_synthetic", IssuedAt: now.Add(time.Minute).Unix(), ExpiresAt: now.Add(2 * time.Minute).Unix()}},
		{name: "missing subject", claims: loadTestClaims{IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Minute).Unix()}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			principal, verifyErr := verifier.Verify(context.Background(), signLoadTestToken(t, secret, test.claims))
			if test.valid && (verifyErr != nil || principal.ClerkUserID != test.claims.Subject) {
				t.Fatalf("expected valid synthetic principal, got %+v, %v", principal, verifyErr)
			}
			if !test.valid && !errors.Is(verifyErr, ErrUnauthenticated) {
				t.Fatalf("expected unauthenticated, got %v", verifyErr)
			}
		})
	}
}

func TestLoadTestVerifierRejectsTamperingAndDelegatesClerkTokens(t *testing.T) {
	secret := []byte("01234567890123456789012345678901")
	delegate := &loadTestDelegate{principal: Principal{ClerkUserID: "user_clerk"}}
	verifier, err := NewLoadTestVerifier(delegate, base64.StdEncoding.EncodeToString(secret))
	if err != nil {
		t.Fatal(err)
	}
	principal, err := verifier.Verify(context.Background(), "clerk.jwt")
	if err != nil || principal.ClerkUserID != "user_clerk" || delegate.seen != "clerk.jwt" {
		t.Fatalf("Clerk token was not delegated: %+v, %v", principal, err)
	}

	token := signLoadTestToken(t, secret, loadTestClaims{Subject: "user_synthetic", IssuedAt: time.Now().Unix(), ExpiresAt: time.Now().Add(time.Minute).Unix()})
	parts := strings.Split(strings.TrimPrefix(token, loadTestTokenPrefix), ".")
	signature, decodeErr := base64.RawURLEncoding.DecodeString(parts[1])
	if decodeErr != nil {
		t.Fatal(decodeErr)
	}
	signature[0] ^= 0xff
	tampered := loadTestTokenPrefix + parts[0] + "." + base64.RawURLEncoding.EncodeToString(signature)
	if _, err := verifier.Verify(context.Background(), tampered); !errors.Is(err, ErrUnauthenticated) {
		t.Fatalf("expected tampered token rejection, got %v", err)
	}
}

func TestNewLoadTestVerifierRejectsUnsafeConfiguration(t *testing.T) {
	if _, err := NewLoadTestVerifier(nil, base64.StdEncoding.EncodeToString(make([]byte, 32))); err == nil {
		t.Fatal("expected nil delegate rejection")
	}
	if _, err := NewLoadTestVerifier(&loadTestDelegate{}, base64.StdEncoding.EncodeToString(make([]byte, 31))); err == nil {
		t.Fatal("expected short secret rejection")
	}
}

func signLoadTestToken(t *testing.T, secret []byte, claims loadTestClaims) string {
	t.Helper()
	payload, err := json.Marshal(claims)
	if err != nil {
		t.Fatal(err)
	}
	mac := hmac.New(sha256.New, secret)
	_, _ = mac.Write(payload)
	return loadTestTokenPrefix + base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
