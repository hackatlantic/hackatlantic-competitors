package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/passes"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

const (
	defaultClaimLimit     = 30
	defaultClaimWindow    = time.Minute
	defaultClaimBucketCap = 10_000
)

type passLifecycleService interface {
	Issue(context.Context, users.User, string) (passes.Issuance, error)
	Reissue(context.Context, users.User, string) (passes.Issuance, error)
	Revoke(context.Context, users.User, string) (passes.Pass, error)
	WebPass(context.Context, users.User) (passes.WebPass, error)
	ResolveClaim(context.Context, string) (passes.ClaimPass, error)
	SummaryForApplication(context.Context, users.User, string) (passes.OrganizerSummary, error)
}

type claimRateBucket struct {
	started time.Time
	count   int
}

// ClaimRateLimiter bounds claim attempts by the request peer address. It keeps
// no credential values and caps state so untrusted addresses cannot grow memory
// without bound.
type ClaimRateLimiter struct {
	mu             sync.Mutex
	buckets        map[string]claimRateBucket
	limit          int
	window         time.Duration
	maxEntries     int
	now            func() time.Time
	trustedProxies []*net.IPNet
}

// TrustProxyCIDRs enables proxy-aware client bucketing. Forwarded addresses
// are ignored unless the immediate peer is in this explicit allowlist.
func (l *ClaimRateLimiter) TrustProxyCIDRs(values []string) error {
	networks := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		_, network, err := net.ParseCIDR(strings.TrimSpace(value))
		if err != nil {
			return fmt.Errorf("parse trusted proxy CIDR: %w", err)
		}
		networks = append(networks, network)
	}
	l.trustedProxies = networks
	return nil
}

// NewClaimRateLimiter creates the per-process public claim limiter.
func NewClaimRateLimiter(limit int, window time.Duration, maxEntries int) *ClaimRateLimiter {
	if limit < 1 {
		limit = defaultClaimLimit
	}
	if window <= 0 {
		window = defaultClaimWindow
	}
	if maxEntries < 1 {
		maxEntries = defaultClaimBucketCap
	}
	return &ClaimRateLimiter{
		buckets: make(map[string]claimRateBucket), limit: limit, window: window, maxEntries: maxEntries, now: time.Now,
	}
}

func (l *ClaimRateLimiter) Allow(request *http.Request) bool {
	address := l.requestClientAddress(request)
	now := l.now()
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, bucket := range l.buckets {
		if now.Sub(bucket.started) >= l.window {
			delete(l.buckets, key)
		}
	}
	bucket, ok := l.buckets[address]
	if !ok {
		if len(l.buckets) >= l.maxEntries {
			oldestKey := ""
			var oldest time.Time
			for key, candidate := range l.buckets {
				if oldestKey == "" || candidate.started.Before(oldest) {
					oldestKey, oldest = key, candidate.started
				}
			}
			delete(l.buckets, oldestKey)
		}
		l.buckets[address] = claimRateBucket{started: now, count: 1}
		return true
	}
	if bucket.count >= l.limit {
		return false
	}
	bucket.count++
	l.buckets[address] = bucket
	return true
}

func (l *ClaimRateLimiter) requestClientAddress(request *http.Request) string {
	host, _, err := net.SplitHostPort(request.RemoteAddr)
	if err != nil || host == "" {
		host = request.RemoteAddr
	}
	peer := net.ParseIP(host)
	if peer == nil {
		return "unknown"
	}
	if !l.isTrustedProxy(peer) {
		return peer.String()
	}
	forwarded := strings.Split(request.Header.Get("X-Forwarded-For"), ",")
	for index := len(forwarded) - 1; index >= 0; index-- {
		candidate := net.ParseIP(strings.TrimSpace(forwarded[index]))
		if candidate == nil {
			return peer.String()
		}
		if !l.isTrustedProxy(candidate) {
			return candidate.String()
		}
	}
	return peer.String()
}

func (l *ClaimRateLimiter) isTrustedProxy(address net.IP) bool {
	for _, network := range l.trustedProxies {
		if network.Contains(address) {
			return true
		}
	}
	return false
}

func issuePassHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		service, ok := passService(w, dependencies)
		if !ok {
			return
		}
		issuance, err := service.Issue(request.Context(), organizer, request.PathValue("attendeeId"))
		if err != nil {
			writePassError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, issuance)
	}
}

func reissuePassHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		service, ok := passService(w, dependencies)
		if !ok {
			return
		}
		issuance, err := service.Reissue(request.Context(), organizer, request.PathValue("passId"))
		if err != nil {
			writePassError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, issuance)
	}
}

func revokePassHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		organizer, ok := requireRole(w, request, dependencies, users.RoleOrganizer)
		if !ok {
			return
		}
		service, ok := passService(w, dependencies)
		if !ok {
			return
		}
		pass, err := service.Revoke(request.Context(), organizer, request.PathValue("passId"))
		if err != nil {
			writePassError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, pass)
	}
}

func webPassHandler(dependencies Dependencies) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		attendee, ok := requireRole(w, request, dependencies, users.RoleApplicant)
		if !ok {
			return
		}
		service, ok := passService(w, dependencies)
		if !ok {
			return
		}
		pass, err := service.WebPass(request.Context(), attendee)
		if err != nil {
			writePassError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, pass)
	}
}

func claimPassHandler(dependencies Dependencies, limiter *ClaimRateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, request *http.Request) {
		if !limiter.Allow(request) {
			w.Header().Set("Retry-After", strconv.Itoa(max(1, int(limiter.window.Seconds()))))
			writeError(w, http.StatusTooManyRequests, "claim_rate_limited", "Too many claim attempts. Please try again later.")
			return
		}
		service, ok := passService(w, dependencies)
		if !ok {
			return
		}
		pass, err := service.ResolveClaim(request.Context(), request.PathValue("claimToken"))
		if err != nil {
			writeClaimError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, pass)
	}
}

func passService(w http.ResponseWriter, dependencies Dependencies) (passLifecycleService, bool) {
	if dependencies.Passes == nil {
		writeError(w, http.StatusServiceUnavailable, "dependency_unavailable", "The API is not ready.")
		return nil, false
	}
	return dependencies.Passes, true
}

func writePassError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, passes.ErrForbidden):
		writeError(w, http.StatusForbidden, "forbidden", "The requested pass operation is not permitted.")
	case errors.Is(err, passes.ErrActivePass):
		writeError(w, http.StatusConflict, "pass_active", "The attendee already has an active pass.")
	case errors.Is(err, passes.ErrRSVPRequired):
		writeError(w, http.StatusConflict, "rsvp_required", "The attendee must confirm their RSVP before a pass can be issued or reissued.")
	case errors.Is(err, passes.ErrNotFound), errors.Is(err, passes.ErrInvalidID), errors.Is(err, passes.ErrInvalidCred):
		writeError(w, http.StatusNotFound, "pass_not_found", "Pass not found.")
	default:
		writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
	}
}

func writeClaimError(w http.ResponseWriter, err error) {
	if errors.Is(err, passes.ErrNotFound) || errors.Is(err, passes.ErrInvalidCred) {
		writeError(w, http.StatusNotFound, "pass_not_found", "Pass not found.")
		return
	}
	writeError(w, http.StatusInternalServerError, "internal_error", "An unexpected error occurred.")
}
