package main

import (
	"context"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/google/genai"
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

	// Initialize the Gemini client (it automatically looks for GEMINI_API_KEY environment variable)
	client, err := genai.NewClient(ctx, nil)
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
			"gemini-2.5-flash",
			genai.Text(req.Message),
			nil,
		)
		if err != nil {
			log.Printf("Gemini API error: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"error": "Failed to generate response from AI model",
			})
		}

		// Extract the text response from the model
		answer := resp.Text()

		return c.JSON(ChatResponse{
			Reply: answer,
		})
	})

	log.Println("Server is running on port 8080...")
	log.Fatal(app.Listen(":8080"))
}