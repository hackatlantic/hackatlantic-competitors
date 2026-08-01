package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

func TestJWTVerifierRejectsInvalidClaims(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	verifier := testVerifier(t, key)
	now := time.Now()

	cases := []struct {
		name   string
		claims sessionClaims
		valid  bool
	}{
		{"valid", sessionClaims{RegisteredClaims: jwt.RegisteredClaims{Subject: "user_123", Issuer: "https://issuer.example", ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)), NotBefore: jwt.NewNumericDate(now.Add(-time.Minute))}, AuthorizedParty: "https://app.example"}, true},
		{"missing expiration", sessionClaims{RegisteredClaims: jwt.RegisteredClaims{Subject: "user_123", Issuer: "https://issuer.example", NotBefore: jwt.NewNumericDate(now.Add(-time.Minute))}, AuthorizedParty: "https://app.example"}, false},
		{"expired", sessionClaims{RegisteredClaims: jwt.RegisteredClaims{Subject: "user_123", Issuer: "https://issuer.example", ExpiresAt: jwt.NewNumericDate(now.Add(-time.Minute)), NotBefore: jwt.NewNumericDate(now.Add(-time.Hour))}, AuthorizedParty: "https://app.example"}, false},
		{"not before", sessionClaims{RegisteredClaims: jwt.RegisteredClaims{Subject: "user_123", Issuer: "https://issuer.example", ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)), NotBefore: jwt.NewNumericDate(now.Add(time.Hour))}, AuthorizedParty: "https://app.example"}, false},
		{"wrong issuer", sessionClaims{RegisteredClaims: jwt.RegisteredClaims{Subject: "user_123", Issuer: "https://other.example", ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)), NotBefore: jwt.NewNumericDate(now.Add(-time.Minute))}, AuthorizedParty: "https://app.example"}, false},
		{"wrong authorized party", sessionClaims{RegisteredClaims: jwt.RegisteredClaims{Subject: "user_123", Issuer: "https://issuer.example", ExpiresAt: jwt.NewNumericDate(now.Add(time.Hour)), NotBefore: jwt.NewNumericDate(now.Add(-time.Minute))}, AuthorizedParty: "https://other.example"}, false},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			token := jwt.NewWithClaims(jwt.SigningMethodRS256, test.claims)
			token.Header["kid"] = "test-key"
			raw, err := token.SignedString(key)
			if err != nil {
				t.Fatal(err)
			}
			_, err = verifier.Verify(context.Background(), raw)
			if test.valid && err != nil {
				t.Fatalf("expected valid token, got %v", err)
			}
			if !test.valid && err == nil {
				t.Fatal("expected token rejection")
			}
		})
	}
}

func TestJWTVerifierJWKSOutlivesInitializationContext(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write(testJWKS(t, key))
	}))
	defer server.Close()

	lifecycleCtx, stop := context.WithCancel(context.Background())
	defer stop()
	initialCtx, cancelInitial := context.WithTimeout(context.Background(), time.Second)
	verifier, err := NewJWTVerifier(lifecycleCtx, initialCtx, ClerkConfig{
		Issuer:            "https://issuer.example",
		AuthorizedParties: []string{"https://app.example"},
		JWKSURL:           server.URL,
	})
	if err != nil {
		t.Fatalf("create JWKS verifier: %v", err)
	}
	cancelInitial()

	claims := sessionClaims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   "user_123",
			Issuer:    "https://issuer.example",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
			NotBefore: jwt.NewNumericDate(time.Now().Add(-time.Minute)),
		},
		AuthorizedParty: "https://app.example",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-key"
	raw, err := token.SignedString(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := verifier.Verify(context.Background(), raw); err != nil {
		t.Fatalf("verify after initialization context cancellation: %v", err)
	}
}

func TestJWTVerifierBoundsInitialJWKSRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, request *http.Request) {
		<-request.Context().Done()
	}))
	defer server.Close()

	lifecycleCtx, stop := context.WithCancel(context.Background())
	defer stop()
	initialCtx, cancelInitial := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancelInitial()
	started := time.Now()
	_, err := NewJWTVerifier(lifecycleCtx, initialCtx, ClerkConfig{
		Issuer:            "https://issuer.example",
		AuthorizedParties: []string{"https://app.example"},
		JWKSURL:           server.URL,
	})
	if err == nil {
		t.Fatal("expected slow initial JWKS request to fail")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("initial JWKS request exceeded its context deadline: %s", elapsed)
	}
}

func TestBearerTokenRejectsMalformedCredentials(t *testing.T) {
	for _, header := range []string{"", "Basic token", "Bearer ", "bearer token"} {
		request := httptest.NewRequest("GET", "/v1/me", nil)
		if header != "" {
			request.Header.Set("Authorization", header)
		}
		if _, err := BearerToken(request); err == nil {
			t.Fatalf("expected malformed header %q to be rejected", header)
		}
	}
}

func testVerifier(t *testing.T, key *rsa.PrivateKey) *JWTVerifier {
	t.Helper()
	keys, err := keyfunc.NewJWKSetJSON(testJWKS(t, key))
	if err != nil {
		t.Fatal(err)
	}
	return &JWTVerifier{issuer: "https://issuer.example", authorizedParties: map[string]struct{}{"https://app.example": {}}, keyfunc: func(ctx context.Context) jwt.Keyfunc { return keys.KeyfuncCtx(ctx) }}
}

func testJWKS(t *testing.T, key *rsa.PrivateKey) []byte {
	t.Helper()
	public := key.PublicKey
	encoded := func(value []byte) string { return base64.RawURLEncoding.EncodeToString(value) }
	exponent := big.NewInt(int64(public.E)).Bytes()
	raw, err := json.Marshal(map[string]any{"keys": []map[string]string{{"kty": "RSA", "kid": "test-key", "use": "sig", "alg": "RS256", "n": encoded(public.N.Bytes()), "e": encoded(exponent)}}})
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
