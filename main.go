package main

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	_ "github.com/lib/pq" // <--- This blank import is mandatory for database/sql
	"github.com/valyala/fasthttp"
	"google.golang.org/genai"
)

const (
	GEMINI_API_KEY = "GEMINI_API_KEY"
	DATABASE_URL   = "DATABASE_URL"
)

// ChatRequest defines the payload structure incoming from the client
type ChatRequest struct {
	ChatID  string `json:"chatId,omitempty"`
	Message string `json:"message"`
}

type ChatResponse struct {
	Reply  string `json:"reply"`
	ChatID string `json:"chatId,omitempty"`
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

	// Retrieve DB credentials from environment variables
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbName := os.Getenv("DB_NAME")
	dbHost := os.Getenv("DB_HOST")
	dbPort := os.Getenv("DB_PORT")

	if dbUser == "" || dbPassword == "" || dbName == "" || dbHost == "" || dbPort == "" {
		log.Fatal("Database environment variables are not fully set")
	}

	dbURL := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable",
		dbUser, dbPassword, dbHost, dbPort, dbName,
	)

	// Connect to PostgreSQL
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Database ping failed: %v", err)
	}
	log.Println("Successfully connected to PostgreSQL database.")

	// Ensure database schema exists
	_, err = db.Exec(`
		CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
		CREATE TABLE IF NOT EXISTS chat_history (
			id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
			chat_id UUID NOT NULL,
			prompt TEXT NOT NULL,
			response TEXT NOT NULL,
			created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
		);
	`)
	if err != nil {
		log.Fatalf("Failed to create chat_history table: %v", err)
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

	// 2. Context-Aware, Non-Streaming JSON Endpoint backed by PostgreSQL
	app.Post("/chat/context", func(c *fiber.Ctx) error {
		var req ChatRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
		}

		if req.Message == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Message cannot be empty"})
		}

		var chatUUID uuid.UUID
		var parseErr error

		if req.ChatID == "" {
			// Fresh chat: generate a random UUID
			chatUUID = uuid.New()
		} else {
			chatUUID, parseErr = uuid.Parse(req.ChatID)
			if parseErr != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid chatId UUID format"})
			}
		}

		// Load prior history for this chatId from PostgreSQL to build context
		rows, err := db.QueryContext(ctx, "SELECT prompt, response FROM chat_history WHERE chat_id = $1 ORDER BY created_at ASC", chatUUID)
		if err != nil {
			log.Printf("Database query error: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to fetch chat history"})
		}
		defer rows.Close()

		var history []*genai.Content
		for rows.Next() {
			var dbPrompt, dbResponse string
			if err := rows.Scan(&dbPrompt, &dbResponse); err != nil {
				continue
			}
			// Append historical turn (User -> Model)
			history = append(history, &genai.Content{
				Role:  "user",
				Parts: []*genai.Part{{Text: dbPrompt}},
			})
			history = append(history, &genai.Content{
				Role:  "model",
				Parts: []*genai.Part{{Text: dbResponse}},
			})
		}

		// Create a Gemini chat session initialized with historical context
		chatSession, err := client.Chats.Create(ctx, "gemini-3.8-flash", nil, history)
		if err != nil {
			log.Printf("Failed to create chat session: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to initialize chat session"})
		}

		// Send new prompt to model
		aiResp, err := chatSession.SendMessage(ctx, genai.Part{Text: req.Message})
		if err != nil {
			log.Printf("Gemini API error: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate response"})
		}

		replyText := aiResp.Text()

		// Persist the new turn into PostgreSQL
		_, err = db.ExecContext(ctx,
			"INSERT INTO chat_history (chat_id, prompt, response) VALUES ($1, $2, $3)",
			chatUUID, req.Message, replyText,
		)
		if err != nil {
			log.Printf("Failed to save chat history to database: %v", err)
			// Non-fatal for the response, but good to log
		}

		return c.JSON(ChatResponse{
			Reply:  replyText,
			ChatID: chatUUID.String(),
		})
	})

	log.Println("Server is running on port 8080...")
	log.Fatal(app.Listen(":8080"))
}