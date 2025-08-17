// Package utils provides shared utility functions used across the application.
package utils

import (
	"github.com/golang-jwt/jwt"
)

// ExtractUserRole extracts the user role from the JWT claims.
// This is used by both middleware and services to ensure consistent role checking.
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
