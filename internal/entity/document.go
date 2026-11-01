package entity

import "time"

const (
	DocumentStatusProcessing = "processing"
	DocumentStatusReady      = "ready"
	DocumentStatusFailed     = "failed"
)

type Document struct {
	ID             string    `json:"id"`
	Title          string    `json:"title"`
	FileName       string    `json:"file_name"`
	StoragePath    string    `json:"-"`
	RegulationType string    `json:"regulation_type,omitempty"`
	Status         string    `json:"status"`
	ErrorMessage   string    `json:"error_message,omitempty"`
	CreatedAt      time.Time `json:"created_at"`
}
