# Security — n8n

> Complements `samber/cc-skills-golang@golang-security`; payments-specific rules below override anything generic.

## Rule

1. **JWT** is the only client auth mechanism. Access token TTL 15 min, refresh token TTL 30 days, refresh tokens hashed at rest and bound to a `device_id`.
2. **Webhooks** are signed and idempotent. Signature, timestamp, replay-protection all validated before any business logic runs.
3. **Secrets** are never in code or in committed files. Read via env / secret manager only.
4. **PII / payment data** is minimized in logs and DB JSON columns.
5. **Rate limiting** at the edge per LLD §6 Part 3.

## Why

- Payment apps are high-value targets. The cheap mistakes (long-lived tokens, unverified webhooks, secrets in repo) are the most exploited.
- Refresh tokens bound to a device prevent token theft from rotating into permanent access.
- Replay-protection on webhooks is the difference between "we issued 1 reward" and "we issued 50 rewards because the attacker re-sent the success webhook".

## How to apply

### 1. JWT

- HS256 with separate secrets for access vs. refresh.
- Access token claims: `sub` (user_id), `device_id`, `iat`, `exp`. Nothing else.
- Refresh token: random opaque string, **hashed (bcrypt or sha256-hmac)** in `user_sessions.refresh_token`. Plaintext only on the wire and in the client.
- `Authorization: Bearer <token>` header.
- JWT middleware validates: signature, expiry, user status (not suspended), device_id binding to session.

User must re-login when:

- Device changes (different `device_id` on the request).
- Refresh token is revoked or expired.
- Device row is deleted.

### 2. Webhooks — `/webhooks/zuelpay`

Mandatory checks in order, before any DB write:

1. **Signature verification** — HMAC-SHA256 of the raw body with `ZUELPAY_WEBHOOK_SECRET`, compared in constant time (`subtle.ConstantTimeCompare`).
2. **Timestamp validation** — header `X-Webhook-Timestamp` must be within ±5 min of now.
3. **Replay protection** — `webhook_event_id` stored in Redis with 24h TTL. Duplicate → 200 OK with `"status":"duplicate"` (idempotent ack).
4. **Source validation** — request must come from ZuelPay's IP range (configurable allow-list).

Only after all four pass does the controller call the service.

### 3. Idempotency

Every payment request includes `X-Idempotency-Key` (UUID from app). Middleware in `pkg/idempotency`:

- Key = `hash(idempotency_key, user_id, route)`.
- Redis lookup, 24h TTL.
- Hit → return cached response.
- Miss → process, cache response (with status), return.

### 4. Rate limiting (LLD §6 Part 3)

Implemented in `internal/middleware` using a Redis token bucket keyed by `rate_limit:{user_id}:{endpoint}` (or `:{ip}` for unauthenticated):

| Group | Limit |
| --- | --- |
| OTP APIs | 5 / hour |
| Login APIs | 10 / hour |
| Payment APIs | 100 / day |
| Support APIs | 50 / day |

Exceeding the limit → `429 Too Many Requests` with `error_code=RATE_LIMITED` and a `Retry-After` header.

### 5. Secrets

- Local dev: `.env` (gitignored).
- Production: AWS Secrets Manager (or SSM Parameter Store) injected at task start via environment.
- Never `git add` a file containing `_SECRET`, `_KEY`, `password=`, JWT samples, FCM service account JSON. Add to `.gitignore` proactively.
- Rotation: a secret rotation does not require code changes — the loader re-reads on next deploy.

### 6. Input validation

- Every request DTO has `binding`/`validate` tags. Numeric fields capped (max amount, max length).
- Phone numbers normalized to E.164 in the auth module.
- Amounts validated server-side, **never** trust the client's "computed total".

### 7. Output and error hygiene

- `pkg/response.FromError` never echoes `err.Error()`. Public message comes from a static map keyed by error code.
- 500s log the full error to slog (Sentry captures); the client gets a generic message.
- Stack traces are forbidden in HTTP responses.

### 8. SQL

- No string concatenation for queries. Parameterized only.
- `pgx` handles this naturally; if you ever reach for `fmt.Sprintf` to build SQL, stop.
- `LIKE` patterns from user input: escape `%` and `_` if user input is the literal.

### 9. Cryptography

- `crypto/rand` for any randomness related to security (OTPs, tokens, idempotency keys generated server-side, session ids). `math/rand` is forbidden here.
- Passwords are not stored in this codebase (user auth is OTP-only; admin auth lives in the separate admin service). If a future password column is ever added, hash with `argon2id` via `golang.org/x/crypto/argon2` — not raw bcrypt.
- TLS: terminated at the load balancer (ALB). Internal traffic is plain HTTP within the VPC.

### 10. Audit log everything that matters

Audit logging in this codebase is limited to `transaction_logs` (every state transition) and `webhook_events` (every inbound webhook). System-wide audit of administrative actions lives in the separate admin service's own audit log; when that service calls our `/internal/...` endpoints, it sends `X-Admin-Actor-Id` + `X-Admin-Action` headers, and we attach them to the resulting `transaction_logs` row (`event_source='internal'`).

### 11. Run `govulncheck` in CI

`govulncheck ./...` on every PR. Block merges on known vulnerable dependencies. See `golang-security` + `golang-dependency-management` skills.
