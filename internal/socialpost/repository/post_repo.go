// Package repository is the ONLY place in the socialpost module that talks to
// the database. Handlers and services must not import pgx, sql, or any package
// that constructs SQL strings. (database-access.md §1)
package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/n8n/socialpost/internal/socialpost/model"
)

// PostRepository is the narrow interface the service layer depends on.
// It is defined here (in the provider package) so that consumers can declare
// a still-narrower subset on their side (clean-architecture.md §"Cross-module calls").
//
// All methods accept a context.Context for cancellation / tracing propagation.
type PostRepository interface {
	// Create inserts a new Post row and returns the persisted record (with DB-
	// generated ID, CreatedAt, UpdatedAt).
	Create(ctx context.Context, p model.Post) (model.Post, error)

	// UpdateStatus changes the status of an existing post identified by id.
	// Returns ErrNotFound if the row does not exist or has been soft-deleted.
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
}

// ErrNotFound is returned when a row is not found or has been soft-deleted.
var ErrNotFound = errors.New("social_post: record not found")

// ErrDuplicateHash is returned when content_hash violates the unique constraint.
var ErrDuplicateHash = errors.New("social_post: duplicate content_hash")

// pgxRepo is the concrete PostgreSQL implementation of PostRepository.
// It takes a *pgxpool.Pool so the pool is created once at startup and shared
// (database-access.md §4).
type pgxRepo struct {
	pool *pgxpool.Pool
}

// New creates a new PostRepository backed by the given connection pool.
//
// The pool is expected to be fully configured (MaxConns, MinConns, ConnMaxLifetime)
// by the composition root (cmd/api/main.go) before being passed here.
func New(pool *pgxpool.Pool) PostRepository {
	return &pgxRepo{pool: pool}
}

// Create inserts a social_post row and returns the full persisted record.
//
// If the content_hash already exists the PostgreSQL unique-constraint error is
// translated into the sentinel ErrDuplicateHash so callers can distinguish
// duplicates from generic infrastructure failures.
func (r *pgxRepo) Create(ctx context.Context, p model.Post) (model.Post, error) {
	const q = `
		INSERT INTO social_posts (content_hash, payload, status)
		VALUES ($1, $2, $3)
		RETURNING id, content_hash, payload, status, created_at, updated_at, deleted_at`

	status := p.Status
	if status == "" {
		status = model.StatusPending
	}

	row := r.pool.QueryRow(ctx, q, p.ContentHash, p.Payload, status)

	out, err := scanPost(row)
	if err != nil {
		if isUniqueViolation(err) {
			return model.Post{}, ErrDuplicateHash
		}
		return model.Post{}, fmt.Errorf("repository.Create: %w", err)
	}

	return out, nil
}

// UpdateStatus sets the status column of a post to the provided value and
// refreshes updated_at. Rows that have been soft-deleted are excluded.
func (r *pgxRepo) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	const q = `
		UPDATE social_posts
		SET    status = $2, updated_at = $3
		WHERE  id = $1
		  AND  deleted_at IS NULL`

	tag, err := r.pool.Exec(ctx, q, id, status, time.Now().UTC())
	if err != nil {
		return fmt.Errorf("repository.UpdateStatus: %w", err)
	}

	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}

	return nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

// scanPost reads one row from the RETURNING / SELECT clause into a model.Post.
// It handles the nullable deleted_at column.
func scanPost(row pgx.Row) (model.Post, error) {
	var p model.Post
	var deletedAt *time.Time

	err := row.Scan(
		&p.ID,
		&p.ContentHash,
		&p.Payload,
		&p.Status,
		&p.CreatedAt,
		&p.UpdatedAt,
		&deletedAt,
	)
	if err != nil {
		return model.Post{}, err
	}

	p.DeletedAt = deletedAt

	return p, nil
}

// isUniqueViolation reports whether err is a PostgreSQL unique-constraint
// violation (SQLSTATE 23505).
func isUniqueViolation(err error) bool {
	// pgx v5 wraps constraint errors as *pgconn.PgError.
	// We use errors.As so intermediate wrapping is transparent.
	type pgErr interface{ SQLState() string }
	var pg pgErr
	if errors.As(err, &pg) {
		return pg.SQLState() == "23505"
	}
	return false
}
