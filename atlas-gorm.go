//go:build ignore
// +build ignore

package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/codaxa/tunnelR.git/configs"
	"github.com/codaxa/tunnelR.git/internal/api/core/model"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func main() {
	// Load .env file
	if err := loadEnvFile(".env"); err != nil {
		log.Printf("Warning: failed to load .env file: %v", err)
	}

	cfg := configs.New()

	// Debug: Print loaded config
	log.Printf("Config loaded - User: %s, Host: %s, DB: %s", cfg.DBUser, cfg.DBHost, cfg.DBName)

	// Use a temporary database for schema generation
	tempDB := fmt.Sprintf("%s_temp", cfg.DBName)

	// Connect to postgres database to create temp database
	adminDSN := fmt.Sprintf("host=%s user=%s password=%s dbname=postgres port=%d sslmode=disable",
		cfg.DBHost, cfg.DBUser, cfg.DBPassword, cfg.DBPort)

	adminDB, err := gorm.Open(postgres.Open(adminDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to postgres: %v", err)
	}

	// Create temporary database with proper quoting
	sqlDB, err := adminDB.DB()
	if err != nil {
		log.Fatalf("failed to get admin DB handle: %v", err)
	}
	defer sqlDB.Close()

	// Quote the database name to handle hyphens and escape quotes
	quotedTempDB := `"` + strings.ReplaceAll(tempDB, `"`, `""`) + `"`

	_, err = sqlDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s", quotedTempDB))
	if err != nil {
		log.Printf("Warning: failed to drop temp database: %v", err)
	}

	_, err = sqlDB.Exec(fmt.Sprintf("CREATE DATABASE %s", quotedTempDB))
	if err != nil {
		log.Fatalf("failed to create temp database: %v", err)
	}

	// Connect to temp database
	tempDSN := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=disable",
		cfg.DBHost, cfg.DBUser, cfg.DBPassword, tempDB, cfg.DBPort)

	log.Printf("Connecting to temporary database host=%s dbname=%s", cfg.DBHost, tempDB)

	db, err := gorm.Open(postgres.Open(tempDSN), &gorm.Config{})
	if err != nil {
		log.Fatalf("failed to connect to temp database: %v", err)
	}

	// Auto-migrate to generate schema in temp database
	// Ensure pgcrypto is available for gen_random_uuid()
	if err := db.Exec("CREATE EXTENSION IF NOT EXISTS pgcrypto").Error; err != nil {
		log.Fatalf("failed to ensure pgcrypto extension: %v", err)
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.Team{},
		&model.UserTeam{},
		&model.Machine{},
		&model.MachineTeam{},
		&model.AccessLog{},
	); err != nil {
		log.Fatalf("failed to auto-migrate: %v", err)
	}

	log.Println("GORM schema generated successfully in temporary database for Atlas")
}

// loadEnvFile loads environment variables from a .env file
func loadEnvFile(filename string) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// Remove quotes if present
		if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
			value = value[1 : len(value)-1]
		}
		if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
			value = value[1 : len(value)-1]
		}

		os.Setenv(key, value)
	}

	return scanner.Err()
}
