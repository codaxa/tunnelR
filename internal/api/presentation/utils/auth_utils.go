// Package authutils provides utility functions for HTTP request handling and authentication.
package authutils

import (
	"errors"
	"fmt"
	"net/http"

	appContext "github.com/codaxa/tunnelR.git/internal/api/app/context"
	"github.com/golang-jwt/jwt"
)

// ExtractUserID extracts the user ID from the JWT claims in the request context
func ExtractUserID(r *http.Request) (string, error) {
	ctxValue := r.Context().Value(appContext.UserClaimsKey)
	if ctxValue == nil {
		return "", errors.New("no user claims in context")
	}

	var normalizedClaims map[string]interface{}
	switch v := ctxValue.(type) {
	case *jwt.MapClaims:
		if v == nil {
			return "", errors.New("nil user claims")
		}
		normalizedClaims = *v
	case jwt.MapClaims:
		normalizedClaims = v
	default:
		return "", fmt.Errorf("unexpected claims type: %T", ctxValue)
	}

	userID, ok := normalizedClaims["user_id"].(string)
	if !ok {
		return "", errors.New("user_id not found in claims or not a string")
	}

	return userID, nil
}

// ExtractUsername extracts the username from the JWT claims in the request context
func ExtractUsername(r *http.Request) (string, error) {
	ctxValue := r.Context().Value(appContext.UserClaimsKey)
	if ctxValue == nil {
		return "", errors.New("no user claims in context")
	}

	var normalizedClaims map[string]interface{}
	switch v := ctxValue.(type) {
	case *jwt.MapClaims:
		if v == nil {
			return "", errors.New("nil user claims")
		}
		normalizedClaims = *v
	case jwt.MapClaims:
		normalizedClaims = v
	default:
		return "", fmt.Errorf("unexpected claims type: %T", ctxValue)
	}

	username, ok := normalizedClaims["username"].(string)
	if !ok {
		return "", errors.New("username not found in claims or not a string")
	}

	return username, nil
}
