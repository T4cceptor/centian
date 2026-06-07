# PostgreSQL Backend

Centian stores its SQL data — task/action events, task runs, benchmarks, and
authentication principals — through the [Bun](https://bun.uptrace.dev/) ORM.
By default these live in local SQLite files. You can instead point them at a
**PostgreSQL** database.

Two independent backends can use Postgres:

| Backend | What it stores | Config block |
| --- | --- | --- |
| Event storage | task events, action events, task runs, benchmarks | per-project `capabilities.eventStorage` |
| Auth | principals, credentials, gateway/project grants | global `authBackend` |

You can switch either one (or both) to Postgres; they are configured
separately and may use different databases. **Project/proxy configuration
itself is still file-based** (`~/.centian/config.json`) and is not affected.

> Postgres support is additive — SQLite remains the default and nothing changes
> unless you set `driver`/`type` to `postgres`.

---

## Connection string (DSN)

Both backends take a standard PostgreSQL connection string (libpq / pgx
format):

```
postgres://USER:PASSWORD@HOST:PORT/DATABASE?sslmode=SSLMODE
```

Examples:

```
# Local instance, TLS disabled (typical for local dev / Docker)
postgres://centian:centian@127.0.0.1:5432/centian?sslmode=disable

# Managed database requiring TLS
postgres://app:s3cret@db.example.com:5432/centian?sslmode=require
```

Notes:

- `sslmode` defaults to `prefer` if omitted; set `disable` for a plain local
  container or `require`/`verify-full` for production.
- Keep credentials out of version control. A common pattern is to expand an
  environment variable into the config at deploy time, or to template
  `config.json` from your secrets manager.
- Centian opens the connection at startup and **pings it immediately**, so a bad
  DSN fails fast with a clear error instead of failing on first use.

---

## Configure event storage (per project)

Set the `eventStorage` capability of a project (or the flat `proxy.capabilities`
in the legacy layout) to use the `postgres` driver and supply a `dsn`:

```json
{
  "name": "Team Proxy",
  "version": "1.0.0",
  "projects": {
    "default": {
      "capabilities": {
        "taskVerification": { "enabled": true },
        "eventStorage": {
          "enabled": true,
          "driver": "postgres",
          "dsn": "postgres://centian:centian@127.0.0.1:5432/centian?sslmode=disable"
        },
        "ui": { "enabled": true }
      },
      "gateways": {
        "default": {
          "mcpServers": {
            "memory": { "command": "npx", "args": ["-y", "@modelcontextprotocol/server-memory"] }
          }
        }
      }
    }
  }
}
```

- `dsn` is **required** when `driver` is `postgres`; `path` is ignored.
- **One database per project** for now: give each project its own `dsn`. (A
  shared database with per-project schemas may be added later.)
- The schema is created automatically on first start. JSON payload columns use
  `jsonb`, so you can query them directly (`->`, `->>`, `@>`, GIN indexes).

## Configure the auth backend (global)

Auth is global (a token is resolved to a principal before any project is
chosen), so it lives in the top-level `authBackend` block. For Postgres, set
`type` to `postgres` and put the DSN in `store`:

```json
{
  "authBackend": {
    "type": "postgres",
    "store": "postgres://centian:centian@127.0.0.1:5432/centian?sslmode=disable"
  }
}
```

Then mint an API key. `centian auth new-key` writes to the backend defined by
the config it reads (`~/.centian/config.json` by default, or `--config <path>`):

```bash
centian auth new-key --name "ci bot" --projects default
```

The principals/credentials/grants tables are created automatically the first
time the store is opened. The token is printed once — store it securely.

---

## Quickstart: local Docker container

Run a throwaway Postgres for local development:

```bash
docker run -d --name centian-pg \
  -e POSTGRES_USER=centian \
  -e POSTGRES_PASSWORD=centian \
  -e POSTGRES_DB=centian \
  -p 5432:5432 \
  postgres:16
```

That matches the DSN used above:
`postgres://centian:centian@127.0.0.1:5432/centian?sslmode=disable`.

Stop and remove it when you're done:

```bash
docker rm -f centian-pg
```

### Docker Compose

```yaml
# docker-compose.yml
services:
  postgres:
    image: postgres:16
    environment:
      POSTGRES_USER: centian
      POSTGRES_PASSWORD: centian
      POSTGRES_DB: centian
    ports:
      - "5432:5432"
    volumes:
      - centian-pgdata:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U centian"]
      interval: 5s
      timeout: 5s
      retries: 5

volumes:
  centian-pgdata:
```

```bash
docker compose up -d
```

The named volume (`centian-pgdata`) persists data across restarts; drop it with
`docker compose down -v` for a clean slate.

---

## Schema and behavior notes

- **Auto-bootstrap.** A fresh Postgres database has its tables created at the
  latest schema version on first start; no manual migration step is required.
  (The legacy SQLite-only migrations are never run against Postgres.)
- **Type mapping.** The canonical SQLite schema is adapted for Postgres at
  bootstrap: `BLOB → JSONB` (every blob column holds JSON), `INTEGER → BIGINT`
  (unix-millisecond timestamps overflow a 4-byte `INTEGER`), and
  `REAL → DOUBLE PRECISION`.
- **Foreign keys are enforced.** Unlike SQLite (which does not enforce foreign
  keys by default), Postgres enforces them. Centian's normal flows satisfy these
  constraints (for example, a task run is registered before its events are
  recorded).
- **No automatic data copy.** Switching a backend from SQLite to Postgres starts
  from an empty Postgres database; existing SQLite data is not migrated.

---

## Troubleshooting

| Symptom | Likely cause / fix |
| --- | --- |
| `failed to connect to postgres: ... connection refused` | Server not reachable; check host/port and that the container is running (`docker ps`). |
| `pq: SSL is not enabled on the server` / TLS errors | Add `?sslmode=disable` for local/dev, or fix TLS settings for production. |
| `password authentication failed` | Wrong user/password in the DSN. |
| `event storage driver "postgres" requires a dsn` | `driver` is `postgres` but `dsn` is empty. |
| `auth backend "postgres" requires a store (postgres DSN)` | `authBackend.type` is `postgres` but `store` is empty. |

See the [Configuration Reference](configuration_reference.md) for the full
`eventStorage` and `authBackend` field tables.
