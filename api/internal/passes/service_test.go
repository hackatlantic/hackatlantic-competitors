package passes

import (
	"bytes"
	"encoding/base64"
	"github.com/jackc/pgx/v5/pgtype"
	"strings"
	"testing"
)

var (
	testQRPepperBytes    = []byte("0123456789abcdef0123456789abcdef")
	testClaimPepperBytes = []byte("fedcba9876543210fedcba9876543210")
	testQRTokenPepper    = base64.StdEncoding.EncodeToString(testQRPepperBytes)
	testClaimTokenPepper = base64.StdEncoding.EncodeToString(testClaimPepperBytes)
)

func TestCredentialGenerationAndPurposeHashes(t *testing.T) {
	qr, err := generateCredential(qrCredentialPrefix)
	if err != nil {
		t.Fatalf("generate QR credential: %v", err)
	}
	claim, err := generateCredential(claimCredentialPrefix)
	if err != nil {
		t.Fatalf("generate claim credential: %v", err)
	}
	if !validCredential(qr, qrCredentialPrefix) || !validCredential(claim, claimCredentialPrefix) {
		t.Fatal("generated credential was not valid")
	}
	if validCredential(qr, claimCredentialPrefix) || validCredential(claim, qrCredentialPrefix) {
		t.Fatal("credential prefixes were interchangeable")
	}
	qrHash := credentialHash(testQRPepperBytes, "qr", qr)
	claimHash := credentialHash(testClaimPepperBytes, "claim", claim)
	if len(qrHash) != 32 || len(claimHash) != 32 {
		t.Fatalf("expected 256-bit HMACs, got QR=%d claim=%d", len(qrHash), len(claimHash))
	}
	if bytes.Equal(qrHash, claimHash) {
		t.Fatal("purpose-separated credential hashes matched")
	}
	if bytes.Equal(qrHash, credentialHash(testClaimPepperBytes, "claim", qr)) {
		t.Fatal("QR token hashed as a claim token")
	}
}

func TestDerivedQRCredentialIsStablePerPassAndPepper(t *testing.T) {
	service, err := NewService(nil, 0, 0, Config{QRTokenPepper: testQRTokenPepper, ClaimTokenPepper: testClaimTokenPepper})
	if err != nil {
		t.Fatalf("create pass service: %v", err)
	}
	passID := pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	otherPassID := pgtype.UUID{Bytes: [16]byte{2}, Valid: true}

	first := service.derivedQRCredential(passID)
	if !validCredential(first, qrCredentialPrefix) {
		t.Fatal("derived QR credential was not valid")
	}
	if first != service.derivedQRCredential(passID) {
		t.Fatal("derived QR credential was not stable for the same pass")
	}
	if first == service.derivedQRCredential(otherPassID) {
		t.Fatal("derived QR credential did not bind to the pass ID")
	}
	issued, err := service.newCredentials(passID)
	if err != nil {
		t.Fatalf("create credentials: %v", err)
	}
	if issued.qr != first || bytes.Equal(issued.qrHash, issued.claimHash) {
		t.Fatal("issued credentials did not retain QR derivation and purpose separation")
	}
}

func TestCredentialConfigurationNeverEchoesPepper(t *testing.T) {
	secret := testQRTokenPepper
	_, err := NewService(nil, 0, 0, Config{QRTokenPepper: secret, ClaimTokenPepper: secret})
	if err == nil {
		t.Fatal("expected identical peppers to be rejected")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatal("configuration error leaked a pepper")
	}
}

func TestCredentialConfigurationRejectsWeakOrMalformedPeppers(t *testing.T) {
	for _, pepper := range []string{"not-base64", base64.StdEncoding.EncodeToString([]byte("too-short"))} {
		_, err := NewService(nil, 0, 0, Config{QRTokenPepper: pepper, ClaimTokenPepper: testClaimTokenPepper})
		if err == nil {
			t.Fatalf("expected pepper %q to be rejected", pepper)
		}
	}
}

func TestClaimBaseURLRejectsNonPublicComponents(t *testing.T) {
	for _, rawURL := range []string{
		"https://user:password@app.example",
		"https://app.example/?access_token=secret",
		"https://app.example/#secret",
	} {
		_, err := NewService(nil, 0, 0, Config{
			QRTokenPepper: testQRTokenPepper, ClaimTokenPepper: testClaimTokenPepper, AppBaseURL: rawURL,
		})
		if err == nil {
			t.Fatal("expected non-public base URL to be rejected")
		}
		if strings.Contains(err.Error(), "password") || strings.Contains(err.Error(), "secret") {
			t.Fatal("base URL validation echoed configured URL material")
		}
	}
}

func TestQRTokenHashUsesStoredQRDomainAndRejectsClaims(t *testing.T) {
	service, err := NewService(nil, 0, 0, Config{QRTokenPepper: testQRTokenPepper, ClaimTokenPepper: testClaimTokenPepper})
	if err != nil {
		t.Fatalf("create pass service: %v", err)
	}
	qr := service.derivedQRCredential(pgtype.UUID{Bytes: [16]byte{7}, Valid: true})
	hash, valid := service.QRTokenHash(qr)
	if !valid || !bytes.Equal(hash, credentialHash(testQRPepperBytes, "qr", qr)) {
		t.Fatal("QR resolver did not use the persisted M5 QR hash domain")
	}
	claim, err := generateCredential(claimCredentialPrefix)
	if err != nil {
		t.Fatalf("generate claim credential: %v", err)
	}
	claimHash, valid := service.QRTokenHash(claim)
	if valid || len(claimHash) != credentialEntropyBytes || !bytes.Equal(claimHash, make([]byte, credentialEntropyBytes)) {
		t.Fatal("claim credential was accepted or retained as a scanner QR hash")
	}
}
