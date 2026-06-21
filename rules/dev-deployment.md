# Dev deployment & integration testing — n8n

> Complements `samber/cc-skills-golang@golang-continuous-integration`. How the n8n API is built into a container, deployed to the shared dev server, and verified. Pairs with the `n8n-dev-deploy` skill and the `.github/workflows/ci.yml` + `cd.yml` pipelines.

## Rule

1. **Deploy through the scripts, never by hand.** Use `scripts/deploy/{remote-bootstrap,deploy,integration-test}.sh` (or the `cd.yml` workflow that calls them). No ad-hoc `ssh … docker run …`. The scripts are idempotent and the single source of truth for how a deploy happens.
2. **The app is 12-factor: configured entirely from `BP_`-prefixed env vars.** The distroless production image ships **no** `config.yaml`. Therefore every config key that must be set at runtime in a container MUST be reachable from the environment. Keys with no YAML value and no default are invisible to viper's `AutomaticEnv` during `Unmarshal` — they **must** be registered with `v.BindEnv(...)` in `internal/config/load.go`. Any new connection/secret key gets a `BindEnv` entry **and** a regression line in `TestLoadEnvOnly`.
3. **App container reaches Postgres/Redis over the host network.** On the dev server, Postgres and Redis run as host services bound to `127.0.0.1`; a bridge network cannot see them. The api service uses `network_mode: host` and connects to `127.0.0.1:5432` / `127.0.0.1:6379`. The listen port is `BP_HTTP_ADDR` (default `:8088`, chosen to avoid colliding with other host processes — do not assume `:8080` is free).
4. **Migrations are a one-shot step, never app boot.** `docker compose run --rm migrate` (the `migrate/migrate` image) applies all up migrations before the api starts. The app never runs migrations on startup. Migrations still obey `database-access.md` (paired `*.down.sql`, round-trip clean, no hard deletes).
5. **Secrets stay out of git.** Real credentials live in two gitignored files: server-side `deployments/.env.deploy` (chmod 600, written by `remote-bootstrap.sh`) and controller-side `scripts/deploy/deploy.env`. Only the `*.example` templates are committed. In CI, the same values are GitHub Actions secrets. (Plaintext-on-disk is accepted for the dev server because its datastores are bound to loopback and not internet-reachable; prefer SSH keys and rotate the password for anything longer-lived.)
6. **A deploy is "done" only when `/readyz` returns 200.** `/healthz` is liveness only (static `ok`); it says nothing about Postgres/Redis. Gate every deploy and every smoke test on `/readyz==200` with `"db":"ok"` and `"redis":"ok"`. Fail fast if the container enters `restarting`/`exited`.
7. **Integration tests run against `n8n_test`, never the runtime DB.** Schema + trigger tests (`go test -tags integration ./tests/integration/...`) and the FK-index audit (`scripts/audit/fk_indexes.sql`, which must return 0 rows) run against `n8n_test`. The runtime `n8n` DB is never a test target.
8. **Remote scripts piped over SSH must detach container stdin.** The deploy scripts feed a script to `bash -s` over the ssh stdin; a container started with stdin attached (`docker compose run` by default) silently swallows the rest of the script. Always use `-T` and/or `</dev/null` for in-heredoc `docker run` / `docker compose run`.
9. **Logs flow through Vector — the app never ships logs itself.** The app writes structured **JSON** to stdout (`N8N_APP_LOGFORMAT=json` in deployed envs; `app.logFormat` decouples format from `app.env`). Vector's `docker_logs` source tails the container, parses + redacts + enriches, and writes `/var/log/n8n/app.log` (tailable) — plus Loki when one exists. Vector config is `deployments/vector.dev.yaml`, applied by `scripts/deploy/setup-vector.sh`. The `vector` user must be in the `docker` group to read the socket. Vector-side redaction is defence-in-depth; the primary rule (`slog-logging.md`: never log secrets) still holds.

## Why

- **12-factor / `BindEnv`:** the distroless image has no shell and no config file. The first dev deploy crash-looped on `db.dsn is required` precisely because `db.dsn` and `redis.addr` had neither a YAML value nor a default, so `AutomaticEnv` never bound them. Container deploys are impossible without explicit env binding.
- **Host networking:** the datastores listen on `127.0.0.1` only. `host.docker.internal` / the docker0 gateway cannot reach a loopback-bound service; host networking is the least-invasive way in.
- **`/readyz` gate:** a process that is listening but cannot reach its database is not a successful deploy. `/healthz` returning `ok` while `/readyz` is 503 is the failure mode the gate exists to catch.
- **stdin detach:** a real, debugged-the-hard-way failure — `compose run migrate` ate the heredoc, `bash` hit EOF after migrate, the api was never started, yet the script exited 0 and reported success.

## How to apply

### First-time server setup (once)

```bash
cp scripts/deploy/deploy.env.example scripts/deploy/deploy.env   # fill in host + creds
./scripts/deploy/remote-bootstrap.sh   # creates n8n role + n8n/n8n_test DBs,
                                        # ensures dockerd is up, writes deployments/.env.deploy
./scripts/deploy/setup-vector.sh       # configures Vector → /var/log/n8n/app.log
                                        # (re-run whenever deployments/vector.dev.yaml changes)
```

`remote-bootstrap.sh` is idempotent and also self-heals a missing `docker` group (a fresh Docker install fails `docker.socket` with `216/GROUP` until the group exists).

### Every deploy

```bash
./scripts/deploy/deploy.sh            # rsync → build → migrate → up (force-recreate) → /readyz gate
./scripts/deploy/integration-test.sh  # migrate n8n_test → go integration tests → FK audit → live smoke
```

### Debugging via logs

```bash
ssh <host> 'tail -f /var/log/n8n/app.log'           # structured JSON, one event per line
ssh <host> 'tail -f /var/log/n8n/app.log | jq "select(.level==\"ERROR\")"'   # errors only
ssh <host> 'journalctl -u vector -f'                    # Vector's own health
```

### Adding a config key that must work in a container

```go
// internal/config/load.go — after AutomaticEnv()
for _, key := range []string{ "db.dsn", "redis.addr", /* …, */ "my.new.key" } {
    _ = v.BindEnv(key)
}
```

Then add an assertion to `TestLoadEnvOnly` in `internal/config/load_test.go`.

### Definition of Done (a deploy task)

- [ ] `deploy.sh` ends in `DEPLOY_OK` and `/readyz` is 200 with `db:ok, redis:ok`.
- [ ] `integration-test.sh` ends in `INTEGRATION_OK` (go integration suite green, FK audit 0 rows).
- [ ] No secret added to git (`git status` shows no `.env`, `.env.deploy`, or `deploy.env`).
- [ ] Any new config key is `BindEnv`-bound and covered by `TestLoadEnvOnly`.
- [ ] CI (`ci.yml`) is green on the change before `cd.yml` deploys.

## Cross-references

- Skill: `n8n-dev-deploy` (operational runbook + checklists).
- Pipelines: `.github/workflows/ci.yml`, `.github/workflows/cd.yml`.
- Scripts: `scripts/deploy/`. Compose: `deployments/docker-compose.deploy.yml`.
- Related rules: [`database-access.md`](database-access.md) (migrations), [`security.md`](security.md) (secrets), [`slog-logging.md`](slog-logging.md) (no secrets in logs).
