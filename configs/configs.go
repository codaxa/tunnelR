// Package configs will contain configuration files
package configs

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

// Config holds the configuration settings.
type Config struct {
	Port          string
	DBUser        string
	DBPassword    string
	DBHost        string
	DBName        string
	DBPort        int
	DBUrl         string
	JWTSecret     string
	TokenDuration time.Duration
}

// New creates a new Config instance with default values.
func New() *Config {
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: Error loading .env file: %v", err)
	}

	port := os.Getenv("PORT")
	dbUser := os.Getenv("DB_USER")
	dbPassword := os.Getenv("DB_PASSWORD")
	dbHost := os.Getenv("DB_HOST")
	dbName := os.Getenv("DB_NAME")
	dbPortStr := os.Getenv("DB_PORT")
	dbPort, err := strconv.Atoi(dbPortStr)

	if err != nil {
		dbPort = 5432
		log.Printf("Warning: Invalid DB_PORT value, using default 5432: %v", err)
	}
	if port == "" {
		port = "8080"
		log.Println("Warning: Using default port. Set PORT environment variable in production.")
	}
	if dbUser == "" {
		dbUser = "postgres"
		log.Println("Warning: Using default database user. Set DB_USER environment variable in production.")
	}
	if dbPassword == "" {
		dbPassword = "postgres"
		log.Println("Warning: Using default database password. Set DB_PASSWORD environment variable in production.")
	}
	if dbHost == "" {
		dbHost = "localhost"
		log.Println("Warning: Using default database host. Set DB_HOST environment variable in production.")
	}
	if dbName == "" {
		dbName = "z-chat"
		log.Println("Warning: Using default database name. Set DB_NAME environment variable in production.")
	}

	dbURL := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		dbHost, dbPort, dbUser, dbPassword, dbName)

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		randomBytes := make([]byte, 32)
		if _, err := rand.Read(randomBytes); err != nil {
			log.Fatal("Failed to generate random JWT secret")
		}
		jwtSecret = hex.EncodeToString(randomBytes)
		log.Println("Warning: Generated random JWT secret for development. Set JWT_SECRET environment variable in production.")
	}

	return &Config{
		Port:          port,
		DBUser:        dbUser,
		DBPassword:    dbPassword,
		DBHost:        dbHost,
		DBName:        dbName,
		DBPort:        dbPort,
		DBUrl:         dbURL,
		JWTSecret:     jwtSecret,
		TokenDuration: 240 * time.Hour,
	}
}
