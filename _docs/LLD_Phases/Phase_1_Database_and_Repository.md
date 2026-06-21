# Phase 1: Database Setup and Repository Layer

## Objective
Establish the foundation of the project by defining the database schema and creating the repository layer to handle all PostgreSQL interactions using `pgx/v5`. This phase strictly adheres to the rules defined in `database-access.md`.

## 1. Schema & Migrations
Database changes must be versioned. You will use `golang-migrate` to create `up` and `down` migration files.

**File Location:** `migrations/000001_create_social_posts.up.sql`
```sql
CREATE TABLE IF NOT EXISTS social_posts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    content_hash VARCHAR(255) NOT NULL UNIQUE,
    payload JSONB NOT NULL,
    status VARCHAR(50) NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMP NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMP NOT NULL DEFAULT NOW(),
    deleted_at TIMESTAMP NULL
);
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_social_posts_hash ON social_posts(content_hash);
```
*Note: Make sure to also create `000001_create_social_posts.down.sql` with a simple `DROP TABLE IF EXISTS social_posts;`.*

## 2. Domain Model
Define the pure struct for the entity in the `model` package.
**File Location:** `internal/socialpost/model/post.go`
- Must include `ID uuid.UUID`, `ContentHash string`, `Payload []byte` (or mapped jsonb), `Status string`, `CreatedAt`, `UpdatedAt`, `DeletedAt`.

## 3. Repository Layer
The repository is the **only** place where database interactions (SQL) happen.
**File Location:** `internal/socialpost/repository/post_repo.go`

- **Interface Definitions:**
  ```go
  type PostRepository interface {
      Create(ctx context.Context, p model.Post) (model.Post, error)
      UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
  }
  ```
- **Implementation Rules:** 
  - The concrete `*pgxRepo` must take `*pgxpool.Pool`.
  - Use the `InTx` helper for transaction boundaries if the repository requires executing multiple queries atomically.
  - No HTTP or framework logic is allowed here.

## 4. Integration Testing
Repository tests are primarily integration tests against a real PostgreSQL database.
**File Location:** `internal/socialpost/repository/post_repo_integration_test.go`
- Use the `//go:build integration` tag.
- Spin up the database context.
- Test CRUD operations on the `social_posts` table to ensure constraints (like unique `content_hash`) work appropriately.
