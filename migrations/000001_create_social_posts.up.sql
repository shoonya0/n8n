-- Phase 1: Create social_posts table
-- Follows database-access.md: UUID PK, created_at, updated_at, deleted_at for soft delete.
CREATE TABLE IF NOT EXISTS social_posts (
    id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    content_hash VARCHAR(255) NOT NULL UNIQUE,
    payload      JSONB        NOT NULL,
    status       VARCHAR(50)  NOT NULL DEFAULT 'PENDING',
    created_at   TIMESTAMP    NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMP    NOT NULL DEFAULT NOW(),
    deleted_at   TIMESTAMP    NULL
);

-- Separate statement so it can be run independently if needed.
-- Using CONCURRENTLY requires no lock but must be in its own transaction-less block.
CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_social_posts_hash ON social_posts(content_hash);
