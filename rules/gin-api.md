# Gin API conventions — n8n

## Rule

Gin is the **only** HTTP framework. Use it consistently:

1. One router group per module, mounted under `/api/v1`.
2. Handlers are thin — parse, validate, call service, return.
3. Middleware order is fixed: `RequestID → Logger → Recovery → RateLimiter → JWT → Device → handler`.
4. Responses always go through the `pkg/response` envelope.

## Why

- Consistent middleware order is what makes logs and rate limits trustworthy. A handler that runs before the logger is invisible in production.
- A standard response envelope lets the Flutter app share a single decode path; ad-hoc shapes break the client.
- Versioning at the path (`/api/v1`) lets us ship `/v2` without breaking the app.

## How to apply

### Route registration

Each module exposes a `RegisterRoutes` function. `cmd/api/main.go` calls each one.

```go
// internal/payments/routes/routes.go
package routes

func RegisterRoutes(rg *gin.RouterGroup, h *handler.PayHandler) {
    p := rg.Group("/payments")
    p.POST("",          h.Pay)
    p.GET("/:id",       h.Get)
    p.GET("/:id/status", h.Status)
}

// cmd/api/main.go
v1 := r.Group("/api/v1")
payments.RegisterRoutes(v1, paymentHandler)
bbps.RegisterRoutes(v1, bbpsHandler)
// …
```

### Handler template

```go
func (h *PayHandler) Pay(c *gin.Context) {
    var req dto.PayRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        response.BadRequest(c, errs.CodeValidation, err)
        return
    }
    if err := h.validator.Validate(req); err != nil {
        response.BadRequest(c, errs.CodeValidation, err)
        return
    }
    ctx := logger.WithRequestID(c.Request.Context(), c.GetString("request_id"))
    out, err := h.svc.Pay(ctx, req, c.GetString("user_id"))
    if err != nil {
        response.FromError(c, err)
        return
    }
    response.OK(c, out)
}
```

Things the handler **does not do**:

- Build SQL strings.
- Log business events (the service does, with `slog.InfoContext(ctx, …)`).
- Call HTTP clients directly.
- Compose multiple services into a flow — that's the service's job.

### Middleware order — the canonical chain

```go
r := gin.New()
r.Use(middleware.RequestID())   // generates REQ-XXXXXXXX, sets c.Set + response header
r.Use(middleware.Logger())      // slog access log with request_id, user_id, latency_ms
r.Use(middleware.Recovery())    // recovers panics, logs, returns 500
r.Use(middleware.RateLimiter()) // token bucket in Redis, key = user|ip + endpoint

v1 := r.Group("/api/v1")
v1.Use(middleware.JWT())        // validates access token, sets user_id on context
v1.Use(middleware.Device())     // validates registered device + session
```

Public routes (e.g. send OTP, login) mount **before** `JWT()`, on a separate group.

### Validation

Use `binding:"required"` plus `validate:"…"` tags. Centralize message formatting in `internal/validator/`. Never echo raw validator output in production (it can leak field names — fine here, just be intentional).

### Response envelope

All non-error responses:

```json
{ "success": true, "data": { … }, "request_id": "REQ-…" }
```

All errors:

```json
{ "success": false, "message": "Validation failed", "error_code": "VALIDATION_ERROR", "request_id": "REQ-…" }
```

`pkg/response.OK(c, data)`, `pkg/response.BadRequest(c, code, err)`, `pkg/response.FromError(c, err)` (auto-maps oops error codes/HTTP status). HTTP status codes per LLD §16:

| Category | Status |
| --- | --- |
| Validation | 400 |
| Authentication | 401 |
| Authorization | 403 |
| Resource missing | 404 |
| Business rule | 422 |
| Internal | 500 |

### API versioning

Current: `/api/v1`. New endpoints land in `/v1` until a breaking change is needed; then they move to `/v2`. **Never** modify an existing `/v1` contract — add a field nullably or ship `/v2`.

### Idempotency

Payment endpoints require `X-Idempotency-Key` (a UUID from the app). `pkg/idempotency` provides middleware that:

1. Hashes (key, user_id, route) and looks up Redis (24h TTL).
2. If hit, returns the cached response with the same status code.
3. If miss, runs the handler, then caches the response.

If the client retries with the same key, they get the same response — no duplicate payments.
