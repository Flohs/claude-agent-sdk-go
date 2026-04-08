// Multimodal input examples showing how to send images and documents to Claude.
//
// Usage:
//
//	go run ./examples/multimodal_input
package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"

	claude "github.com/Flohs/claude-agent-sdk-go"
)

func imageInputExample(ctx context.Context) {
	fmt.Println("=== Image Input Example ===")

	// Read and encode an image file
	imageData, err := os.ReadFile("image.png")
	if err != nil {
		log.Fatal("Failed to read image.png:", err)
	}
	encoded := base64.StdEncoding.EncodeToString(imageData)

	// Build multimodal content: text + image
	content := []any{
		claude.NewTextContent("Describe what you see in this image."),
		claude.NewBase64Content("image/png", encoded),
	}

	client := claude.NewClient(nil)
	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	if err := client.SendQueryWithContent(ctx, content); err != nil {
		log.Fatal(err)
	}

	for msg := range client.ReceiveResponse(ctx) {
		if m, ok := msg.(*claude.AssistantMessage); ok {
			for _, block := range m.Content {
				if tb, ok := block.(claude.TextBlock); ok {
					fmt.Println("Claude:", tb.Text)
				}
			}
		}
	}
	fmt.Println()
}

func documentInputExample(ctx context.Context) {
	fmt.Println("=== Document Input Example ===")

	// Read and encode a PDF file
	pdfData, err := os.ReadFile("document.pdf")
	if err != nil {
		log.Fatal("Failed to read document.pdf:", err)
	}
	encoded := base64.StdEncoding.EncodeToString(pdfData)

	// Build multimodal content: text + document
	content := []any{
		claude.NewTextContent("Summarize the key points of this document."),
		claude.NewBase64Content("application/pdf", encoded),
	}

	client := claude.NewClient(nil)
	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	if err := client.SendQueryWithContent(ctx, content); err != nil {
		log.Fatal(err)
	}

	for msg := range client.ReceiveResponse(ctx) {
		if m, ok := msg.(*claude.AssistantMessage); ok {
			for _, block := range m.Content {
				if tb, ok := block.(claude.TextBlock); ok {
					fmt.Println("Claude:", tb.Text)
				}
			}
		}
	}
	fmt.Println()
}

func multipleImagesExample(ctx context.Context) {
	fmt.Println("=== Multiple Images Example ===")

	// You can attach multiple images in a single message
	img1, err := os.ReadFile("photo1.jpg")
	if err != nil {
		log.Fatal("Failed to read photo1.jpg:", err)
	}
	img2, err := os.ReadFile("photo2.jpg")
	if err != nil {
		log.Fatal("Failed to read photo2.jpg:", err)
	}

	content := []any{
		claude.NewTextContent("Compare these two images. What are the differences?"),
		claude.NewBase64Content("image/jpeg", base64.StdEncoding.EncodeToString(img1)),
		claude.NewBase64Content("image/jpeg", base64.StdEncoding.EncodeToString(img2)),
	}

	client := claude.NewClient(nil)
	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	if err := client.SendQueryWithContent(ctx, content); err != nil {
		log.Fatal(err)
	}

	for msg := range client.ReceiveResponse(ctx) {
		if m, ok := msg.(*claude.AssistantMessage); ok {
			for _, block := range m.Content {
				if tb, ok := block.(claude.TextBlock); ok {
					fmt.Println("Claude:", tb.Text)
				}
			}
		}
	}
	fmt.Println()
}

func textOnlyExample(ctx context.Context) {
	fmt.Println("=== Text-Only (Backward Compatible) ===")

	// SendQuery still works for text-only messages
	client := claude.NewClient(nil)
	if err := client.Connect(ctx); err != nil {
		log.Fatal(err)
	}
	defer func() { _ = client.Close() }()

	if err := client.SendQuery(ctx, "What is 2 + 2?"); err != nil {
		log.Fatal(err)
	}

	for msg := range client.ReceiveResponse(ctx) {
		if m, ok := msg.(*claude.AssistantMessage); ok {
			for _, block := range m.Content {
				if tb, ok := block.(claude.TextBlock); ok {
					fmt.Println("Claude:", tb.Text)
				}
			}
		}
	}
	fmt.Println()
}

func main() {
	ctx := context.Background()

	if len(os.Args) < 2 {
		fmt.Println("Usage: go run ./examples/multimodal_input <example>")
		fmt.Println()
		fmt.Println("Examples:")
		fmt.Println("  text        Text-only (backward compatible)")
		fmt.Println("  image       Single image input (requires image.png)")
		fmt.Println("  document    PDF document input (requires document.pdf)")
		fmt.Println("  multi       Multiple images (requires photo1.jpg, photo2.jpg)")
		return
	}

	switch os.Args[1] {
	case "text":
		textOnlyExample(ctx)
	case "image":
		imageInputExample(ctx)
	case "document":
		documentInputExample(ctx)
	case "multi":
		multipleImagesExample(ctx)
	default:
		fmt.Fprintf(os.Stderr, "Unknown example: %s\n", os.Args[1])
		os.Exit(1)
	}
}
