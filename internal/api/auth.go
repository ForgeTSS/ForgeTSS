package api

import (
	"context"
	"net/http"
	"strings"
)

// apiKeyCtxKey is the context key for the authenticated API key.
type apiKeyCtxKey struct{}

// authMiddleware checks the Authorization header for a valid bearer API key.
// It returns 401 if no key is provided or none match the configured keys.
func (s *Server) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if len(s.cfg.APIKeys) == 0 {
			// No keys configured — allow all requests (development mode).
			next.ServeHTTP(w, r)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth == "" {
			http.Error(w, "authorization required", http.StatusUnauthorized)
			return
		}

		prefix := "Bearer "
		if !strings.HasPrefix(auth, prefix) {
			http.Error(w, "invalid authorization header format", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(auth, prefix)
		valid := false
		for _, key := range s.cfg.APIKeys {
			if token == key {
				valid = true
				break
			}
		}

		if !valid {
			http.Error(w, "invalid API key", http.StatusUnauthorized)
			return
		}

		ctx := context.WithValue(r.Context(), apiKeyCtxKey{}, token)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// getAPIKey extracts the authenticated API key from the request context.
func getAPIKey(r *http.Request) string {
	key, _ := r.Context().Value(apiKeyCtxKey{}).(string)
	return key
}
