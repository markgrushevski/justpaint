// Package web holds shared HTTP helpers: JSON responses and the API error envelope.
package web

import (
	"encoding/json"
	"net/http"
)

// Closed v1 error-code set (docs/API.md §3).
const (
	CodeValidationFailed   = "validation_failed"
	CodeInvalidCredentials = "invalid_credentials"
	CodeUnauthorized       = "unauthorized"
	CodeForbidden          = "forbidden"
	CodeNotFound           = "not_found"
	CodeConflict           = "conflict"
	CodeDocumentTooLarge   = "document_too_large"
	CodeRateLimited        = "rate_limited"
	CodeInternal           = "internal"
)

// JSON writes v as a JSON body with the given status code.
func JSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if v != nil {
		_ = json.NewEncoder(w).Encode(v)
	}
}

type errorEnvelope struct {
	Error errorDetail `json:"error"`
}

type errorDetail struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// Error writes the standard envelope { "error": { "code", "message" } } (docs/API.md §3).
func Error(w http.ResponseWriter, status int, code, message string) {
	JSON(w, status, errorEnvelope{Error: errorDetail{Code: code, Message: message}})
}

// DecodeJSON strictly decodes a small request body into dst — it rejects unknown
// fields and caps the body at maxBytes (docs/API.md §1: auth/game bodies decode
// strictly into their small fixed shapes).
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any, maxBytes int64) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	return dec.Decode(dst)
}
