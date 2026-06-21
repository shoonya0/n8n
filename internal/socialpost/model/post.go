// Package model contains the pure domain entities for the socialpost module.
// This package must not import any other internal layer (handler, service, repository, dto).
package model

import (
	"time"

	"github.com/google/uuid"
)

// Post represents a social-media posting request persisted in the database.
// Fields map 1-to-1 with the social_posts table columns.
//
// Rules (database-access.md §3):
//   - ID is a UUID primary key — avoids sequential enumeration.
//   - CreatedAt / UpdatedAt are always set; DeletedAt enables soft delete.
//   - Payload stores the raw JSONB blob forwarded to n8n.
//   - Status transitions: PENDING → SENT (or FAILED after dead-letter).
type Post struct {
	ID          uuid.UUID  `json:"id"`
	ContentHash string     `json:"content_hash"`
	Payload     []byte     `json:"payload"` // raw JSONB; never store secrets here
	Status      string     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
	DeletedAt   *time.Time `json:"deleted_at,omitempty"` // nil → not deleted
}

// Status sentinel values.
const (
	StatusPending = "PENDING"
	StatusSent    = "SENT"
	StatusFailed  = "FAILED"
)

// WithStatus returns a new Post with Status changed to s.
// Following functional-programming.md §3: returns a new value instead of mutating.
func (p Post) WithStatus(s string) Post {
	p.Status = s
	return p
}
