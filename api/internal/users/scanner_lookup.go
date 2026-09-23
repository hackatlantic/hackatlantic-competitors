package users

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/database/sqlc"
)

var (
	ErrInvalidEmail       = errors.New("invalid email")
	ErrAmbiguousEmail     = errors.New("multiple accounts match this email")
	ErrProfileUnavailable = errors.New("verified profile unavailable")
)

// ScannerAccessUser deliberately excludes applications, Clerk IDs, and credentials.
type ScannerAccessUser struct {
	ID            string  `json:"id"`
	Email         string  `json:"email"`
	DisplayName   *string `json:"displayName"`
	ScannerAccess bool    `json:"scannerAccess"`
	CanManage     bool    `json:"canManage"`
}

func normalizeLookupEmail(value string) (string, error) {
	email := strings.ToLower(strings.TrimSpace(value))
	address, err := mail.ParseAddress(email)
	if err != nil || address.Address != email || len(email) > 254 {
		return "", ErrInvalidEmail
	}
	return email, nil
}

// LookupScannerUser resolves an exact email only, for admins. It never creates
// an account or assigns a role, and refuses ambiguous or stale email matches.
func (s *Service) LookupScannerUser(ctx context.Context, actor User, value string) (ScannerAccessUser, error) {
	if !actor.HasRole(RoleAdmin) {
		return ScannerAccessUser{}, ErrForbidden
	}
	email, err := normalizeLookupEmail(value)
	if err != nil {
		return ScannerAccessUser{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, s.queryTimeout)
	defer cancel()
	matches, err := sqlc.New(s.pool).LookupScannerUserByEmail(ctx, email)
	if err != nil {
		return ScannerAccessUser{}, fmt.Errorf("lookup scanner user: %w", err)
	}
	if len(matches) == 0 {
		return ScannerAccessUser{}, ErrNotFound
	}
	if len(matches) != 1 {
		return ScannerAccessUser{}, ErrAmbiguousEmail
	}
	match := matches[0]
	if s.profiles == nil {
		return ScannerAccessUser{}, ErrProfileUnavailable
	}
	profile, err := s.profiles.Profile(ctx, match.ClerkUserID)
	if err != nil {
		return ScannerAccessUser{}, ErrProfileUnavailable
	}
	if profile.ClerkUserID != match.ClerkUserID || strings.ToLower(strings.TrimSpace(profile.Email)) != email {
		return ScannerAccessUser{}, ErrNotFound
	}
	return ScannerAccessUser{
		ID: match.ID.String(), Email: profile.Email, DisplayName: profile.DisplayName,
		ScannerAccess: match.ScannerAccess || match.IsAdmin,
		CanManage:     !match.IsAdmin && match.ID.String() != actor.ID,
	}, nil
}
