package main

import (
	"context"
	"log"

	"github.com/gofiber/fiber/v2"
)

func main() {
	ctx := context.Background()

	// Initialize Gemini and PostgreSQL using lib helpers
	client := InitGeminiClient(ctx)
	db := InitPostgresDB()
	defer db.Close()

	app := fiber.New()

	// Register all chat routes
	RegisterChatRoutes(app, db, client)

	log.Println("Server is running on port 8080...")
	log.Fatal(app.Listen(":8080"))
}