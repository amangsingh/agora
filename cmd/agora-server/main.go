package main

import (
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/amangsingh/agora"
	"github.com/amangsingh/agora/llm"
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

	// 3. The Self — constructed ONCE at process start from its blueprint.
	// The server is the first consumer of the framework type: every request
	// executes through this one Self's banks (models, persona, memory) for
	// the lifetime of the process. Nothing below conjures resources per
	// request.
	self, err := agora.NewSelf(agora.Blueprint{
		ID:            "agora",
		Name:          "agora",
		Models:        []string{"llama3"},
		SystemPrompts: []string{"You are a helpful API agent."},
		Memory:        repo,
		ModelFactory: func(model string) agora.Model {
			return llm.NewOllamaLLM("http://localhost:11434/v1", model)
		},
	})
	if err != nil {
		log.Fatalf("Failed to construct Self: %v", err)
	}
	log.Printf("Self %q constructed; banks live for process lifetime", self.ID())

	// 3b. The Hum (Step 5): the server's Self starts its non-returning loop
	// at process start. Loop A (inbound requests) and the Hum share this one
	// Self and its D bank; episodes are serialized on the Self. The Hum is
	// egress-less until Step 6 lands behind the SEC gate.
	hum, err := self.StartHum(agora.HumConfig{})
	if err != nil {
		log.Fatalf("Failed to start Hum: %v", err)
	}
	defer hum.Stop() // liveness includes dying well
	log.Printf("Self %q is humming (interval %s)", self.ID(), agora.DefaultHumInterval)

	// 4. Handlers
	handler := &server.AgentHandler{Repo: repo, Self: self}

	// 4. Router
	mux := http.NewServeMux()
	mux.HandleFunc("POST /run", handler.HandleRun)
	mux.HandleFunc("POST /selves", handler.HandleEstablishSelf)
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
