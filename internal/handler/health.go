package handler

import (
	"encoding/json"
	"net/http"
	"time"
)

// healthResponse is the JSON response for the health check endpoint.
type healthResponse struct {
	Status    string `json:"status"`
	Timestamp string `json:"timestamp"`
}

// HandleHealth handles GET /health.
// Returns a simple JSON response indicating the server is running.
func HandleHealth(w http.ResponseWriter, r *http.Request) {
	resp := healthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}
