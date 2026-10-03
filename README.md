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

## MCP server (AI assistants)

AI tools such as Claude and ChatGPT can connect to a user's account at `PUBLIC_APP_URL/mcp` (Streamable HTTP, stateless, JSON responses). Access is granted with OAuth 2.1: the tool discovers `/.well-known/oauth-protected-resource/mcp`, registers itself at `/oauth/register`, sends the user through `/oauth/authorize` to the frontend consent page, and exchanges the code (PKCE S256 required) at `/oauth/token`. Only the frontend is public; it forwards these paths here. Tokens are opaque and stored hashed, scopes are `tripmate.read` and `tripmate.write`, and every tool call goes through the same domain services and trip membership checks as the REST API. Users manage connected apps under `/api/v1/oauth/grants`. See `MCP_PLAN.md` for the tools and the bill-splitting flow.
