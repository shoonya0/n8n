# Asynq workers & queues — n8n

## Rule

Background work goes through **Asynq** (Redis-backed). Three priorities:

| Queue | Priority | Use for |
| --- | --- | --- |
| `critical` | highest | payment reconciliation, webhook side-effects (rewards, notifications) |
| `default`  | medium  | reward processing, FCM dispatch, bill reminders |
| `low`      | background | analytics aggregation, campaign expiry, cleanup |

Workers run as the same binary in `-mode=worker` (or separate deployment). Task handlers live in `internal/worker/tasks/`.

## Why

- A payment retry has different urgency than a daily analytics roll-up. Queue priority makes the workload tunable.
- Async work survives restarts because Redis persists the queue (AOF + snapshots in our deploy).
- Failed jobs don't disappear — they go to `dead_letter` after 5 retries with backoff so we can investigate.

## How to apply

### 1. Define a task type per workflow

```go
// internal/worker/tasks/payment_reconciliation.go
const TypePaymentReconciliation = "payment:reconciliation"

type PaymentReconciliationPayload struct {
    TransactionID uuid.UUID `json:"transaction_id"`
}

func NewPaymentReconciliationTask(txnID uuid.UUID) (*asynq.Task, error) {
    p, err := json.Marshal(PaymentReconciliationPayload{TransactionID: txnID})
    if err != nil { return nil, oops.Wrap(err) }
    return asynq.NewTask(TypePaymentReconciliation, p, asynq.Queue("critical"), asynq.MaxRetry(5)), nil
}
```

### 2. Register the handler

```go
mux := asynq.NewServeMux()
mux.HandleFunc(tasks.TypePaymentReconciliation, paymentReconHandler.Handle)
mux.HandleFunc(tasks.TypeRewardProcessing,      rewardHandler.Handle)
mux.HandleFunc(tasks.TypeNotificationDispatch,  notifHandler.Handle)
mux.HandleFunc(tasks.TypeBillReminder,          billHandler.Handle)
mux.HandleFunc(tasks.TypeCircleApprovalTimeout, circleHandler.Handle)
mux.HandleFunc(tasks.TypeCampaignProcessor,     campaignHandler.Handle)
mux.HandleFunc(tasks.TypeAnalyticsAggregator,   analyticsHandler.Handle)
```

Each handler is a thin function that:

1. Unmarshals the payload.
2. Calls the relevant service method (services don't know they're being called from a worker — they take `ctx` and inputs).
3. Returns the error; Asynq handles retry/backoff.

### 3. Retry schedule (LLD §12 Part 2)

Per-task retry schedule: **5min → 15min → 30min → 60min → 24h**, then dead letter.

```go
asynq.NewTask(TypePaymentReconciliation, payload,
    asynq.Queue("critical"),
    asynq.MaxRetry(5),
    asynq.RetentionDays(7),
)
```

For irregular schedules pass `asynq.ProcessIn(d)` or use the `RetryDelayFunc` on the server.

### 4. Dead letter queue

After 5 failed retries, Asynq moves the task to its archived/failed bucket. We mirror it to a `dead_letter` queue (alert + manual triage). The worker's `RetryDelayFunc` and `ErrorHandler` log a structured error and increment a Prometheus counter (`asynq_dead_letter_total{type=...}`).

### 5. Distributed locks for non-idempotent work

If a worker handler performs work that must not run concurrently for the same key (e.g. paying out a single transaction), acquire a Redis lock:

```go
key := fmt.Sprintf("payment_lock:%s", txnID)
locked, err := cache.AcquireLock(ctx, key, 30*time.Second)
if err != nil { return oops.Wrap(err) }
if !locked { return nil } // someone else has it; this task can no-op or retry
defer cache.ReleaseLock(ctx, key)
```

### 6. Reconciliation worker (LLD §12 Part 2)

When ZuelPay returns `PENDING`, enqueue a reconciliation task. The handler:

1. Loads the txn.
2. Calls ZuelPay's status API.
3. Updates the txn.
4. On success: triggers reward processing + notification.
5. On still-pending: returns a "please retry" error to bounce into the next retry slot.

### 7. Scheduled workers

For periodic jobs (bill reminders daily, campaign expiry, analytics hourly), use Asynq's scheduler (`asynq.NewScheduler`) wired in the same binary. Cron specs live in `internal/worker/schedule.go`.

### 8. Worker logging

Every task starts with one `slog.Info` (queue, type, task_id, payload IDs — never full payload). Every failure logs `slog.Error` with `err` and `attempt`. Success at `Info` only if the work is rare; for high-volume tasks (notifications) log success at `Debug` and let metrics carry the rate.

### 9. Testing

Workers are testable by calling the handler function directly with a constructed `asynq.Task`. Don't spin up a real Asynq server in unit tests.

```go
func TestPaymentReconciliation(t *testing.T) {
    task, _ := tasks.NewPaymentReconciliationTask(uuid.MustParse("…"))
    err := handler.Handle(ctx, task)
    require.NoError(t, err)
    // assert side effects via fakes
}
```
