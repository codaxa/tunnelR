// Package service provides business logic implementations for the application.
package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/codaxa/tunnelR.git/internal/api/core/model"
	"github.com/codaxa/tunnelR.git/internal/api/core/repository"
	"github.com/golang-jwt/jwt"
	"golang.org/x/crypto/bcrypt"
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
		return fmt.Errorf("username already exists")
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
		return "", fmt.Errorf("invalid username")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		return "", fmt.Errorf("invalid password")
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

	token, err := jwt.Parse(tokenString, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return s.jwtSecret, nil
	})

	if token == nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)

	if !ok {
		return nil, fmt.Errorf("failed to parse token claims")
	}

	err = claims.Valid()
	if err != nil {
		return nil, fmt.Errorf("failed to parse token claims: %w", err)
	}

	return &claims, nil
}
