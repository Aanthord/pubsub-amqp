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

// respondWithXML encodes the response as XML
func respondWithXML(w http.ResponseWriter, code int, payload interface{}) {
    response, _ := mxj.AnyXmlIndent(payload, "", "  ")
    w.Header().Set("Content-Type", "application/xml")
    w.WriteHeader(code)
    w.Write(response)
}



// respondWithError sends an error response
func respondWithError(w http.ResponseWriter, r *http.Request, code int, message string) {
    respondWithJSON(w, code, map[string]string{"error": message})
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

// respondWithJSON encodes the response as JSON
func respondWithJSON(w http.ResponseWriter, code int, payload interface{}) {
    response, _ := json.Marshal(payload)
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(code)
    w.Write(response)
}