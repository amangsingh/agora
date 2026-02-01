package server

import (
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
)

// Logger is a middleware that logs request duration and status.
func Logger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		// Capture response status (using a wrapper)
		// For brevity, we just log start/method/path
		log.Printf("[REQ] %s %s", r.Method, r.URL.Path)

		next.ServeHTTP(w, r)

		log.Printf("[RES] %s %s took %v", r.Method, r.URL.Path, time.Since(start))
	})
}

// BearerAuth ensures strict Bearer token authentication.
func BearerAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := os.Getenv("AGORA_AUTH_TOKEN")
		if token == "" {
			// If no token configured, warn but allow? Or deny?
			// Secure by default: Deny if not configured or empty is dangerous.
			// Let's assume localhost dev mode if empty, or safer: Deny.
			log.Println("WARNING: AGORA_AUTH_TOKEN is not set. Denying all requests.")
			http.Error(w, "Server Misconfiguration: Auth Token missing", http.StatusInternalServerError)
			return
		}

		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Unauthorized: No token provided", http.StatusUnauthorized)
			return
		}

		parts := strings.Split(authHeader, " ")
		if len(parts) != 2 || parts[0] != "Bearer" {
			http.Error(w, "Unauthorized: Invalid Authorization header format", http.StatusUnauthorized)
			return
		}

		if subtle.ConstantTimeCompare([]byte(parts[1]), []byte(token)) != 1 {
			http.Error(w, "Unauthorized: Invalid token", http.StatusUnauthorized)
			return
		}

		next.ServeHTTP(w, r)
	})
}
