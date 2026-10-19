package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/valyala/fasthttp"
	"google.golang.org/genai"
)

const (
	GEMINI_API_KEY = "GEMINI_API_KEY"
)

// ChatRequest defines the payload structure incoming from the client
type ChatRequest struct {
	Message string `json:"message"`
}

func main() {
	ctx := context.Background()

	// Retrieve the API key directly from the OS environment
	apiKey := os.Getenv(GEMINI_API_KEY)
	if apiKey == "" {
		log.Fatal(GEMINI_API_KEY + " environment variable is not set")
	}

	// Initialize the Gemini client using the retrieved key
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		log.Fatalf("Failed to create Gemini client: %v", err)
	}

	app := fiber.New()

	// POST /chat endpoint (stateless streaming with metadata logging)
	app.Post("/chat", func(c *fiber.Ctx) error {
		var req ChatRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Invalid request body",
			})
		}

		if req.Message == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Message cannot be empty",
			})
		}

		// Set required SSE headers for Fiber
		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("Transfer-Encoding", "chunked")

		// Stream response back via fasthttp stream writer
		c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
			iter := client.Models.GenerateContentStream(
				ctx,
				"gemini-3.8-flash",
				genai.Text(req.Message),
				nil,
			)

			for resp, err := range iter {
				if err != nil {
					log.Printf("Gemini stream error: %v", err)
					fmt.Fprintf(w, "event: error\ndata: %s\n\n", err.Error())
					w.Flush()
					return
				}

				// Extract text and stream immediately to client
				if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
					for _, part := range resp.Candidates[0].Content.Parts {
						if part.Text != "" {
							fmt.Fprintf(w, "data: %s\n\n", part.Text)
							w.Flush()
						}
					}
				}

				// Log token usage metadata if present in the chunk
				if resp.UsageMetadata != nil {
					log.Printf("[Token Usage] Prompt: %d | Candidates: %d | Total: %d",
						resp.UsageMetadata.PromptTokenCount,
						resp.UsageMetadata.CandidatesTokenCount,
						resp.UsageMetadata.TotalTokenCount,
					)
				}

				// Log candidate details like finish reason and safety ratings
				for _, candidate := range resp.Candidates {
					if candidate.FinishReason != "" {
						log.Printf("[Finish Reason]: %v", candidate.FinishReason)
					}
					if len(candidate.SafetyRatings) > 0 {
						for _, rating := range candidate.SafetyRatings {
							log.Printf("[Safety Rating] Category: %v | Probability: %v", rating.Category, rating.Probability)
						}
					}
				}
			}

			// End event to signal completion
			fmt.Fprintf(w, "event: end\ndata: [DONE]\n\n")
			w.Flush()
		}))

		return nil
	})

	log.Println("Server is running on port 8080...")
	log.Fatal(app.Listen(":8080"))
}