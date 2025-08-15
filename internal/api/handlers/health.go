// Package handlers provides HTTP handlers for the backend application.
package handlers

import (
	"encoding/json"
	"net/http"
)

// HealthHandler responds to GET requests on the /healthz endpoint with a JSON status message.
func HealthHandler(w http.ResponseWriter, _ *http.Request) {
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
