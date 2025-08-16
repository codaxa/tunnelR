// Package service provides business logic implementations for the application.
package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/codaxa/tunnelR.git/internal/api/core/model"
	"github.com/codaxa/tunnelR.git/internal/api/core/repository"
	"github.com/golang-jwt/jwt"
	"golang.org/x/crypto/bcrypt"
)

var (
	// ErrUsernameExists indicates a duplicate username during registration.
	ErrUsernameExists = errors.New("username already exists")
	// ErrInvalidCredentials indicates either an unknown username or wrong password.
	ErrInvalidCredentials = errors.New("invalid credentials")
)

// AuthService handles authentication operations.
type AuthService struct {
	userRepository repository.UserRepository
	jwtSecret      []byte
	tokenDuration  time.Duration
}

// NewAuthService creates and returns a new instance of AuthService.
func NewAuthService(userRepository repository.UserRepository, jwtSecret string, tokenDuration time.Duration) *AuthService {
	return &AuthService{
		userRepository: userRepository,
		jwtSecret:      []byte(jwtSecret),
		tokenDuration:  tokenDuration,
	}
}

// Register creates a new user account with the provided username, role and password.
func (s *AuthService) Register(ctx context.Context, username, password, role string) error {
	existingUser, err := s.userRepository.GetUserByUsername(ctx, username)
	if err != nil {
		return fmt.Errorf("failed to check username availability: %w", err)
	}
	if existingUser != nil {
		return ErrUsernameExists
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("failed to hash password: %w", err)
	}

	user := model.User{
		Username: username,
		Password: string(hashedPassword),
		Role:     role,
	}

	return s.userRepository.CreateUser(ctx, user)
}

// Login authenticates a user with the provided username and password.
func (s *AuthService) Login(ctx context.Context, username, password string) (string, error) {
	user, err := s.userRepository.GetUserByUsername(ctx, username)
	if err != nil {
		return "", fmt.Errorf("failed to fetch user: %w", err)
	}

	if user == nil {
		return "", ErrInvalidCredentials
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", ErrInvalidCredentials
	}

	token, err := s.generateJWT(user)
	if err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}

	return token, nil
}

func (s *AuthService) generateJWT(user *model.User) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"user_id":  user.ID,
		"username": user.Username,
		"role":     user.Role,
		"exp":      now.Add(s.tokenDuration).Unix(),
		"iat":      now.Unix(),
		"iss":      "z-chat",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.jwtSecret)
}

// ValidateToken checks the validity of a JWT token and returns the claims if valid.
func (s *AuthService) ValidateToken(tokenString string) (*jwt.MapClaims, error) {
	tokenString = strings.TrimPrefix(tokenString, "Bearer ")

	claims := jwt.MapClaims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		// Enforce HMAC and prefer HS256
		if m, ok := token.Method.(*jwt.SigningMethodHMAC); !ok || m.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})

	if err != nil || token == nil {
		return nil, fmt.Errorf("invalid token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("invalid token")
	}

	parsedClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("failed to parse token claims")
	}

	if err := parsedClaims.Valid(); err != nil {
		return nil, fmt.Errorf("failed to validate token claims: %w", err)
	}

	return &parsedClaims, nil
}
