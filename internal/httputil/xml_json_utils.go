package httputil

import (
    "encoding/json"
    "encoding/xml"
    "io"
    "net/http"
    "strings"
)

// Marshal converts a struct to JSON or XML based on the Accept header
func Marshal(r *http.Request, v interface{}) ([]byte, string, error) {
    if preferXML(r) {
        data, err := xml.Marshal(v)
        return data, "application/xml", err
    }
    data, err := json.Marshal(v)
    return data, "application/json", err
}

// Unmarshal converts JSON or XML to a struct based on the Content-Type header
func Unmarshal(r *http.Request, data []byte, v interface{}) error {
    contentType := r.Header.Get("Content-Type")
    if strings.Contains(contentType, "application/xml") {
        return xml.Unmarshal(data, v)
    }
    return json.Unmarshal(data, v)
}

// Encode writes a struct as JSON or XML to the given writer based on the Accept header
func Encode(w http.ResponseWriter, r *http.Request, v interface{}) error {
    if preferXML(r) {
        w.Header().Set("Content-Type", "application/xml")
        return xml.NewEncoder(w).Encode(v)
    }
    w.Header().Set("Content-Type", "application/json")
    return json.NewEncoder(w).Encode(v)
}

// Decode reads JSON or XML from the given reader and stores it in the value pointed to by v
func Decode(r *http.Request, body io.Reader, v interface{}) error {
    contentType := r.Header.Get("Content-Type")
    if strings.Contains(contentType, "application/xml") {
        return xml.NewDecoder(body).Decode(v)
    }
    return json.NewDecoder(body).Decode(v)
}

// RespondWithData writes a success response (JSON or XML) to the http.ResponseWriter
func RespondWithData(w http.ResponseWriter, r *http.Request, code int, v interface{}) error {
    w.WriteHeader(code)
    return Encode(w, r, v)
}

// RespondWithError writes an error response (JSON or XML) to the http.ResponseWriter
func RespondWithError(w http.ResponseWriter, r *http.Request, code int, message string) error {
    w.WriteHeader(code)
    return Encode(w, r, ErrorResponse{Error: message})
}

// preferXML checks if the client prefers XML over JSON
func preferXML(r *http.Request) bool {
    accept := r.Header.Get("Accept")
    return strings.Contains(accept, "application/xml") && !strings.Contains(accept, "application/json")
}

// ErrorResponse represents an error response
type ErrorResponse struct {
    Error string `json:"error" xml:"error"`
}