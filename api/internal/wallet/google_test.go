package wallet

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/passes"
)

func testConfig(t *testing.T) map[string]string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	credentials, _ := json.Marshal(map[string]string{"type": "service_account", "client_email": "wallet@hackatlantic-test.iam.gserviceaccount.com", "private_key_id": "test-key", "private_key": string(pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}))})
	return map[string]string{"GOOGLE_WALLET_ENABLED": "true", "GOOGLE_WALLET_ISSUER_ID": "123456789", "GOOGLE_WALLET_CLASS_ID": "123456789.hackatlantic_2026", "GOOGLE_WALLET_CYCLE_SLUG": "hackatlantic-2026", "APP_BASE_URL": "https://apply.hackatlantic.ca", "GOOGLE_WALLET_SERVICE_ACCOUNT_JSON": string(credentials)}
}

func TestGoogleConfiguration(t *testing.T) {
	config := testConfig(t)
	for _, test := range []struct{ key, value string }{
		{"GOOGLE_WALLET_ENABLED", "yes"}, {"GOOGLE_WALLET_ISSUER_ID", "wrong"},
		{"GOOGLE_WALLET_CLASS_ID", "999.other"}, {"GOOGLE_WALLET_CYCLE_SLUG", ""},
		{"APP_BASE_URL", "http://apply.hackatlantic.ca"}, {"APP_BASE_URL", "https://user:password@example.com"},
		{"APP_BASE_URL", "https://apply.hackatlantic.ca/path"}, {"GOOGLE_WALLET_SERVICE_ACCOUNT_JSON", "SECRET-invalid-key"},
	} {
		t.Run(test.key+test.value, func(t *testing.T) {
			getenv := func(k string) string {
				if k == test.key {
					return test.value
				}
				return config[k]
			}
			if _, err := LoadGoogle(getenv); err == nil || strings.Contains(err.Error(), "SECRET") {
				t.Fatal("invalid configuration accepted or secret leaked")
			}
		})
	}
	for _, flag := range []string{"", "false"} {
		g, err := LoadGoogle(func(k string) string {
			if k == "GOOGLE_WALLET_ENABLED" {
				return flag
			}
			return "bad"
		})
		if g != nil || err != nil {
			t.Fatal("disabled wallet must ignore incomplete setup")
		}
	}
}

func TestGoogleSaveURLSignedMinimalAndStable(t *testing.T) {
	config := testConfig(t)
	g, err := LoadGoogle(func(k string) string { return config[k] })
	if err != nil {
		t.Fatal(err)
	}
	g.now = func() time.Time { return time.Unix(1800000000, 0) }
	pass := passes.WebPass{Pass: passes.Pass{ID: "12345678-1234-1234-1234-123456789012", DisplayName: "Test Attendee", Status: "active"}, QRToken: "qr_v1." + strings.Repeat("a", 43)}
	link, err := g.SaveURL(pass)
	if err != nil {
		t.Fatal(err)
	}
	again, _ := g.SaveURL(pass)
	if link != again || !strings.HasPrefix(link, saveURLPrefix) || len(strings.TrimPrefix(link, saveURLPrefix)) > 1800 {
		t.Fatal("unstable or oversized link")
	}
	parsed, err := jwt.Parse(strings.TrimPrefix(link, saveURLPrefix), func(token *jwt.Token) (any, error) { return &g.key.PublicKey, nil }, jwt.WithValidMethods([]string{"RS256"}), jwt.WithAudience("google"), jwt.WithIssuer(g.email), jwt.WithTimeFunc(g.now))
	if err != nil || !parsed.Valid {
		t.Fatal("signature or claims verification failed")
	}
	claims := parsed.Claims.(jwt.MapClaims)
	if claims["typ"] != "savetowallet" || parsed.Header["kid"] != "test-key" || claims["exp"].(float64)-claims["iat"].(float64) != 600 {
		t.Fatal("incorrect Wallet claims")
	}
	object := claims["payload"].(map[string]any)["eventTicketObjects"].([]any)[0].(map[string]any)
	if object["id"] != "123456789.pass_12345678123412341234123456789012" || object["classId"] != config["GOOGLE_WALLET_CLASS_ID"] || object["barcode"].(map[string]any)["value"] != pass.QRToken || len(object) != 5 {
		t.Fatal("unexpected pass payload")
	}
	pass.DisplayName = strings.Repeat("Long name", 500)
	if _, err := g.SaveURL(pass); err != nil {
		t.Fatal("optional long name should not break save")
	}
	pass.Status = "revoked"
	if _, err := g.SaveURL(pass); err == nil {
		t.Fatal("signed revoked pass")
	}
	pass.Status = "active"
	pass.QRToken = "claim_v1." + strings.Repeat("a", 43)
	if _, err := g.SaveURL(pass); err == nil {
		t.Fatal("signed claim credential instead of QR")
	}
	pass.QRToken = "qr_v1." + strings.Repeat("a", 43)
	pass.ID = "invalid"
	if _, err := g.SaveURL(pass); err == nil {
		t.Fatal("accepted invalid pass ID")
	}
}
