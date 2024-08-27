package models

import (
	"time"
)

type MessagePayload struct {
	Sender    string                 `json:"sender"`
	Timestamp string                 `json:"timestamp"`
	Version   string                 `json:"version"`
	Content   map[string]interface{} `json:"content"`
	S3URI     string                 `json:"s3_uri,omitempty"`
}

func NewMessagePayload(sender string, content map[string]interface{}) *MessagePayload {
	return &MessagePayload{
		Sender:    sender,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Version:   "1.0",
		Content:   content,
	}
}
