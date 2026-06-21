# Clean architecture — n8n

> Supersedes `samber/cc-skills-golang@golang-project-layout` for n8n.

## Rule

Every business module under `internal/<module>/` follows the same four-layer flow:

```
Handler → Service → Repository → Database
```

**Dependency rule:** upper layers depend on lower layers, never the reverse. A repository never imports a service; a service never imports a handler; nothing imports `gin` outside `handler/`, `middleware/`, and `routes/`.

## Why

- Swapping the HTTP framework (or adding gRPC) should not ripple into services.
- Services must be unit-testable without spinning up Gin or a DB.
- Payments code lives forever; the cost of a leaky abstraction here is measured in incidents and audits.

## How to apply

### Layer responsibilities

| Layer | Owns | Must not |
| --- | --- | --- |
| `handler/` | Parse + validate request, call service, format response (via `pkg/response`). | Contain business rules. Touch the DB. Compose multiple services into a flow. |
| `service/` | Business logic, orchestration of repositories and clients, transactions. | Touch `*gin.Context` (use `context.Context`). Write SQL. Format HTTP responses. |
| `repository/` | SQL, row mapping, pagination, transactions. Returns domain types from `model/`. | Contain business rules. Return Gin-flavored errors. Make HTTP calls. |
| `dto/` | Request and response payload structs with validation tags. | Contain logic. Be imported by `repository/`. |
| `model/` | Domain entities. Pure structs and value methods. | Import any other layer. |
| `routes/` | Wire handlers onto the Gin engine. | Contain business rules. |

### Concrete example — `internal/payments/`

```
payments/
├── handler/
│   └── pay_handler.go          // POST /api/v1/payments — parse, validate, call service.Pay
├── service/
│   ├── pay_service.go          // service.Pay(ctx, req) — acquire lock, create txn, call client, persist
│   └── pay_service_test.go     // unit-tests with mocked repo + client
├── repository/
│   ├── txn_repo.go             // CreateTransaction, UpdateStatus, FindByID
│   └── txn_repo_test.go        // integration test against a real Postgres in CI
├── dto/
│   ├── pay_request.go          // PayRequest with `validate:"required"` tags
│   └── pay_response.go         // PayResponse
├── model/
│   └── transaction.go          // Transaction entity (UUID id, amount, status enum, …)
├── client/                     // ZuelPay HTTP — see internal/clients/zuelpay for the shared client
│   └── payment.go              // module-local wrapper if needed
└── routes/
    └── routes.go               // RegisterRoutes(r *gin.RouterGroup, h *handler.PayHandler)
```

### The composition root is `cmd/api/main.go`

`main.go` is the only place that knows about all modules. It builds the dependency graph (manual constructors or `samber/do`), registers routes, and starts the server. **Modules do not call each other through globals**; they receive each other through constructors when one module legitimately needs another (e.g., rewards service depends on transactions repository — wire it explicitly).

### Cross-module calls — keep them narrow

When module A needs module B's data, define an interface in module A (consumer side) with the smallest surface possible (1–3 methods). Implement it in module B. Wire it in `main.go`. This is the **dependency inversion principle** for cross-module calls — see also `golang-dependency-injection` skill.

```go
// internal/rewards/service/types.go (consumer)
type TransactionLookup interface {
    FindByID(ctx context.Context, id uuid.UUID) (model.Transaction, error)
}

// internal/transactions/repository/txn_repo.go (provider, already exists)
// Its FindByID method satisfies rewards.TransactionLookup by structural typing.
```

### What goes in `pkg/` vs `internal/`

- `pkg/` — reusable libraries with **zero** business knowledge. Examples: error codes (`pkg/errs`), JSON response envelope (`pkg/response`), idempotency-key helpers (`pkg/idempotency`).
- `internal/` — everything that knows about n8n domain.

If you can't import a package into a hypothetical second product without dragging payments concepts along, it belongs in `internal/`.
