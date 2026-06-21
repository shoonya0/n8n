# Database access — n8n

> Complements `samber/cc-skills-golang@golang-database`. PostgreSQL 16+ via `pgx/v5`; migrations via `golang-migrate`.

## Rule

1. **Only `repository/` packages talk to the database.** Services and handlers must not import `pgx`, `sql`, or anything that could form a SQL string.
2. Every schema change is a versioned migration with a matching `*.down.sql`.
3. Every table that LLD's DB doc requires gets `id UUID PK`, `created_at TIMESTAMP NOT NULL`, `updated_at TIMESTAMP NOT NULL`, and (where applicable) `deleted_at TIMESTAMP NULL` for soft delete.
4. Transactions are owned by the service, executed via a repository helper, with `context.Context` propagation.

## Why

- A service that builds SQL grows tentacles. Limiting SQL to repositories is the single most effective Go architectural decision the LLD makes.
- A migration without a rollback is a one-way door. Production schema changes must be reversible.
- UUIDs avoid the integer-id leak class of bugs in payments (sequential enumeration). Per LLD §1 DB Standards.

## How to apply

### 1. Repository signatures

Repositories take `ctx`, return domain `model.X` (not `db.X` row types), and return wrapped errors:

```go
type TxnRepository interface {
    Create(ctx context.Context, t model.Transaction) (model.Transaction, error)
    FindByID(ctx context.Context, id uuid.UUID) (model.Transaction, error)
    UpdateStatus(ctx context.Context, id uuid.UUID, status model.TxnStatus) error
    ListByUser(ctx context.Context, userID uuid.UUID, p Pagination) ([]model.Transaction, error)
}
```

Interface lives in `internal/transactions/repository/`. Concrete implementation `*pgxRepo` is also there. Consumers (e.g. rewards service) declare their own narrower interface — see [`clean-architecture.md`](clean-architecture.md) §"Cross-module calls".

### 2. Transactions

```go
// pkg-level helper in internal/repository
func InTx(ctx context.Context, pool *pgxpool.Pool, fn func(pgx.Tx) error) error {
    tx, err := pool.BeginTx(ctx, pgx.TxOptions{})
    if err != nil { return oops.Wrap(err) }
    defer tx.Rollback(ctx) // no-op if Commit succeeded
    if err := fn(tx); err != nil { return err }
    return oops.Wrap(tx.Commit(ctx))
}

// service code
err := repository.InTx(ctx, s.pool, func(tx pgx.Tx) error {
    if err := s.txnRepo.CreateTx(ctx, tx, txn); err != nil { return err }
    if err := s.walletRepo.DebitTx(ctx, tx, userID, amount); err != nil { return err }
    return nil
})
```

Service owns the transaction boundary; repository methods accept a `pgx.Tx` (or use a small `Querier` interface that both `*pgxpool.Pool` and `pgx.Tx` satisfy) so the same code runs in or out of a tx.

### 3. Migrations

```
migrations/000001_create_users.up.sql
migrations/000001_create_users.down.sql
```

- One logical change per migration.
- `up.sql` and `down.sql` always paired. `down.sql` must actually reverse the up.
- Use `IF NOT EXISTS` / `IF EXISTS` so a partial migration is recoverable.
- Indexes that touch large tables: `CREATE INDEX CONCURRENTLY` (split into a separate migration when needed — `migrate` runs each file in a single statement, so concurrent indexes go in their own migration).

### 4. Connection pool

`pgxpool.New` once at startup. Pool size from config (default 25 open / 5 idle, conn max lifetime 30m). The pool is injected into every repository.

### 5. Schema highlights (per LLD DB doc)

**40 tables in two groups.** Always check the DB schema PDFs before adding fields.

| Domain | Tables |
| --- | --- |
| Auth & users | `users`, `user_sessions`, `devices`, `contacts`, `beneficiaries` |
| Payments | `transactions`, `transaction_logs`, `payment_receipts` |
| BBPS | `billers`, `saved_billers`, `bbps_transactions` |
| Recharge | `recharges` |
| Expenses | `expense_categories`, `expenses`, `expense_insights` |
| Split | `splits`, `split_members`, `settlements` |
| UPI Circle | `upi_circles`, `circle_members`, `circle_limits`, `circle_approvals`, `circle_transactions` |
| Rewards | `reward_wallets`, `reward_transactions`, `reward_rules`, `reward_campaigns` |
| Comms | `notifications`, `notification_logs`, `chat_rooms`, `chat_messages`, `payment_requests` |
| Support | `support_tickets`, `ticket_messages` |
| Config (read-only here; mutated by the separate admin service) | `feature_flags`, `app_configs` |

**Note:** `admin_users`, `admin_roles`, `admin_permissions`, `audit_logs`, `analytics_events` are **not** in this database — they live with the separate admin / back-office service in its own schema. Do not add migrations for them here.

### 6. Indexing per LLD

Every table comes with required indexes (e.g. `idx_users_mobile`, `idx_txn_user`, `idx_txn_status`, `idx_txn_utr`, …). When adding a query that filters or joins on a column, **first** check if the index exists in the migration. If not, add it in a new migration.

### 7. JSONB columns

Several tables use `JSONB` (`transactions.metadata`, `transaction_logs.response_payload`, `audit_logs.old_value`/`new_value`, `analytics_events.event_payload`, `app_configs.config_value`). Don't dump raw third-party responses there with secrets — strip first.

### 8. Soft delete

**Soft-delete only. No hard deletes — ever.** Every table that supports row removal carries `deleted_at TIMESTAMPTZ NULL` (or, for short-lived rows, a `revoked_at` / `invalid_at` flag). Queries default-filter `WHERE deleted_at IS NULL`. Provide an explicit `IncludeDeleted` option only where it's genuinely needed (forensic reads from the `/internal/*` admin-service-facing endpoints). `DELETE FROM …` statements in repository code are a review block; physical removal of soft-deleted rows happens only via an out-of-band batch retention job that lives outside the request path.

### 9. Money fields

Amounts are `NUMERIC(18,2)`. Map to `decimal.Decimal` (shopspring or pgx native), **never** `float64`. Float math + money = incident.

### 10. Repository tests

- **Unit:** mocks not needed — repositories are thin enough that the unit test value is in the integration test.
- **Integration:** spin up Postgres via the `dev-up` compose, run migrations, exercise CRUD. Tests live in `tests/integration/` or `<module>/repository/*_integration_test.go` with a `//go:build integration` tag.
