# Testing — n8n

> Complements `samber/cc-skills-golang@golang-testing` and `@golang-stretchr-testify`.

## Rule

1. Every pure service-core function has a **table-driven** unit test.
2. Every repository has at least one **integration** test against a real Postgres.
3. Every webhook / payment flow has an end-to-end test (faked ZuelPay client).
4. Tests are **deterministic** — no real time, no real randomness, no real network.
5. `go test -race ./...` passes in CI.

## Why

- Payments code has high blast radius. Tests are how we sleep at night.
- Table-driven tests document the spec inline — readers learn the rules from the tests.
- Deterministic tests don't flake; flakes destroy trust in the suite and people start ignoring failures.

## How to apply

### 1. Table-driven unit tests for pure functions

```go
func TestCalculateReward(t *testing.T) {
    tests := []struct {
        name  string
        txn   model.Transaction
        rules []model.RewardRule
        state model.UserDailyState
        want  int64
    }{
        {"no rules → 0",     mkTxn(100), nil,                 nostate, 0},
        {"flat 5 coins",     mkTxn(100), []m.RewardRule{flat5}, nostate, 5},
        {"daily limit hit",  mkTxn(100), []m.RewardRule{flat5}, atLimit, 0},
        // …
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := CalculateReward(tt.txn, tt.rules, tt.state)
            require.Equal(t, tt.want, got)
        })
    }
}
```

Goal: when the spec changes, the **table** changes — not the test structure.

### 2. Integration tests for repositories — `//go:build integration`

```go
//go:build integration

func TestTxnRepo_CreateFind(t *testing.T) {
    pool := testpg.New(t)         // helper: spin up via docker compose, run migrations
    repo := repository.New(pool)
    txn  := mkTxn(123)
    got, err := repo.Create(ctx, txn)
    require.NoError(t, err)
    found, err := repo.FindByID(ctx, got.ID)
    require.NoError(t, err)
    require.Equal(t, got.Amount, found.Amount)
}
```

Run with `go test -tags=integration ./...`. Default `go test ./...` runs only unit tests.

### 3. Fake the shell, not the core

- DB → in-tests, use the real DB (integration) or a fake repo that implements the consumer interface (unit).
- Time → inject `Clock` (`func() time.Time`); pass `func() time.Time { return fixed }`.
- UUIDs → inject `IDGen` (`func() uuid.UUID`); pass a deterministic generator.
- ZuelPay → fake client implementing the consumer interface, returning canned responses.
- FCM → fake dispatcher capturing sent notifications for assertions.

The unit test for a service composes a real service struct with **fake** dependencies. There's almost no need for a mocking framework — handwritten fakes are simpler and clearer.

### 4. HTTP handler tests with `httptest`

```go
func TestPayHandler(t *testing.T) {
    r := setupRouter(t)
    body, _ := json.Marshal(dto.PayRequest{…})
    req := httptest.NewRequest(http.MethodPost, "/api/v1/payments", bytes.NewReader(body))
    req.Header.Set("X-Idempotency-Key", "test-key-1")
    req.Header.Set("Authorization", "Bearer "+token)
    w := httptest.NewRecorder()
    r.ServeHTTP(w, req)
    require.Equal(t, http.StatusOK, w.Code)
    var resp response.Envelope
    require.NoError(t, json.NewDecoder(w.Body).Decode(&resp))
    require.True(t, resp.Success)
}
```

### 5. Race detector

```bash
go test -race ./...
```

Required to pass in CI. Concurrency bugs that survive a race-clean test suite are still possible but rare; concurrency bugs that fail it are guaranteed.

### 6. Coverage — guidance, not target

We don't enforce a percent. Instead, the rule is:

- Every **pure** function in `service/` has a table test.
- Every **branch** in error handling has at least one test that walks it.
- New code lands with tests; legacy code is covered when it changes.

Trying to hit a 90% number with assertions on getters is how skills lose to fakes. See `golang-testing` skill.

### 7. Webhook signature tests

The webhook controller's tests include:

- valid signature → 200, side effects observed
- invalid signature → 401, no side effects
- expired timestamp → 401
- duplicate event_id → 200 (idempotent), no duplicate side effects

### 8. Time-based code

Where a worker schedules a task `5min`, `15min`, etc., wrap the schedule in an exported variable / function so tests can drive it directly.

### 9. Determinism — the master rule

If you find yourself adding `time.Sleep` or "wait for goroutine" loops to make a test pass, restructure the code so the test doesn't need to wait. Channels, `WaitGroup`, or a fake clock that advances explicitly are the answers.
