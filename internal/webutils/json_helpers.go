package webutils

import (
	"encoding/json"
	"net/http"
)

type jsonError struct {
	Error string `json:"error"`
}

func WriteJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data) //nolint:errcheck
}

// RawJSON writes a pre-encoded JSON byte slice directly - used for cache hits to avoid re-encoding.
func RawJSON(w http.ResponseWriter, status int, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data) //nolint:errcheck
}

func ErrorJSON(w http.ResponseWriter, err error, status int) {
	WriteJSON(w, status, jsonError{Error: err.Error()})
}

func ReadJSON(r *http.Request, dst any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dst)
}
