# Functional programming in Go — n8n

> This rule defines the "pragmatic functional Go" style this project uses. It complements `samber/cc-skills-golang@golang-samber-lo`, `@golang-samber-mo`, `@golang-samber-ro` but does not duplicate them.

## Rule

Default to a **functional core, imperative shell** structure:

1. The **core** of every service is a set of **pure functions**: input in, output out, no side effects, no `time.Now()`, no `rand`, no logging, no DB.
2. The **shell** (handlers, repository calls, HTTP clients, FCM dispatch, time, randomness) wraps the core and performs the side effects.
3. Errors are **values**; mutation is **localized**; collections are **transformed by composition** with `samber/lo`.

## Why

- Payments code has a high cost of bugs. Pure functions are exhaustively testable with table-driven tests — no mocks, no fixtures, no flakes.
- Money math (fees, splits, reward rules, limits) is naturally functional: it is a deterministic computation over inputs. Mixing it with I/O produces "I changed the logger and a tax calculation broke" incidents.
- Concurrency safety: pure functions are trivially goroutine-safe. Most data races in Go come from accidentally shared mutable state.

## How to apply

### 1. Split the service function in two

```go
// internal/rewards/service/reward_service.go

// Pure core — exhaustively unit-testable.
//   inputs: txn amount + currency + applicable rules + user state
//   output: reward coins to credit (or zero)
//
// No DB, no time.Now, no logging. Pass everything in.
func CalculateReward(txn model.Transaction, rules []model.RewardRule, userToday model.UserDailyState) int64 {
    return lo.Reduce(rules, func(acc int64, r model.RewardRule, _ int) int64 {
        if !r.Matches(txn) {
            return acc
        }
        return acc + r.CoinsFor(txn, userToday)
    }, 0)
}

// Imperative shell — orchestrates I/O.
func (s *Service) CreditReward(ctx context.Context, txnID uuid.UUID) error {
    txn, err := s.txnRepo.FindByID(ctx, txnID)
    if err != nil { return oops.Wrapf(err, "load txn %s", txnID) }
    rules, err := s.rulesRepo.ActiveForModule(ctx, txn.Type)
    if err != nil { return oops.Wrap(err) }
    today, err := s.walletRepo.UserDailyState(ctx, txn.UserID, s.clock.Now())
    if err != nil { return oops.Wrap(err) }

    coins := CalculateReward(txn, rules, today)   // ← pure
    if coins == 0 { return nil }

    return s.walletRepo.Credit(ctx, txn.UserID, coins, txn.ID)
}
```

The test for `CalculateReward` reads like a spec — no `gomock`, no DB, no clock.

### 2. Pass `time.Now`, `uuid.New`, `rand` in — never call them in the core

Inject a `Clock` interface (or just a `func() time.Time`) and a `func() uuid.UUID` into services. Tests use a frozen clock. Production wires `time.Now` and `uuid.New`.

```go
type Clock func() time.Time
type IDGen func() uuid.UUID

func NewService(clock Clock, idGen IDGen, …) *Service { … }
```

### 3. Immutability by convention

- Don't mutate input slices, maps, or struct fields. Return new values.
- Use **value receivers** unless you genuinely need pointer semantics.
- Prefer constructing a new struct over `s.field = …` mutation.

```go
// ✓ Good — returns a new Transaction with status changed
func (t Transaction) WithStatus(s Status) Transaction {
    t.Status = s
    t.UpdatedAt = now()
    return t
}

// ✗ Bad — silently mutates caller's value
func (t *Transaction) SetStatus(s Status) { t.Status = s }
```

### 4. Use `samber/lo` for collection transformations

Replace for-loops that build slices/maps with `lo.Map`, `lo.Filter`, `lo.Reduce`, `lo.GroupBy`, `lo.Partition`, `lo.UniqBy`. **Why:** the intent of a transformation ("project these txns to amounts") becomes the type signature, not buried in a loop. See `golang-samber-lo` skill.

```go
// ✓ Good
amounts := lo.Map(txns, func(t model.Transaction, _ int) decimal.Decimal { return t.Amount })
totals := lo.GroupBy(amounts, func(a decimal.Decimal) string { return a.Currency })

// ✗ Bad — manual accumulator, easy to introduce off-by-one
amounts := make([]decimal.Decimal, 0, len(txns))
for _, t := range txns { amounts = append(amounts, t.Amount) }
```

**Exception:** for-loops remain better when the iteration has early exit, side effects with sequencing, or performance hot paths (benchmark — see `golang-benchmark` skill).

### 5. Use `samber/mo` for optional values and structured results

- `mo.Option[T]` instead of `*T` or `(T, bool)` for "value or none". `None()` cannot be dereferenced by mistake.
- `mo.Result[T]` for computations whose failure is a domain outcome (e.g. "limit exceeded"), distinct from infrastructure errors.

See `golang-samber-mo` skill.

### 6. Function composition for pipelines

When a service flow is "do A, then B, then C, short-circuit on failure", build it as a sequence of `mo.Result[T]` transformations rather than nested `if err != nil` rungs.

### 7. Errors as values — never panic in business code

Wrap with `samber/oops` to add stack + tags + context. Recover only in the recovery middleware (panics from third-party libs). See [`error-handling.md`](error-handling.md).

### 8. No package-level mutable state

- No `var globalCache = …`
- No `init()` that mutates anything observable
- No singletons that are not `sync.Once`-guarded constants

Dependencies enter through constructors. The composition root (`cmd/api/main.go`) is the only place that knows the full graph.

### 9. Interfaces at the consumer

Declare an interface where it is **used**, with the **smallest** surface that satisfies that use site. Don't define a "complete" interface in the provider package "in case someone else wants it". See `golang-structs-interfaces` skill.

### 10. Composition over inheritance — Go has no inheritance anyway

When you reach for embedding to "share behavior", first ask whether a free function over the data would do. It usually does.

## Quick test

If a service function compiles without importing any of: `database/sql`, `*pgx*`, `*redis*`, `time` (unless an injected `Clock`), `os`, `net/http`, `gin`, `log/slog`, `firebase.google.com/*`, `*asynq*` — it is part of the **core**. Otherwise, it is the **shell**.

The core can be 50% of your service code or 5% — the point is it exists, is named, is exported (for tests), and is the part you change first when business rules change.
