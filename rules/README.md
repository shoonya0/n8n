# n8n rules

Project-specific rules. Read the ones relevant to your task. These **override** the generic `samber/cc-skills-golang` skills on conflict.

| Rule | When |
| --- | --- |
| [`clean-architecture.md`](clean-architecture.md) | Module structure, layer boundaries |
| [`functional-programming.md`](functional-programming.md) | Service-layer logic, data transformations |
| [`gin-api.md`](gin-api.md) | HTTP handlers, routes, middleware |
| [`slog-logging.md`](slog-logging.md) | Any logging |
| [`error-handling.md`](error-handling.md) | Errors, response envelope |
| [`database-access.md`](database-access.md) | SQL, repositories, migrations |
| [`asynq-workers.md`](asynq-workers.md) | Background jobs, queues, retries |
| [`security.md`](security.md) | Auth, JWT, webhooks, secrets |
| [`testing.md`](testing.md) | Tests, coverage, integration |

Each rule has the form:

1. **What** the rule says (imperative).
2. **Why** it exists (the consequence of breaking it).
3. **How to apply** (concrete code patterns, do/don't).
