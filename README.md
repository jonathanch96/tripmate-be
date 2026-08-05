# TripMate backend

Go 1.26.5 API for TripMate. The service uses Gin, GORM/PostgreSQL, versioned SQL migrations, Argon2id password hashing, rotating refresh tokens, HS256 access tokens, a uniform response envelope, and strict controller → domain → database boundaries.

## Local setup

```bash
cp .env.example .env
make up
make migrate-up
make run
curl http://localhost:8080/healthz
curl http://localhost:8080/api/v1/ping
open http://localhost:8080/swagger/index.html
```

Run `make test`, `make test-int`, `make lint-arch`, and `make build` before opening a PR. `make down` stops the local stack. Migrations are authoritative and automatic migration is disabled by default. Sprint 01 adds `users` and auditable `refresh_tokens`; authentication routes live under `/api/v1/auth` and bearer-protected profile routes under `/api/v1/users`.
