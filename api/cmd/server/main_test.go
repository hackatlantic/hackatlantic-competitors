package main

import (
	"encoding/base64"
	"testing"
)

func TestLoadClerkSettings(t *testing.T) {
	cases := []struct {
		name    string
		values  map[string]string
		enabled bool
		wantErr bool
	}{
		{name: "empty settings reject protected routes", values: map[string]string{}, enabled: false},
		{name: "secret only", values: map[string]string{"CLERK_SECRET_KEY": "secret"}, wantErr: true},
		{name: "issuer only", values: map[string]string{"CLERK_ISSUER_URL": "https://issuer.example"}, wantErr: true},
		{name: "authorized party only", values: map[string]string{"CLERK_AUTHORIZED_PARTIES": "https://app.example"}, wantErr: true},
		{name: "JWKS override only", values: map[string]string{"CLERK_JWKS_URL": "https://issuer.example/jwks"}, wantErr: true},
		{name: "secret and issuer only", values: map[string]string{"CLERK_SECRET_KEY": "secret", "CLERK_ISSUER_URL": "https://issuer.example"}, wantErr: true},
		{name: "complete settings", values: map[string]string{"CLERK_SECRET_KEY": "secret", "CLERK_ISSUER_URL": "https://issuer.example", "CLERK_AUTHORIZED_PARTIES": "https://app.example", "CLERK_JWKS_URL": "https://issuer.example/jwks"}, enabled: true},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			settings, enabled, err := loadClerkSettings(func(name string) string { return test.values[name] })
			if test.wantErr {
				if err == nil {
					t.Fatal("expected configuration error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected configuration error: %v", err)
			}
			if enabled != test.enabled {
				t.Fatalf("expected enabled=%t, got %t", test.enabled, enabled)
			}
			if enabled && (settings.SecretKey == "" || settings.Issuer == "" || len(settings.AuthorizedParties) != 1) {
				t.Fatalf("unexpected complete settings: %+v", settings)
			}
		})
	}
}

func TestLoadTestAuthenticationEnvironmentGate(t *testing.T) {
	secret := base64.StdEncoding.EncodeToString(make([]byte, 32))
	for _, test := range []struct {
		name        string
		environment string
		secret      string
		wantErr     bool
	}{
		{name: "disabled in production", environment: "production"},
		{name: "enabled in staging", environment: "staging", secret: secret},
		{name: "rejected in production", environment: "production", secret: secret, wantErr: true},
		{name: "rejected in development", environment: "development", secret: secret, wantErr: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			err := validateLoadTestAuthentication(test.environment, test.secret)
			if test.wantErr && err == nil {
				t.Fatal("expected environment gate error")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("unexpected environment gate error: %v", err)
			}
		})
	}
}

func TestLoadPassSettings(t *testing.T) {
	cases := []struct {
		name    string
		values  map[string]string
		wantErr bool
	}{
		{name: "missing peppers", values: map[string]string{}, wantErr: true},
		{name: "missing QR pepper", values: map[string]string{"CLAIM_TOKEN_PEPPER": "claim-secret"}, wantErr: true},
		{name: "missing claim pepper", values: map[string]string{"QR_TOKEN_PEPPER": "qr-secret"}, wantErr: true},
		{name: "same peppers", values: map[string]string{"QR_TOKEN_PEPPER": "same-secret", "CLAIM_TOKEN_PEPPER": "same-secret"}, wantErr: true},
		{name: "trimmed distinct peppers", values: map[string]string{"QR_TOKEN_PEPPER": " qr-secret ", "CLAIM_TOKEN_PEPPER": " claim-secret ", "APP_BASE_URL": "http://app.example"}},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			settings, err := loadPassSettings(func(name string) string { return test.values[name] })
			if test.wantErr {
				if err == nil {
					t.Fatal("expected configuration error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected configuration error: %v", err)
			}
			if settings.QRTokenPepper != "qr-secret" || settings.ClaimTokenPepper != "claim-secret" {
				t.Fatal("credential settings were not trimmed or preserved correctly")
			}
		})
	}
}
