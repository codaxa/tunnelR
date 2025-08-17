// Package middleware provides HTTP middleware functionalities for the backend application.
package middleware

import (
	"context"
	"net/http"

	appContext "github.com/codaxa/tunnelR.git/internal/api/app/context"
	"github.com/codaxa/tunnelR.git/internal/api/app/service"
	"github.com/codaxa/tunnelR.git/internal/api/core/model"
	"github.com/golang-jwt/jwt"
)

// AuthMiddleware holds the auth service dependency
type AuthMiddleware struct {
	authService *service.AuthService
}

// NewAuthMiddleware creates a new auth middleware with the provided auth service
func NewAuthMiddleware(authService *service.AuthService) *AuthMiddleware {
	return &AuthMiddleware{
		authService: authService,
	}
}

// Authenticate checks the request for a valid JWT token and extracts user claims
func (am *AuthMiddleware) Authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		claims, err := am.authService.ValidateToken(authHeader)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		r = r.WithContext(context.WithValue(r.Context(), appContext.UserClaimsKey, claims))
		next.ServeHTTP(w, r)
	})
}

// AdminAuthenticate checks the request for a valid JWT token, checks of the user is admin and extracts user claims
func (am *AuthMiddleware) AdminAuthenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		claims, err := am.authService.ValidateToken(authHeader)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		role := extractUserRole(claims)
		if role != model.RoleAdmin {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		r = r.WithContext(context.WithValue(r.Context(), appContext.UserClaimsKey, claims))
		next.ServeHTTP(w, r)
	})
}

// ExtractUserRole extracts the user role from the JWT claims in the request context.
// Exported so it can be used by other packages
func ExtractUserRole(claims *jwt.MapClaims) (role string) {
	if claims == nil {
		return ""
	}
	claimsMap := map[string]interface{}(*claims)
	if userRoleVal, exists := claimsMap["role"]; exists {
		if userRoleStr, ok := userRoleVal.(string); ok {
			role = userRoleStr
		}
	}
	return role
}

// extractUserRole extracts the user role from the JWT claims in the request context.
func extractUserRole(claims *jwt.MapClaims) (role string) {
	return ExtractUserRole(claims)
}
