package handlers

import (
    "encoding/json"
    "net/http"

    "github.com/opentracing/opentracing-go"
    "github.com/opentracing/opentracing-go/ext"
)

type ErrorResponse struct {
    Error string `json:"error"`
}

func respondWithError(w http.ResponseWriter, r *http.Request, code int, message string) {
    span := opentracing.SpanFromContext(r.Context())
    if span != nil {
        ext.Error.Set(span, true)
        span.SetTag("http.status_code", code)
        span.SetTag("error.message", message)
    }

    respondWithJSON(w, r, code, ErrorResponse{Error: message})
}

func respondWithJSON(w http.ResponseWriter, r *http.Request, code int, payload interface{}) {
    span := opentracing.SpanFromContext(r.Context())
    if span != nil {
        span.SetTag("http.status_code", code)
    }

    response, _ := json.Marshal(payload)
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    w.Write(response)
}