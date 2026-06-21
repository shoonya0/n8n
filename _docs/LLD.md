# Low-Level Design (LLD) - Automated Social Media Posting System

## 1. Overview
This document specifies the Low-Level Design (LLD) for the automated social media posting system. It adheres strictly to the project's Golang architecture rules, utilizing Clean Architecture, the Gin web framework, PostgreSQL with `pgx/v5`, and Asynq for background job processing.

## 2. Directory Structure & Clean Architecture Layers
The business domain will reside under `internal/socialpost/`.

```
internal/socialpost/
├── handler/
│   └── post_handler.go        // Parses HTTP req, validates DTO, calls service
├── service/
│   └── post_service.go        // Deduplication logic, repository calls, task enqueuing
├── repository/
│   └── post_repo.go           // PostgreSQL queries using pgx/v5
├── dto/
│   ├── post_request.go        // Request payload with `binding:"required"` tags
│   └── post_response.go       // Response DTO
├── model/
│   └── post.go                // Domain entity representing the post
└── routes/
    └── routes.go              // Registers endpoints onto gin.RouterGroup under /api/v1
```

## 3. Database Schema
Following the `database-access.md` rule: UUID primary keys, soft deletes, and `created_at`/`updated_at` timestamps.

### Migration `migrations/000001_create_social_posts.up.sql`
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

CREATE INDEX idx_social_posts_hash ON social_posts(content_hash);
```
*(The corresponding `.down.sql` will contain `DROP TABLE IF EXISTS social_posts;`)*

## 4. API Endpoints (Gin Framework)
All routes are mounted under `/api/v1/socialpost`. The handler utilizes the `pkg/response` envelope format.

**Endpoint:** `POST /api/v1/socialpost`
**Handler Responsibility:**
1. Bind JSON to `dto.PostRequest`.
2. Validate the request payload.
3. Call `service.CreatePost(ctx, req)`.
4. Wrap success in `response.OK(c, data)` or errors in `response.FromError(c, err)`.

**Example Request:**
```json
{
  "content": "Excited to launch our new feature!",
  "platforms": ["twitter", "linkedin"]
}
```

## 5. Service Layer (Business Logic)
The service layer orchestration prevents duplicate posting and decouples the webhook triggering from the synchronous HTTP request.

**Flow:**
1. Compute `content_hash` from the incoming payload (e.g., using SHA-256).
2. Use `post_repo.go` to insert the row. If the `content_hash` already exists, PostgreSQL throws a unique constraint violation.
3. If duplicate: The service catches the constraint violation and returns a specific `errs.CodeConflict` to the handler (returning `409 Conflict` or `200 OK` depending on product requirements).
4. If unique: The record is successfully created in the database.
5. The service then **enqueues an Asynq task** to deliver the payload to n8n asynchronously.

## 6. Background Workers (Asynq)
Following `asynq-workers.md`, we use Asynq to reliably forward the data to the n8n webhook, protecting the system against n8n downtime.

**Task Definition:**
```go
// internal/worker/tasks/n8n_webhook.go
const TypeSendToN8n = "webhook:n8n_send"

type SendToN8nPayload struct {
    PostID UUID `json:"post_id"`
}
```

**Worker Handler (`internal/worker/handlers/n8n_handler.go`):**
1. Receives `TypeSendToN8n` task.
2. Fetches the post payload from the DB using `PostID`.
3. Makes an HTTP POST request to the n8n webhook URL.
4. On success: Updates the DB status to `SENT`.
5. On failure: Returns an error. Asynq automatically retries using exponential backoff (up to 5 times) before moving it to a `dead_letter` queue.

## 7. Interfaces & Dependency Injection
- `PostRepository`: Declared in `internal/socialpost/repository/` and injected into the service.
- `TaskEnqueuer`: Declared as an interface in the service and implemented by Asynq client, allowing the service to be unit-testable without a real Redis instance.

```go
type PostRepository interface {
    Create(ctx context.Context, p model.Post) (model.Post, error)
    UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
}

type TaskEnqueuer interface {
    EnqueueContext(ctx context.Context, task *asynq.Task) (*asynq.TaskInfo, error)
}
```

## 8. Summary of Benefits
- **Idempotency:** Unique DB constraint ensures duplicate payloads from Postman do not cause double-posting.
- **Resilience:** Asynq worker acts as a buffer and retry mechanism for n8n downtime.
- **Maintainability:** Clean Architecture keeps web handling, business rules, and DB persistence entirely isolated.

## 9. Implementation Phases

### Phase 1: Database Setup and Repository Layer
- **Migrations:** Create the PostgreSQL migration scripts (`up.sql` and `down.sql`) for the `social_posts` table.
- **Domain Models:** Define the `model.Post` entity.
- **Repository Implementation:** Implement `post_repo.go` with methods to create records and update their status (`PENDING` -> `SENT`), leveraging `pgx/v5`.
- **Testing:** Write integration tests against a local PostgreSQL instance.

### Phase 2: Core Service Logic & Worker Definitions
- **Task Definitions:** Define the Asynq task `TypeSendToN8n` and payload structures in `internal/worker/tasks/`.
- **Service Layer:** Implement `post_service.go` logic to calculate the payload hash, attempt insertion, handle duplicate constraint errors (`errs.CodeConflict`), and enqueue the Asynq task.
- **Testing:** Write unit tests for the service using mocked repository and Asynq interfaces.

### Phase 3: API Layer & Routing
- **Data Transfer Objects:** Define `dto.PostRequest` with required validation tags.
- **Handlers:** Implement `post_handler.go` with Gin context binding, payload validation, and standardize output using `pkg/response`.
- **Routing:** Register the `POST /api/v1/socialpost` endpoint.
- **Testing:** Test the end-to-end API logic using Postman.

### Phase 4: Background Worker Execution
- **Worker Handler:** Implement `n8n_handler.go` to process `TypeSendToN8n` jobs.
- **Integration:** Execute the outbound HTTP POST request to the n8n webhook URL.
- **State Management:** Update the database status to `SENT` upon success. Let Asynq handle retries on failure.
- **Dead Letter Handling:** Ensure jobs failing 5 times are correctly moved to dead letter for manual review.

### Phase 5: n8n Workflow Configuration
- **Webhook Setup:** Configure the n8n Webhook Node to receive incoming requests.
- **Data Mapping:** Add Code or Set nodes to format the payload appropriately for social platforms.
- **Social Media Nodes:** Connect Twitter/LinkedIn/Facebook nodes to post the content automatically.
