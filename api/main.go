// It implements the main entry point for the backend server application.
package main

import (
	"fmt"
	"log"
	"net/http"

	"github.com/codaxa/tunnelR.git/configs"
	"github.com/codaxa/tunnelR.git/internal/api"
)

// main initializes and starts the backend server, setting up HTTP endpoints.
func main() {
	if err := run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run() error {
	cfg := configs.New()

	router := api.NewRouter()

	fmt.Printf("Backend server is running on port %s\n", cfg.Port)

	return http.ListenAndServe(":"+cfg.Port, router)

}
