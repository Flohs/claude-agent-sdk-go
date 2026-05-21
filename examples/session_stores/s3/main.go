// S3 SessionStore demo.
//
// Prerequisites:
//
//	go mod tidy
//	AWS credentials configured (env vars, ~/.aws/credentials, or instance profile)
//	S3_BUCKET environment variable set to an existing bucket
//	Claude CLI auth in place
//
// Usage:
//
//	S3_BUCKET=my-sessions-bucket go run .
package main

import (
	"context"
	"fmt"
	"log"
	"os"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"

	claude "github.com/Flohs/claude-agent-sdk-go"
)

func main() {
	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		log.Fatal("S3_BUCKET environment variable is required")
	}

	ctx := context.Background()
	cfg, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		log.Fatalf("aws config: %v", err)
	}
	s3Client := s3.NewFromConfig(cfg)
	store := NewS3SessionStore(s3Client, bucket)

	// Run a short Claude query with the S3 store enabled.
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

	// Verify the session landed in S3.
	projectKey := claude.ProjectKeyForDirectory("")
	sessions, err := store.ListSessions(ctx, projectKey)
	if err != nil {
		log.Fatal(err)
	}
	if len(sessions) == 0 {
		log.Fatal("no sessions found in S3")
	}
	fmt.Printf("S3: %d session(s) stored; newest = %s\n", len(sessions), sessions[0].SessionID)
}
