package models

import (
    "time"

    "github.com/google/uuid"
)

type MessagePayload struct {
    Header  HeaderData `json:"header"`
    Content Content    `json:"content"`
}

type HeaderData struct {
    ID        string `json:"id"`
    TraceID   string `json:"trace_id"`
    Sender    string `json:"sender"`
    Timestamp string `json:"timestamp"`
    Version   string `json:"version"`
    Retries   int    `json:"retries"`
    S3URI     string `json:"s3_uri,omitempty"`
}

type Content struct {
    Data map[string]interface{} `json:"data"`
}

// NewMessagePayload creates a new MessagePayload with a generated ID and timestamp.
func NewMessagePayload(sender string, content map[string]interface{}, traceID string) *MessagePayload {
    if traceID == "" {
        traceID = uuid.New().String()
    }
    return &MessagePayload{
        Header: HeaderData{
            ID:        uuid.New().String(),
            TraceID:   traceID,
            Sender:    sender,
            Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
            Version:   "1.0",
            Retries:   0,
        },
        Content: Content{
            Data: content,
        },
    }
}
