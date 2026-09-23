package wallet

import (
	"crypto/rsa"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/passes"
)

const saveURLPrefix = "https://pay.google.com/gp/v/save/"

var (
	issuerPattern = regexp.MustCompile(`^[0-9]{1,30}$`)
	suffixPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,80}$`)
	passIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
)

// Google signs save links locally. It does not send attendee data to Google:
// only the attendee's explicit save action does that. Never log returned URLs.
type Google struct {
	issuerID, classID, cycleSlug, origin, email, keyID string
	key                                                *rsa.PrivateKey
	now                                                func() time.Time
}

// LoadGoogle is opt-in so incomplete onboarding cannot break existing passes.
// Enabled configurations fail closed without including secret values in errors.
func LoadGoogle(getenv func(string) string) (*Google, error) {
	enabled := strings.TrimSpace(getenv("GOOGLE_WALLET_ENABLED"))
	if enabled == "" || enabled == "false" {
		return nil, nil
	}
	if enabled != "true" {
		return nil, errors.New("GOOGLE_WALLET_ENABLED must be true or false")
	}
	issuer := strings.TrimSpace(getenv("GOOGLE_WALLET_ISSUER_ID"))
	class := strings.TrimSpace(getenv("GOOGLE_WALLET_CLASS_ID"))
	cycle := strings.TrimSpace(getenv("GOOGLE_WALLET_CYCLE_SLUG"))
	if !issuerPattern.MatchString(issuer) || !strings.HasPrefix(class, issuer+".") || !suffixPattern.MatchString(strings.TrimPrefix(class, issuer+".")) || !suffixPattern.MatchString(cycle) {
		return nil, errors.New("Google Wallet requires a numeric issuer ID, matching issuer.class ID, and cycle slug")
	}
	origin, err := url.Parse(strings.TrimSpace(getenv("APP_BASE_URL")))
	if err != nil || origin.Scheme != "https" || origin.Host == "" || origin.User != nil || origin.RawQuery != "" || origin.Fragment != "" || (origin.Path != "" && origin.Path != "/") {
		return nil, errors.New("Google Wallet requires APP_BASE_URL to be an HTTPS origin")
	}
	var credentials struct {
		Type         string `json:"type"`
		ClientEmail  string `json:"client_email"`
		PrivateKey   string `json:"private_key"`
		PrivateKeyID string `json:"private_key_id"`
	}
	if json.Unmarshal([]byte(getenv("GOOGLE_WALLET_SERVICE_ACCOUNT_JSON")), &credentials) != nil || credentials.Type != "service_account" || !strings.HasSuffix(credentials.ClientEmail, ".iam.gserviceaccount.com") || strings.ContainsAny(credentials.ClientEmail, " \r\n") || len(credentials.ClientEmail) > 160 || credentials.PrivateKeyID == "" || len(credentials.PrivateKeyID) > 100 {
		return nil, errors.New("Google Wallet requires valid service-account JSON credentials")
	}
	key, err := jwt.ParseRSAPrivateKeyFromPEM([]byte(credentials.PrivateKey))
	if err != nil || key.N.BitLen() < 2048 || key.Validate() != nil {
		return nil, errors.New("Google Wallet requires a valid RSA private key of at least 2048 bits")
	}
	return &Google{issuerID: issuer, classID: class, cycleSlug: cycle, origin: origin.Scheme + "://" + origin.Host,
		email: credentials.ClientEmail, keyID: credentials.PrivateKeyID, key: key, now: time.Now}, nil
}

func (g *Google) CycleSlug() string { return g.cycleSlug }

// SaveURL uses an existing, approved Event Ticket Class. Stable object IDs make
// repeated saves refer to the same issued pass; reissues use the new pass ID.
func (g *Google) SaveURL(pass passes.WebPass) (string, error) {
	if pass.Status != "active" || !passIDPattern.MatchString(pass.ID) || !strings.HasPrefix(pass.QRToken, "qr_v1.") || len(pass.QRToken) != 49 {
		return "", errors.New("Google Wallet requires an active pass and its QR credential")
	}
	name := strings.TrimSpace(pass.DisplayName)
	if name == "" {
		name = "Hack Atlantic attendee"
	}
	object := map[string]any{
		"id":      g.issuerID + ".pass_" + strings.ReplaceAll(pass.ID, "-", ""),
		"classId": g.classID, "state": "ACTIVE", "ticketHolderName": name,
		"barcode": map[string]string{"type": "QR_CODE", "value": pass.QRToken},
	}
	now := g.now()
	claims := jwt.MapClaims{"iss": g.email, "aud": "google", "typ": "savetowallet", "iat": now.Unix(), "exp": now.Add(10 * time.Minute).Unix(),
		"origins": []string{g.origin}, "payload": map[string]any{"eventTicketObjects": []any{object}}}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = g.keyID
	signed, err := token.SignedString(g.key)
	if err != nil {
		return "", errors.New("could not sign Google Wallet pass")
	}
	// Google recommends a maximum 1800-character JWT for reliable save links.
	// Omit the optional name for unusually long names rather than truncate it.
	if len(signed) > 1800 {
		delete(object, "ticketHolderName")
		signed, err = token.SignedString(g.key)
	}
	if err != nil || len(signed) > 1800 {
		return "", errors.New("Google Wallet save link exceeds supported size")
	}
	return saveURLPrefix + signed, nil
}
