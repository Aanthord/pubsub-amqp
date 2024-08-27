package models

import (
    "time"
    "github.com/google/uuid"
)

type MessagePayload struct {
    ID        string                 `json:"id"`
    TraceID   string                 `json:"trace_id"`
    Sender    string                 `json:"sender"`
    Timestamp string                 `json:"timestamp"`
    Version   string                 `json:"version"`
    Content   map[string]interface{} `json:"content"`
    S3URI     string                 `json:"s3_uri,omitempty"`
    Retries   int                    `json:"retries"`
}

func NewMessagePayload(sender string, content map[string]interface{}, traceID string) *MessagePayload {
    if traceID == "" {
        traceID = uuid.New().String()
    }
    return &MessagePayload{
        ID:        uuid.New().String(),
        TraceID:   traceID,
        Sender:    sender,
        Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
        Version:   "1.0",
        Content:   content,
        Retries:   0,
    }
}

func (m *MessagePayload) IncrementRetry() {
    m.Retries++
}
