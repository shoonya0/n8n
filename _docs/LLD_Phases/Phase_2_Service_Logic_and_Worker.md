# Phase 2: Core Service Logic & Worker Definitions

## Objective
Implement the business logic orchestration. The service layer is responsible for deduplication logic, interacting with the repository, and enqueuing background tasks to `Asynq`. This follows the rules established in `clean-architecture.md` and `asynq-workers.md`.

## 1. Task Definitions (Asynq)
Define the background task payload that the worker will eventually execute. This lives outside the specific domain handler since it's worker-related.

**File Location:** `internal/worker/tasks/n8n_webhook.go`
```go
const TypeSendToN8n = "webhook:n8n_send"

type SendToN8nPayload struct {
    PostID uuid.UUID `json:"post_id"`
}

func NewSendToN8nTask(postID uuid.UUID) (*asynq.Task, error) {
    p, err := json.Marshal(SendToN8nPayload{PostID: postID})
    if err != nil { return nil, oops.Wrap(err) }
    // Max retry set to 5 as per rules for standard webhook delivery
    return asynq.NewTask(TypeSendToN8n, p, asynq.Queue("default"), asynq.MaxRetry(5)), nil
}
```

## 2. Service Layer Implementation
The service performs the heavy lifting and links the database to the background tasks.

**File Location:** `internal/socialpost/service/post_service.go`
- **Dependencies:** Needs `repository.PostRepository` and an `asynq.Client` interface wrapper (`TaskEnqueuer`).
- **Core Workflow (`CreatePost` method):**
  1. Hash the incoming payload JSON using SHA-256.
  2. Map the payload to the `model.Post` struct.
  3. Call `repo.Create(ctx, post)`.
  4. If the repository returns a unique constraint error (duplicate hash):
     - Log it using `slog.InfoContext` as a duplicate.
     - Return an `errs.CodeConflict` to safely tell the handler to return an idempotent success or HTTP 409, depending on product rules.
  5. If the creation succeeds:
     - Generate a new `SendToN8nTask`.
     - Enqueue the task using `asynq.Client.EnqueueContext()`.

## 3. Unit Testing
- The service must be unit-testable without spinning up PostgreSQL or Redis.
- **File Location:** `internal/socialpost/service/post_service_test.go`
- Use mocking libraries (like `gomock` or standard struct mocks) for `PostRepository` and `TaskEnqueuer`.
- Ensure tests cover both the "successful new post" path and the "duplicate post conflict" path.
