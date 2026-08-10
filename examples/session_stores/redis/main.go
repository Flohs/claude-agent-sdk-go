// Redis SessionStore demo.
//
// Prerequisites:
//
//	go mod tidy
//	a running Redis instance (default: localhost:6379)
//	Claude CLI auth in place
//
// Usage:
//
//	REDIS_ADDR=localhost:6379 go run .
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	claude "github.com/Flohs/claude-agent-sdk-go/v3"
	"github.com/redis/go-redis/v9"
)

func main() {
	addr := os.Getenv("REDIS_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	rdb := redis.NewClient(&redis.Options{Addr: addr})
	defer rdb.Close()

	store := NewRedisSessionStore(rdb)
	ctx := context.Background()

	// Run a short Claude query with the Redis store enabled. The SDK mirrors
	// every transcript line to Redis as the CLI emits it.
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

	// Verify the session landed in Redis.
	projectKey := claude.ProjectKeyForDirectory("")
	sessions, err := store.ListSessions(ctx, projectKey)
	if err != nil {
		log.Fatal(err)
	}
	if len(sessions) == 0 {
		log.Fatal("no sessions found in Redis — check that the store is configured correctly")
	}
	fmt.Printf("Redis: %d session(s) stored; newest = %s\n", len(sessions), sessions[0].SessionID)
}
