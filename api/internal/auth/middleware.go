package auth

import (
	"net/http"
	"strings"
)

// BearerToken extracts one strictly formatted Authorization bearer token.
func BearerToken(request *http.Request) (string, error) {
	values := request.Header.Values("Authorization")
	if len(values) != 1 {
		return "", ErrUnauthenticated
	}
	const prefix = "Bearer "
	value := values[0]
	if !strings.HasPrefix(value, prefix) || strings.TrimSpace(strings.TrimPrefix(value, prefix)) == "" {
		return "", ErrUnauthenticated
	}
	return strings.TrimPrefix(value, prefix), nil
}
