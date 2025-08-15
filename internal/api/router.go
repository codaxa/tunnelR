// Package api provides API layer functionalities.
package api

import (
	"github.com/codaxa/tunnelR.git/internal/api/handlers"
	"github.com/go-chi/chi"
)

// NewRouter creates a new HTTP router with the necessary routes and middleware.
func NewRouter() *chi.Mux {
	router := chi.NewRouter()

	// Public routes
	router.Get("/healthz", handlers.HealthHandler)
	router.Get("/version", handlers.VersionHandler)

	return router
}
