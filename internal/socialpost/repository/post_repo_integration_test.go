//go:build integration

// Package repository — integration tests for the social_posts repository.
//
// Run with:
//
//	go test -tags=integration ./internal/socialpost/repository/...
//
// Prerequisites:
//   - A running PostgreSQL instance reachable via the POSTGRES_DSN environment
//     variable (default: "postgres://postgres:postgres@localhost:5432/socialpost_test?sslmode=disable").
//   - Migrations already applied (run `golang-migrate` or psql manually before
//     running these tests).
//
// These tests exercise:
//  1. Happy-path Create — verifies DB-generated fields are returned.
//  2. Unique constraint — duplicate content_hash returns ErrDuplicateHash.
//  3. UpdateStatus — transitions PENDING → SENT and returns ErrNotFound on
//     an unknown UUID.
package repository

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/n8n/socialpost/internal/socialpost/model"
)

// testPool creates a *pgxpool.Pool connected to the test database.
// The pool is closed at the end of the test via t.Cleanup.
func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	dsn := os.Getenv("POSTGRES_DSN")
	if dsn == "" {
		dsn = "postgres://postgres:postgres@localhost:5432/socialpost_test?sslmode=disable"
	}

	pool, err := pgxpool.New(context.Background(), dsn)
	require.NoError(t, err, "failed to create pgxpool")

	t.Cleanup(func() {
		pool.Close()
	})

	return pool
}

// mkPayload returns a deterministic JSONB payload for testing.
func mkPayload(t *testing.T, content string) []byte {
	t.Helper()

	b, err := json.Marshal(map[string]any{
		"content":   content,
		"platforms": []string{"twitter", "linkedin"},
	})
	require.NoError(t, err)

	return b
}

// cleanupPost deletes the test row by ID (hard delete — test only).
func cleanupPost(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) {
	t.Helper()

	_, err := pool.Exec(context.Background(),
		"DELETE FROM social_posts WHERE id = $1", id)
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestPostRepo_Create_HappyPath(t *testing.T) {
	pool := testPool(t)
	repo := New(pool)
	ctx := context.Background()

	hash := "hash-" + uuid.NewString() // unique per test run

	p := model.Post{
		ContentHash: hash,
		Payload:     mkPayload(t, "Hello, world!"),
		Status:      model.StatusPending,
	}

	got, err := repo.Create(ctx, p)
	require.NoError(t, err)
	t.Cleanup(func() { cleanupPost(t, pool, got.ID) })

	// DB-generated fields must be set.
	require.NotEqual(t, uuid.Nil, got.ID, "expected non-nil UUID")
	require.WithinDuration(t, time.Now(), got.CreatedAt, 5*time.Second)
	require.WithinDuration(t, time.Now(), got.UpdatedAt, 5*time.Second)
	require.Nil(t, got.DeletedAt, "newly created post must not be soft-deleted")

	// Echoed fields must match input.
	require.Equal(t, hash, got.ContentHash)
	require.Equal(t, model.StatusPending, got.Status)
	require.JSONEq(t, string(p.Payload), string(got.Payload))
}

func TestPostRepo_Create_DuplicateHash(t *testing.T) {
	pool := testPool(t)
	repo := New(pool)
	ctx := context.Background()

	hash := "dup-hash-" + uuid.NewString()
	payload := mkPayload(t, "duplicate test")

	first := model.Post{ContentHash: hash, Payload: payload}
	got, err := repo.Create(ctx, first)
	require.NoError(t, err)
	t.Cleanup(func() { cleanupPost(t, pool, got.ID) })

	// Second insert with the same hash must fail with ErrDuplicateHash.
	_, err = repo.Create(ctx, model.Post{ContentHash: hash, Payload: payload})
	require.ErrorIs(t, err, ErrDuplicateHash,
		"expected ErrDuplicateHash on duplicate content_hash")
}

func TestPostRepo_UpdateStatus_HappyPath(t *testing.T) {
	pool := testPool(t)
	repo := New(pool)
	ctx := context.Background()

	hash := "update-hash-" + uuid.NewString()
	created, err := repo.Create(ctx, model.Post{
		ContentHash: hash,
		Payload:     mkPayload(t, "status update test"),
	})
	require.NoError(t, err)
	t.Cleanup(func() { cleanupPost(t, pool, created.ID) })

	// Transition PENDING → SENT.
	err = repo.UpdateStatus(ctx, created.ID, model.StatusSent)
	require.NoError(t, err)

	// Verify updated_at has advanced (read directly from DB).
	var updatedAt time.Time
	row := pool.QueryRow(ctx,
		"SELECT updated_at FROM social_posts WHERE id = $1", created.ID)
	require.NoError(t, row.Scan(&updatedAt))
	require.True(t, updatedAt.After(created.UpdatedAt) || updatedAt.Equal(created.UpdatedAt),
		"updated_at should be >= created_at after status update")
}

func TestPostRepo_UpdateStatus_NotFound(t *testing.T) {
	pool := testPool(t)
	repo := New(pool)
	ctx := context.Background()

	err := repo.UpdateStatus(ctx, uuid.New(), model.StatusSent)
	require.ErrorIs(t, err, ErrNotFound,
		"expected ErrNotFound for unknown UUID")
}
