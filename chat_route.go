package main

import (
	"bufio"
	"database/sql"
	"fmt"
	"log"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/valyala/fasthttp"
	"google.golang.org/genai"
)

type ChatRequest struct {
	ChatID  string `json:"chatId,omitempty"`
	Message string `json:"message"`
}

type ChatResponse struct {
	Reply  string `json:"reply"`
	ChatID string `json:"chatId,omitempty"`
}

// RegisterChatRoutes mounts the chat endpoints onto the Fiber app
func RegisterChatRoutes(app *fiber.App, db *sql.DB, client *genai.Client) {

	// 1. Existing Streaming Endpoint (Stateless)
	app.Post("/chat", func(c *fiber.Ctx) error {
		var req ChatRequest
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid request body"})
		}

		if req.Message == "" {
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Message cannot be empty"})
		}

		c.Set("Content-Type", "text/event-stream")
		c.Set("Cache-Control", "no-cache")
		c.Set("Connection", "keep-alive")
		c.Set("Transfer-Encoding", "chunked")

		c.Context().SetBodyStreamWriter(fasthttp.StreamWriter(func(w *bufio.Writer) {
			streamCtx := c.Context()
			iter := client.Models.GenerateContentStream(
				streamCtx,
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

				if len(resp.Candidates) > 0 && resp.Candidates[0].Content != nil {
					for _, part := range resp.Candidates[0].Content.Parts {
						if part.Text != "" {
							fmt.Fprintf(w, "data: %s\n\n", part.Text)
							w.Flush()
						}
					}
				}

				if resp.UsageMetadata != nil {
					log.Printf("[Token Usage] Prompt: %d | Candidates: %d | Total: %d",
						resp.UsageMetadata.PromptTokenCount,
						resp.UsageMetadata.CandidatesTokenCount,
						resp.UsageMetadata.TotalTokenCount,
					)
				}

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
			chatUUID = uuid.New()
		} else {
			chatUUID, parseErr = uuid.Parse(req.ChatID)
			if parseErr != nil {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "Invalid chatId UUID format"})
			}
		}

		reqCtx := c.Context()
		rows, err := db.QueryContext(reqCtx, "SELECT prompt, response FROM chat_history WHERE chat_id = $1 ORDER BY created_at ASC", chatUUID)
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
			history = append(history, &genai.Content{
				Role:  "user",
				Parts: []*genai.Part{{Text: dbPrompt}},
			})
			history = append(history, &genai.Content{
				Role:  "model",
				Parts: []*genai.Part{{Text: dbResponse}},
			})
		}

		// Check for any error encountered during iteration
		if err := rows.Err(); err != nil {
			log.Printf("Row iteration error: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to process chat history"})
		}

		chatSession, err := client.Chats.Create(reqCtx, "gemini-3.8-flash", nil, history)
		if err != nil {
			log.Printf("Failed to create chat session: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to initialize chat session"})
		}

		aiResp, err := chatSession.SendMessage(reqCtx, genai.Part{Text: req.Message})
		if err != nil {
			log.Printf("Gemini API error: %v", err)
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "Failed to generate response"})
		}

		replyText := aiResp.Text()

		_, err = db.ExecContext(reqCtx,
			"INSERT INTO chat_history (chat_id, prompt, response) VALUES ($1, $2, $3)",
			chatUUID, req.Message, replyText,
		)
		if err != nil {
			log.Printf("Failed to save chat history to database: %v", err)
		}

		return c.JSON(ChatResponse{
			Reply:  replyText,
			ChatID: chatUUID.String(),
		})
	})
}