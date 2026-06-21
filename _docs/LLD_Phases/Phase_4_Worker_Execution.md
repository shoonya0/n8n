# Phase 4: Background Worker Execution

## Objective
Handle the asynchronous delivery of valid, deduplicated data to n8n using Asynq background workers. This adheres to `asynq-workers.md` and isolates external dependencies from blocking user-facing API calls.

## 1. Asynq Worker Handler
Write the function that actually processes the `TypeSendToN8n` job.

**File Location:** `internal/worker/handlers/n8n_handler.go`
- Parse the JSON payload from the `asynq.Task` to retrieve the `PostID`.
- Fetch the `model.Post` payload from the database using the `PostID` via the `repository`.
- **HTTP POST to n8n:**
  - Create an HTTP request to the n8n webhook URL.
  - Send the raw payload JSON.
- **Handling Outcomes:**
  - If n8n returns `200 OK`: 
    - Update the database record using `repo.UpdateStatus(ctx, PostID, "SENT")`.
    - Log success via `slog.Debug`.
    - Return `nil`.
  - If n8n returns an error or timeout:
    - Return the error to Asynq so the retry logic is automatically engaged.
    - Log error via `slog.Error`.

## 2. Retry Logic & Dead Lettering
- Rely on Asynq’s built-in exponential backoff (e.g., 5 min -> 15 min -> 30 min -> 60 min -> 24h).
- The worker configuration in `cmd/worker/main.go` must have an `ErrorHandler` capable of shifting tasks that reach maximum retries into the `dead_letter` queue.
- Monitor `asynq_dead_letter_total` Prometheus metrics.

## 3. Concurrency and Locks (Optional)
Since sending webhooks is generally idempotent (because n8n is handling new tasks), we might not explicitly require Redis locks for this specific webhook. However, if n8n workflows aren't idempotent on their side, consider adding standard distributed locks (`AcquireLock`, `ReleaseLock`) from `asynq-workers.md`.

## 4. Testing
- Create an instance of `asynq.Task` directly in unit tests and pass it to `n8n_handler.Handle()`.
- Mock out the HTTP Client to simulate n8n returning 200s and 500s.
