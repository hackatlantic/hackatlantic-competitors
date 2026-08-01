package resumes

import (
	"context"
	"strings"
	"testing"
)

func TestNewSpacesStoreRequiresCompleteHTTPSConfiguration(t *testing.T) {
	t.Parallel()

	if _, err := NewSpacesStore(context.Background(), "", "tor1", "resumes", "access", "secret"); err == nil || !strings.Contains(err.Error(), "required together") {
		t.Fatalf("expected incomplete configuration error, got %v", err)
	}
	if _, err := NewSpacesStore(context.Background(), "http://tor1.digitaloceanspaces.com", "tor1", "resumes", "access", "secret"); err == nil || !strings.Contains(err.Error(), "must use HTTPS") {
		t.Fatalf("expected HTTPS configuration error, got %v", err)
	}
	if _, err := NewSpacesStore(context.Background(), "https://tor1.digitaloceanspaces.com", "tor1", "resumes", "access", "secret"); err != nil {
		t.Fatalf("expected valid Spaces configuration, got %v", err)
	}
}
