package handlers

import (
    "encoding/json"
    "net/http"
    "github.com/opentracing/opentracing-go"
    "github.com/opentracing/opentracing-go/ext"
)

// ErrorResponse is a common structure for error responses.
type ErrorResponse struct {
    Error string `json:"error"`
}

// respondWithError sends an error response in JSON format.
func respondWithError(w http.ResponseWriter, r *http.Request, code int, message string) {
    span := opentracing.SpanFromContext(r.Context())
    if span != nil {
        ext.Error.Set(span, true)
        span.SetTag("http.status_code", code)
        span.SetTag("error.message", message)
    }

    respondWithJSON(w, code, ErrorResponse{Error: message})
}

// respondWithJSON sends a JSON response.
func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
    response, _ := json.Marshal(payload)
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    w.Write(response)
}

func respondWithSuccess(w http.ResponseWriter, r *http.Request, statusCode int, message string) {
    response := SuccessResponse{
        Message: message,
    }

    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(statusCode)
    if err := json.NewEncoder(w).Encode(response); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
    }
}