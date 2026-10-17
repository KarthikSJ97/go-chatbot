package main

import (
	"context"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"google.golang.org/genai"
)

const(
	GEMINI_API_KEY = "GEMINI_API_KEY"
)

// ChatRequest defines the payload structure incoming from the client
type ChatRequest struct {
	Message string `json:"message"`
}

// ChatResponse defines the outgoing response payload
type ChatResponse struct {
	Reply string `json:"reply"`
}

func main() {
	ctx := context.Background()

	// Retrieve the API key directly from the OS environment
	apiKey := os.Getenv(GEMINI_API_KEY)
	if apiKey == "" {
		log.Fatal(GEMINI_API_KEY+" environment variable is not set")
	}

	// Initialize the Gemini client using the retrieved key
	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		log.Fatalf("Failed to create Gemini client: %v", err)
	}

	app := fiber.New()

	// POST /chat endpoint (stateless - no history)
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

		// Call Gemini API using the fast and free-tier friendly flash model
		resp, err := client.Models.GenerateContent(
			ctx,
			"gemini-3.8-flash",
			genai.Text(req.Message),
			nil,
		)
		if err != nil {
			log.Printf("Gemini API error: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to generate response from AI model",
			})
		}

		return c.JSON(ChatResponse{
			Reply: resp.Text(),
		})
	})

	log.Println("Server is running on port 8080...")
	log.Fatal(app.Listen(":8080"))
}