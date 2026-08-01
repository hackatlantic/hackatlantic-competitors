package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/hackatlantic/hackatlantic-competitors/api/internal/database"
	"github.com/hackatlantic/hackatlantic-competitors/api/migrations"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	pool, err := database.Open(ctx, database.Config{URL: os.Getenv("DATABASE_URL")})
	if err != nil {
		log.Fatalf("configure database: %v", err)
	}
	defer pool.Close()
	if err := pool.Ping(ctx); err != nil {
		log.Fatalf("connect database: %v", err)
	}
	if err := migrations.Apply(ctx, pool); err != nil {
		log.Fatalf("apply migrations: %v", err)
	}
}
