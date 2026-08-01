// bootstrap-organizer assigns the one-time initial organizer role to an existing local user.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/database"
	"github.com/hackatlantic/hackatlantic-competitors/api/internal/users"
)

const bootstrapTimeout = 30 * time.Second

func main() {
	if len(os.Args) != 2 || os.Args[1] == "" {
		log.Fatal("usage: bootstrap-organizer <clerk-user-id>")
	}
	ctx, cancel := context.WithTimeout(context.Background(), bootstrapTimeout)
	defer cancel()

	pool, err := database.Open(ctx, database.Config{URL: os.Getenv("DATABASE_URL")})
	if err != nil {
		log.Fatalf("configure database: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("connect database: %v", err)
	}
	if _, err := users.BootstrapFirstOrganizer(ctx, pool.Pool, bootstrapTimeout, os.Args[1]); err != nil {
		log.Fatalf("bootstrap organizer: %v", err)
	}
}
