// Package entities contains the core domain types shared across layers.
package entities

import "time"

// Job represents a single HLS-to-MP4 download task.
type Job struct {
	ID        string    `json:"id"`
	URL       string    `json:"url"`
	Filename  string    `json:"filename"`
	Status    string    `json:"status"` // queued, running, done, error, cancelled
	Error     string    `json:"error,omitempty"`
	StartedAt time.Time `json:"started_at"`
	EndedAt   time.Time `json:"ended_at,omitempty"`
}
