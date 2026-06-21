# slog logging — n8n

> **Supersedes** `samber/cc-skills-golang@golang-samber-slog` for n8n projects. We use stdlib `log/slog`, not `samber/slog`. The samber wrappers (`samber/slog-gin`, `samber/slog-multi`) are not used.

## Rule

1. Logger is stdlib `log/slog`.
2. Output is JSON in non-development (`slog.NewJSONHandler`); text in development.
3. **Every log line carries `request_id`** (and `user_id` when known). They come from `context.Context`, not from `*gin.Context` outside the handler.
4. Use the `Context`-aware methods: `slog.InfoContext(ctx, …)`, not `slog.Info(…)`.
5. Never log secrets, OTPs, JWT tokens, full webhook payloads, plain card/UPI PIN, or any PII beyond `user_id`.

## Why

- `request_id` is what lets ops trace a 500 across handler → service → repository → DB → ZuelPay. Logs without it are a graveyard.
- JSON in production is non-negotiable — **Vector** parses JSON keys and forwards them to Loki. The app must not import a Loki client; writing directly to Loki couples request latency to Loki's ingest health. Vector decouples them by buffering, batching, and retrying on the host.
- A leaked OTP in logs is a P0; a single careless `slog.Info("got otp", "otp", otp)` is enough to fail audit.

## How to apply

### 1. Setup in `cmd/api/main.go`

```go
var handler slog.Handler
if cfg.App.Env == "development" {
    handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: cfg.LogLevel()})
} else {
    handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
        Level: cfg.LogLevel(),
        ReplaceAttr: logger.RedactSensitive,
    })
}
slog.SetDefault(slog.New(handler))
```

`logger.RedactSensitive` replaces values of attribute keys in a denylist (`password`, `otp`, `access_token`, `refresh_token`, `pin`, `secret`, `authorization`) with `"[REDACTED]"`. This is **defense in depth**; the primary line is to not log those keys at all.

### 2. Context propagation

The logger middleware stores `request_id` in `context.Context` and on `*gin.Context`. Inside services, retrieve from `ctx` and log:

```go
slog.InfoContext(ctx, "payment created",
    "request_id", logger.RequestID(ctx),
    "user_id",    logger.UserID(ctx),
    "txn_id",     txn.ID,
    "amount",     txn.Amount,
    "method",     txn.PaymentMethod,
)
```

A cleaner pattern — bind the logger to ctx once:

```go
log := slog.With(
    "request_id", logger.RequestID(ctx),
    "user_id",    logger.UserID(ctx),
)
log.InfoContext(ctx, "payment created", "txn_id", txn.ID)
```

### 3. Levels — keep them honest

| Level | Use for |
| --- | --- |
| `Debug` | Local development; verbose internal state. Disabled in prod by default. |
| `Info`  | Successful business events (txn created, OTP sent, webhook processed). |
| `Warn`  | Recoverable anomalies (retry succeeded after failure, rate limit triggered, fallback used). |
| `Error` | Operation failed in a way the user notices. Always include the error attribute. |

**Do not** use `Error` for expected business failures (e.g. user entered wrong OTP) — that is `Info` with `outcome=failure`.

### 4. The mandatory request log fields (LLD §16)

Every request emits one access log with at minimum:

```
request_id, user_id, endpoint, method, status, latency_ms, ip, device_id
```

This is the logger middleware's job; module code does not duplicate it.

### 5. Errors

Pass the error as an attribute, not in the message string:

```go
// ✓ Good — structured, greppable, redactable
slog.ErrorContext(ctx, "zuelpay pay call failed", "err", err, "txn_id", txn.ID)

// ✗ Bad — error vanishes into a string
slog.Error(fmt.Sprintf("zuelpay failed for %s: %v", txn.ID, err))
```

When using `samber/oops`, log the error directly — `oops` errors expose `slog.Attr` through their `LogValuer` so structured fields propagate.

### 6. What never goes in logs

- OTP digits, even partial
- JWT access/refresh tokens, even partial
- Full request/response bodies of payment APIs (log shape + IDs, not amounts unless masked, never PINs)
- Card numbers, UPI PINs, raw `Authorization` headers
- Phone numbers in full — last-4 only if needed
- ZuelPay webhook raw payload — log the parsed `webhook_event_id`, `status`, `transaction_id`

Add to the denylist before merging if you find a new sensitive key.

### 7. Sampling and rate

Asynq worker logs at `Info` for start/finish of each task. If a queue is processing 1000s of tasks/sec, switch its task handler to `Debug` for the success path and `Warn` on retry-needed.

### 8. Logging vs. metrics vs. traces

- **Log** what happened with context for forensics.
- **Metric** count it (Prometheus) for dashboards.
- **Trace** the request span (future, when OpenTelemetry is added).

A useful production rule: if you find yourself grepping logs to compute a percentage, that should be a metric. See `golang-observability` skill.
