// Package main implements the main entry point for the backend server application.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/codaxa/tunnelR.git/configs"
	"github.com/codaxa/tunnelR.git/internal/api/app/service"
	"github.com/codaxa/tunnelR.git/internal/api/infrastructure/repository"
	apirouter "github.com/codaxa/tunnelR.git/internal/api/presentation"
	"github.com/jackc/pgx/v5/pgxpool"
)

// main initializes and starts the backend server, setting up HTTP endpoints.
func main() {
	if err := run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run() error {
	cfg := configs.New()

	// Initialize database connection
	connConfig, err := pgxpool.ParseConfig(cfg.DBUrl)
	if err != nil {
		log.Fatalf("Failed to parse database configuration: %v", err)
	}

	dbConn, err := pgxpool.NewWithConfig(context.Background(), connConfig)
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer dbConn.Close()
	if err := dbConn.Ping(context.Background()); err != nil {
		return fmt.Errorf("failed to ping database: %v", err)
	}

	// Initialize repositories
	userRepo := repository.NewUserRepository(dbConn)
	teamRepo := repository.NewTeamRepository(dbConn)
	machineRepo := repository.NewMachineRepository(dbConn)

	// Initialize services
	authService := service.NewAuthService(userRepo, cfg.JWTSecret, cfg.TokenDuration)
	teamService := service.NewTeamService(teamRepo, userRepo)
	machineService := service.NewMachineService(teamRepo, machineRepo)

	router := apirouter.NewRouter(authService, teamService, machineService)

	fmt.Printf("Backend server is running on port %s\n", cfg.Port)

	return http.ListenAndServe(":"+cfg.Port, router)
}
