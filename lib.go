package main

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"

	_ "github.com/lib/pq"
	"google.golang.org/genai"
)

// InitGeminiClient initializes and returns the Google Gemini client
func InitGeminiClient(ctx context.Context) *genai.Client {
	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		log.Fatal("GEMINI_API_KEY environment variable is not set")
	}

	client, err := genai.NewClient(ctx, &genai.ClientConfig{
		APIKey: apiKey,
	})
	if err != nil {
		log.Fatalf("Failed to create Gemini client: %v", err)
	}

	return client
}

// InitPostgresDB establishes the database connection and runs auto-migrations
func InitPostgresDB() *sql.DB {
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

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}

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

	return db
}