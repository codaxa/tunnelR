// Package configs will contain configuration files
package configs

import (
	"github.com/joho/godotenv"
	"log"
	"os"
)

// Config holds the configuration settings.
type Config struct {
	Port string
}

// New creates a new Config instance with default values.
func New() *Config {
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: Error loading .env file: %v", err)
	}

	port := os.Getenv("PORT")

	if port == "" {
		port = "8080"
		log.Println("Warning: Using default port. Set PORT environment variable in production.")
	}

	return &Config{
		Port: port,
	}
}
