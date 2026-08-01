package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/applications"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/auth"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/checkpoints"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/database"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/decisions"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/httpapi"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/operations"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/passes"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/redemptions"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/resumes"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/reviews"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

const (
	defaultAddress  = ":8080"
	shutdownTimeout = 10 * time.Second
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	lifecycleCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	address := os.Getenv("HTTP_ADDRESS")
	if address == "" {
		address = defaultAddress
	}

	configureCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	pool, err := database.Open(configureCtx, database.Config{
		URL:                os.Getenv("DATABASE_URL"),
		MaxConns:           int32Env("DATABASE_MAX_CONNS", 10),
		QueryTimeout:       durationEnv("DATABASE_QUERY_TIMEOUT", 5*time.Second),
		TransactionTimeout: durationEnv("DATABASE_TRANSACTION_TIMEOUT", 15*time.Second),
	})
	if err != nil {
		logger.Error("configure database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	passSettings, err := loadPassSettings(os.Getenv)
	if err != nil {
		logger.Error("configure pass credentials", "error", err)
		os.Exit(1)
	}
	passService, err := passes.NewService(
		pool.Pool,
		durationEnv("DATABASE_QUERY_TIMEOUT", 5*time.Second),
		durationEnv("DATABASE_TRANSACTION_TIMEOUT", 15*time.Second),
		passSettings,
	)
	if err != nil {
		logger.Error("configure pass credentials", "error", err)
		os.Exit(1)
	}

	checkpointService := checkpoints.NewService(
		pool.Pool,
		durationEnv("DATABASE_QUERY_TIMEOUT", 5*time.Second),
	)
	redemptionService := redemptions.NewService(
		pool.Pool,
		durationEnv("DATABASE_TRANSACTION_TIMEOUT", 15*time.Second),
		passService,
	)
	claimRateLimiter := httpapi.NewClaimRateLimiter(0, 0, 0)
	if err := claimRateLimiter.TrustProxyCIDRs(commaSeparatedEnv("TRUSTED_PROXY_CIDRS")); err != nil {
		logger.Error("configure trusted proxies", "error", err)
		os.Exit(1)
	}
	resumeStore, err := loadResumeStore(os.Getenv)
	if err != nil {
		logger.Error("configure resume storage", "error", err)
		os.Exit(1)
	}

	dependencies := httpapi.Dependencies{
		Readiness: pool,
		Applications: applications.NewService(
			pool.Pool,
			durationEnv("DATABASE_QUERY_TIMEOUT", 5*time.Second),
			durationEnv("DATABASE_TRANSACTION_TIMEOUT", 15*time.Second),
		),
		Reviews: reviews.NewService(
			pool.Pool,
			durationEnv("DATABASE_QUERY_TIMEOUT", 5*time.Second),
			durationEnv("DATABASE_TRANSACTION_TIMEOUT", 15*time.Second),
		),
		Decisions: decisions.NewService(
			pool.Pool,
			durationEnv("DATABASE_QUERY_TIMEOUT", 5*time.Second),
			durationEnv("DATABASE_TRANSACTION_TIMEOUT", 15*time.Second),
		),
		Passes:           passService,
		Checkpoints:      checkpointService,
		Redemptions:      redemptionService,
		ClaimRateLimiter: claimRateLimiter,
		Operations: operations.NewService(
			pool.Pool,
			durationEnv("DATABASE_QUERY_TIMEOUT", 5*time.Second),
			durationEnv("DATABASE_TRANSACTION_TIMEOUT", 15*time.Second),
		),
		Resumes:        resumes.NewService(pool.Pool, resumeStore, durationEnv("DATABASE_QUERY_TIMEOUT", 5*time.Second)),
		AllowedOrigins: commaSeparatedEnv("CORS_ALLOWED_ORIGINS"),
	}
	if verifier, resolver, err := clerkDependencies(lifecycleCtx, configureCtx, pool.Pool); err != nil {
		logger.Error("configure Clerk authentication", "error", err)
		os.Exit(1)
	} else if verifier != nil {
		dependencies.Verifier = verifier
		dependencies.Users = resolver
		dependencies.StaffRoles = resolver
	} else {
		logger.Warn("Clerk authentication is disabled; protected routes reject every request")
	}

	server := &http.Server{
		Addr:              address,
		Handler:           httpapi.NewHandlerWithDependencies("dev", dependencies),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		logger.Info("api listening", "address", address)
		if err := server.ListenAndServe(); err != nil &&
			!errors.Is(err, http.ErrServerClosed) {
			logger.Error("api stopped unexpectedly", "error", err)
			os.Exit(1)
		}
	}()

	<-lifecycleCtx.Done()
	logger.Info("api shutting down")

	ctx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("api shutdown failed", "error", err)
		os.Exit(1)
	}
}

func loadResumeStore(getenv func(string) string) (resumes.Store, error) {
	supabaseURL := strings.TrimSpace(getenv("SUPABASE_URL"))
	serviceKey := strings.TrimSpace(getenv("SUPABASE_SERVICE_ROLE_KEY"))
	bucket := strings.TrimSpace(getenv("RESUME_STORAGE_BUCKET"))
	if supabaseURL != "" || serviceKey != "" || bucket != "" {
		return resumes.NewSupabaseStore(supabaseURL, serviceKey, bucket)
	}
	directory := strings.TrimSpace(getenv("RESUME_STORAGE_DIRECTORY"))
	if directory == "" {
		directory = ".data/resumes"
	}
	return resumes.NewFileStore(directory)
}

type clerkSettings struct {
	SecretKey         string
	Issuer            string
	AuthorizedParties []string
	JWKSURL           string
}

func clerkDependencies(lifecycleCtx context.Context, initialCtx context.Context, pool *pgxpool.Pool) (auth.Verifier, *users.Service, error) {
	settings, enabled, err := loadClerkSettings(os.Getenv)
	if err != nil {
		return nil, nil, err
	}
	if !enabled {
		return nil, nil, nil
	}
	verifier, err := auth.NewJWTVerifier(lifecycleCtx, initialCtx, auth.ClerkConfig{
		Issuer:            settings.Issuer,
		AuthorizedParties: settings.AuthorizedParties,
		JWKSURL:           settings.JWKSURL,
	})
	if err != nil {
		return nil, nil, err
	}
	profiles, err := users.NewClerkProfileSource(settings.SecretKey)
	if err != nil {
		return nil, nil, err
	}
	return verifier, users.NewService(pool, profiles, durationEnv("DATABASE_QUERY_TIMEOUT", 5*time.Second)), nil
}

func loadClerkSettings(getenv func(string) string) (clerkSettings, bool, error) {
	settings := clerkSettings{
		SecretKey:         strings.TrimSpace(getenv("CLERK_SECRET_KEY")),
		Issuer:            strings.TrimSpace(getenv("CLERK_ISSUER_URL")),
		AuthorizedParties: commaSeparated(getenv("CLERK_AUTHORIZED_PARTIES")),
		JWKSURL:           strings.TrimSpace(getenv("CLERK_JWKS_URL")),
	}
	if settings.SecretKey == "" && settings.Issuer == "" && len(settings.AuthorizedParties) == 0 && settings.JWKSURL == "" {
		return clerkSettings{}, false, nil
	}
	if settings.SecretKey == "" || settings.Issuer == "" || len(settings.AuthorizedParties) == 0 {
		return clerkSettings{}, false, fmt.Errorf("CLERK_SECRET_KEY, CLERK_ISSUER_URL, and CLERK_AUTHORIZED_PARTIES must be configured together")
	}
	return settings, true, nil
}

func loadPassSettings(getenv func(string) string) (passes.Config, error) {
	settings := passes.Config{
		QRTokenPepper:    strings.TrimSpace(getenv("QR_TOKEN_PEPPER")),
		ClaimTokenPepper: strings.TrimSpace(getenv("CLAIM_TOKEN_PEPPER")),
		AppBaseURL:       strings.TrimSpace(getenv("APP_BASE_URL")),
	}
	if settings.QRTokenPepper == "" || settings.ClaimTokenPepper == "" {
		return passes.Config{}, fmt.Errorf("QR_TOKEN_PEPPER and CLAIM_TOKEN_PEPPER are required")
	}
	if settings.QRTokenPepper == settings.ClaimTokenPepper {
		return passes.Config{}, fmt.Errorf("QR_TOKEN_PEPPER and CLAIM_TOKEN_PEPPER must differ")
	}
	return settings, nil
}

func commaSeparatedEnv(name string) []string {
	return commaSeparated(os.Getenv(name))
}

func commaSeparated(raw string) []string {
	rawValues := strings.Split(raw, ",")
	values := make([]string, 0, len(rawValues))
	for _, value := range rawValues {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return values
}

func int32Env(name string, fallback int32) int32 {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || value < 1 {
		return fallback
	}
	return int32(value)
}

func durationEnv(name string, fallback time.Duration) time.Duration {
	raw := os.Getenv(name)
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil || value <= 0 {
		return fallback
	}
	return value
}
