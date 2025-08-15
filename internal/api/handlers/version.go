// Package handlers provides HTTP handlers for the backend application.
package handlers

import (
	"encoding/json"
	"net/http"
)

// VersionHandler responds to GET requests on the /version endpoint with a JSON version number.
func VersionHandler(w http.ResponseWriter, _ *http.Request) {
	if err := json.NewEncoder(w).Encode(map[string]string{"version": "0.1.0"}); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
}
