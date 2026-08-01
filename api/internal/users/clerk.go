package users

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const clerkUsersURL = "https://api.clerk.com/v1/users/"

// Profile is trusted identity data sourced from Clerk after JWT verification.
type Profile struct {
	ClerkUserID string
	Email       string
	DisplayName *string
}

// ProfileSource resolves verified profile data for a verified Clerk subject.
type ProfileSource interface {
	Profile(context.Context, string) (Profile, error)
}

// ClerkProfileSource loads profile data through Clerk's Backend API.
type ClerkProfileSource struct {
	secretKey string
	client    *http.Client
}

// NewClerkProfileSource creates a Clerk Backend API client.
func NewClerkProfileSource(secretKey string) (*ClerkProfileSource, error) {
	if secretKey == "" {
		return nil, fmt.Errorf("CLERK_SECRET_KEY is required")
	}
	return &ClerkProfileSource{secretKey: secretKey, client: &http.Client{Timeout: 5 * time.Second}}, nil
}

type clerkUserResponse struct {
	ID                    string  `json:"id"`
	FirstName             string  `json:"first_name"`
	LastName              string  `json:"last_name"`
	Username              *string `json:"username"`
	PrimaryEmailAddressID string  `json:"primary_email_address_id"`
	EmailAddresses        []struct {
		ID           string `json:"id"`
		EmailAddress string `json:"email_address"`
		Verification struct {
			Status string `json:"status"`
		} `json:"verification"`
	} `json:"email_addresses"`
}

// Profile loads a verified primary email and display name. Browser-supplied
// profile fields are never accepted.
func (s *ClerkProfileSource) Profile(ctx context.Context, clerkUserID string) (Profile, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, clerkUsersURL+url.PathEscape(clerkUserID), nil)
	if err != nil {
		return Profile{}, fmt.Errorf("build Clerk profile request: %w", err)
	}
	request.Header.Set("Authorization", "Bearer "+s.secretKey)
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return Profile{}, fmt.Errorf("fetch Clerk profile: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return Profile{}, fmt.Errorf("fetch Clerk profile: unexpected status %d", response.StatusCode)
	}

	var clerkUser clerkUserResponse
	if err := json.NewDecoder(response.Body).Decode(&clerkUser); err != nil {
		return Profile{}, fmt.Errorf("decode Clerk profile: %w", err)
	}
	if clerkUser.ID != clerkUserID {
		return Profile{}, fmt.Errorf("Clerk profile subject mismatch")
	}
	var email string
	for _, address := range clerkUser.EmailAddresses {
		if address.ID == clerkUser.PrimaryEmailAddressID && address.Verification.Status == "verified" {
			email = address.EmailAddress
			break
		}
	}
	if email == "" {
		return Profile{}, fmt.Errorf("Clerk user has no verified primary email")
	}

	displayName := strings.TrimSpace(clerkUser.FirstName + " " + clerkUser.LastName)
	if displayName == "" && clerkUser.Username != nil {
		displayName = strings.TrimSpace(*clerkUser.Username)
	}
	var name *string
	if displayName != "" {
		name = &displayName
	}
	return Profile{ClerkUserID: clerkUserID, Email: email, DisplayName: name}, nil
}
