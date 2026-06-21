# Error handling — n8n

> Supersedes `samber/cc-skills-golang@golang-error-handling` and complements `@golang-samber-oops` for n8n.

## Rule

1. Errors are **values**. Return, wrap with `samber/oops`, never panic in business code.
2. Every error eventually maps to one of the project's error codes in `pkg/errs` (`INVALID_OTP`, `TOKEN_EXPIRED`, `UPI_NOT_FOUND`, `PAYMENT_FAILED`, `BILL_FETCH_FAILED`, `LIMIT_EXCEEDED`, …).
3. HTTP layer translates the error into the standard envelope (`pkg/response.FromError`).
4. **Panics** are recovered only by the recovery middleware. Anything that panics is a bug.

## Why

- A consistent error code lets the Flutter app render the right localized message and the right CTA ("retry", "re-login", "contact support").
- Wrapping adds stack + tags + context so a single log line in production tells the on-caller where it broke.
- Distinguishing "infrastructure failure" from "business rule" is what makes monitoring usable — only one of those should page.

## How to apply

### 1. Define error codes once, in `pkg/errs`

```go
package errs

// CodeXxx — exported constants. Stable; consumed by Flutter.
const (
    CodeValidation       = "VALIDATION_ERROR"
    CodeAuthentication   = "AUTH_ERROR"
    CodeAuthorization    = "FORBIDDEN"
    CodeNotFound         = "NOT_FOUND"
    CodeInvalidOTP       = "INVALID_OTP"
    CodeTokenExpired     = "TOKEN_EXPIRED"
    CodeUPINotFound      = "UPI_NOT_FOUND"
    CodePaymentFailed    = "PAYMENT_FAILED"
    CodeBillFetchFailed  = "BILL_FETCH_FAILED"
    CodeLimitExceeded    = "LIMIT_EXCEEDED"
    CodeIdempotencyConf  = "IDEMPOTENCY_CONFLICT"
    CodeInternal         = "INTERNAL_ERROR"
)

// HTTPStatus maps a code to its HTTP status.
func HTTPStatus(code string) int { … }
```

### 2. Wrap with `samber/oops` at the boundary you raise from

```go
import "github.com/samber/oops"

func (r *TxnRepo) FindByID(ctx context.Context, id uuid.UUID) (model.Transaction, error) {
    var t model.Transaction
    err := r.db.QueryRow(ctx, q, id).Scan(&t.ID, &t.Amount, …)
    if errors.Is(err, pgx.ErrNoRows) {
        return t, oops.Code(errs.CodeNotFound).In("payments.repo").With("txn_id", id).Errorf("transaction not found")
    }
    if err != nil {
        return t, oops.Code(errs.CodeInternal).In("payments.repo").Wrapf(err, "find txn %s", id)
    }
    return t, nil
}
```

The service layer can re-wrap with additional context as the error bubbles up, but should not change the `Code` unless it is genuinely a different category.

### 3. Map to HTTP at the edge — `pkg/response.FromError`

```go
func FromError(c *gin.Context, err error) {
    code := oops.AsOops(err).Code()
    if code == "" { code = errs.CodeInternal }
    status := errs.HTTPStatus(code)
    c.JSON(status, gin.H{
        "success":    false,
        "error_code": code,
        "message":    publicMessage(code),  // never err.Error() — never leak internals
        "request_id": c.GetString("request_id"),
    })
    if status >= 500 {
        slog.ErrorContext(c.Request.Context(), "request failed", "err", err)
    }
}
```

**Critical:** `publicMessage(code)` is a static map. Never echo raw `err.Error()` in a response. Stack traces in HTTP responses is the most common Go security finding in audits.

### 4. Business outcomes vs. infrastructure errors

| Kind | Example | Where it ends |
| --- | --- | --- |
| **Business outcome** — expected, often the user's fault | wrong OTP, insufficient balance, limit exceeded | `Info`/`Warn` log, 4xx response, no page |
| **Infrastructure failure** — unexpected | DB down, ZuelPay 5xx, deserialization bug | `Error` log, 5xx response, Sentry, alert |

Use `oops.Code(…)` consistently to keep these separate. Sentry captures only `errs.CodeInternal` (and other 5xx codes) by default.

### 5. Errors as values composition — `mo.Result[T]`

For pipelines where multiple steps can fail with structured outcomes:

```go
result := mo.Ok(input).
    FlatMap(s.validate).
    FlatMap(s.acquireLock).
    FlatMap(s.callZuelPay).
    FlatMap(s.persist)

if result.IsError() { return result.Error() }
return result.MustGet()
```

This linearizes a 5-deep `if err != nil` chain.

### 6. Panics

Panics are reserved for:

- Programmer errors that should crash a goroutine (impossible code paths, invariant violations during startup).
- Third-party libraries that panic on their own (e.g. some DB drivers in edge cases).

The recovery middleware catches **request-scoped** panics and returns `500 INTERNAL_ERROR`. Worker panics are caught by Asynq's recovery and the task is sent to retry/dead-letter.

**Never** `panic()` from a handler or service to signal a user-facing error. Use `oops.Code(…)` and return.

### 7. Retries — only retry idempotent operations

The repository / client signals whether a failure is retriable via the error code. The caller (worker, retry loop) inspects:

```go
if errs.Retriable(err) { … }   // e.g. CodePaymentFailed-Pending, network blips
```

ZuelPay payment status checks are retriable; the initial pay-call is not (idempotency is enforced via `X-Idempotency-Key` at ZuelPay's side too — see `rules/security.md`).
