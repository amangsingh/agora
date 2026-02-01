package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/amangsingh/agora/pkg/server"
	"github.com/amangsingh/agora/pkg/storage"
	"github.com/rs/cors"
)

func main() {
	// 1. Config
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	dbPath := os.Getenv("AGORA_DB")
	if dbPath == "" {
		dbPath = "agora.db"
	}

	// 2. Persistence
	repo, err := storage.NewRepository(dbPath)
	if err != nil {
		log.Fatalf("Failed to init storage: %v", err)
	}
	log.Printf("Storage initialized at %s", dbPath)

	// 3. Handlers
	handler := &server.AgentHandler{Repo: repo}

	// 4. Router
	mux := http.NewServeMux()
	mux.HandleFunc("POST /run", handler.HandleRun)
	mux.HandleFunc("GET /history", handler.HandleGetHistory)

	// 5. Middleware Chain
	// Apply CORS
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"}, // Restrict in production
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	})

	// Wrap Mux with Auth and Logger
	// Order: CORS -> Logger -> Auth -> Mux
	var rootHandler http.Handler = mux
	rootHandler = server.BearerAuth(rootHandler)
	rootHandler = server.Logger(rootHandler)
	rootHandler = c.Handler(rootHandler)

	// 6. Start
	serverAddr := fmt.Sprintf(":%s", port)
	log.Printf("Agora Sovereign Server listening on %s", serverAddr)
	if err := http.ListenAndServe(serverAddr, rootHandler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
