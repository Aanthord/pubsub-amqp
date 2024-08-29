package handlers

import (
	"encoding/json"
	"encoding/xml"
	"net/http"
    "strings"
	"github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

type ErrorResponse struct {
	Error string `json:"error" xml:"error"`
}

func respondWithError(w http.ResponseWriter, r *http.Request, code int, message string) {
	span := opentracing.SpanFromContext(r.Context())
	if span != nil {
		ext.Error.Set(span, true)
		span.SetTag("http.status_code", code)
		span.SetTag("error.message", message)
	}

	if preferXML(r) {
		respondWithXML(w, r, code, ErrorResponse{Error: message})
	} else {
		respondWithJSON(w, r, code, ErrorResponse{Error: message})
	}
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

func respondWithXML(w http.ResponseWriter, r *http.Request, code int, payload interface{}) {
	span := opentracing.SpanFromContext(r.Context())
	if span != nil {
		span.SetTag("http.status_code", code)
	}

	response, _ := xml.Marshal(payload)
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(code)
	w.Write(response)
}

// preferXML checks if the client prefers XML over JSON
func preferXML(r *http.Request) bool {
	accept := r.Header.Get("Accept")
	return strings.Contains(accept, "application/xml") && !strings.Contains(accept, "application/json")
}
