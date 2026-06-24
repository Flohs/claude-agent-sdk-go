// PostgreSQL SessionStore demo.
//
// Prerequisites:
//
//	go mod tidy
//	a running PostgreSQL instance
//	Claude CLI auth in place
//
// Usage:
//
//	DATABASE_URL=postgres://user:pass@localhost/mydb go run .
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	claude "github.com/Flohs/claude-agent-sdk-go/v2"
	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://localhost/claude_sessions?sslmode=disable"
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		log.Fatalf("connect: %v", err)
	}
	defer pool.Close()

	store := NewPostgresSessionStore(pool)
	if err := store.InitSchema(ctx); err != nil {
		log.Fatalf("init schema: %v", err)
	}

	// Run a short Claude query with the Postgres store enabled.
	maxTurns := 1
	msgs, errs := claude.Query(ctx, "Say hello in one sentence.", &claude.Options{
		SessionStore: store,
		MaxTurns:     &maxTurns,
	})
	for msg := range msgs {
		if am, ok := msg.(*claude.AssistantMessage); ok {
			for _, b := range am.Content {
				if tb, ok := b.(claude.TextBlock); ok {
					fmt.Println("Claude:", tb.Text)
				}
			}
		}
	}
	for err := range errs {
		log.Println("error:", err)
	}

	// Verify the session landed in Postgres.
	projectKey := claude.ProjectKeyForDirectory("")
	sessions, err := store.ListSessions(ctx, projectKey)
	if err != nil {
		log.Fatal(err)
	}
	if len(sessions) == 0 {
		log.Fatal("no sessions found in Postgres")
	}
	fmt.Printf("Postgres: %d session(s) stored; newest = %s\n", len(sessions), sessions[0].SessionID)
}
