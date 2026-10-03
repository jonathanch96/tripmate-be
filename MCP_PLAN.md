# TripMate MCP server

Status: phases 1–4 implemented (OAuth server, MCP endpoint, read tools, bill flow, write tools,
password-less invites). Phase 5 items remain open.

Let users connect ChatGPT, Claude or any MCP-capable AI tool to their TripMate account (OAuth), then
create trips, add expenses, split bills, record settlements and ask about balances from the chat.

## Architecture

```
AI tool (Claude / ChatGPT / …)
  │ 1. user adds connector: https://tripmate.jonathanchang.my.id/mcp
  │ 2. 401 → discovers OAuth metadata → registers itself (DCR)
  │ 3. browser → /oauth/consent (Next.js, requires TripMate login) → Allow
  │ 4. exchanges code → calls MCP tools with Bearer <mcp token>
  ▼
tripmate-fe (only public service)
  pass-through route handlers: /mcp, /oauth/authorize, /oauth/token, /oauth/register,
                               /.well-known/oauth-protected-resource[/mcp], /.well-known/oauth-authorization-server
  ▼
tripmate-be (127.0.0.1 / shared_network only)
```

- MCP transport: Streamable HTTP, **stateless, JSON responses** (no SSE) so it proxies cleanly
  through the frontend. Route handlers are used instead of `next.config` rewrites because rewrites
  are resolved at build time and `BACKEND_BASE_URL` is only known at runtime.
- SDK: official Go SDK `github.com/modelcontextprotocol/go-sdk`.
- Tool handlers call existing domain services directly. Trip resolution reuses the `tripGuard`
  logic (extract it from `pkg/middleware/trip_authorization.go` into a shared helper), so every
  membership / planner / `own_only` rule still applies. The AI can never do more than the user.

## OAuth 2.1 authorization server (tripmate-be)

| Endpoint | Purpose |
|---|---|
| `GET /.well-known/oauth-protected-resource` | RFC 9728 metadata for `/mcp` |
| `GET /.well-known/oauth-authorization-server` | RFC 8414 metadata |
| `POST /oauth/register` | Dynamic client registration (RFC 7591), rate limited |
| `GET /oauth/authorize` | Validate client, exact redirect_uri, PKCE S256, scope, resource → store auth request → redirect to FE `/oauth/consent?req=…` |
| `GET /api/v1/oauth/requests/:id` | Consent page data (client name, logo, redirect host, scopes) — user bearer |
| `POST /api/v1/oauth/requests/:id/approve` / `deny` | Called by FE server action with the user's normal bearer token; returns redirect URL with code |
| `POST /oauth/token` | `authorization_code` (PKCE required) and `refresh_token` |
| `GET /api/v1/oauth/grants`, `DELETE /api/v1/oauth/grants/:id` | Connected apps list / revoke |

Tokens
- Opaque random tokens, stored as SHA-256 hashes → instant revocation.
- Access 1h; refresh 30d, rotating, reuse detection revokes the whole grant (same pattern as
  `refresh_tokens`).
- Bound to the `/mcp` resource (RFC 8707). `/mcp` never accepts app session JWTs; the REST API never
  accepts MCP tokens.
- Grant expires after 90 days without use.

Scopes
- `tripmate.read`, `tripmate.write` (write implies read). Consent screen offers "read-only".
- `/mcp` accepts any valid token (all carry `tripmate.read`), but its 401 challenge names both
  scopes: MCP clients request exactly the challenge's scope, so naming only `tripmate.read` would
  hide the "Create and edit" choice on the consent page.
- Scopes only narrow what the user can already do; roles still enforced by the domain.

Migration `000020_oauth_mcp`
- `oauth_clients` (client_id, optional secret hash, name, redirect_uris, logo_uri, client_uri, auth method)
- `oauth_auth_requests` (client, redirect_uri, scopes, state, code_challenge, resource, status) — the
  authorization code is minted onto the approved request (hash, 60s expiry) and marked `used` on exchange
- `oauth_grants` (client, user, scopes, last_used_at, revoked_at) — one active per user and client
- `oauth_tokens` (grant, kind access/refresh, token_hash, expires_at, revoked_at)
- `expenses.source` gains `assistant`, plus `expenses.created_via` (the AI tool's name) → "via Claude"

## MCP tools (v1)

Read (`tripmate.read`)
- `list_trips` — the user's trips (status filter)
- `get_active_trip(date)` — see bill flow
- `get_trip(trip_code)` — settings, participants, categories, base currency, whether other
  currencies are allowed and the exchange rates the trip has
- `list_expenses(trip_code, filters, page)`
- `get_balances(trip_code)` — who owes whom (optimized)
- `list_settlements(trip_code)`

Write (`tripmate.write`)
- `create_trip`
- `add_expense` — equal / manual / percent / shares; duplicate guard (same trip + amount + date +
  description within ~2 min returns the existing expense)
- `create_bill_expense` — see bill flow
- `record_settlement`
- `set_exchange_rate(trip_code, currency, rate_to_base)` — planner only; see currencies
- `invite_participant(trip_code, email)` — planner only; see invitations

Not in v1 (do these in the web app): delete expense/settlement, finalize/unfinalize,
approve/reject, remove participant, archive.

Currencies: amounts are recorded in the currency the user gave or the bill shows, never converted by
the AI. Before saving (and before a bill preview), a currency other than the base is checked: the
trip must allow multiple currencies and have a rate to the base. Without a rate the tool saves
nothing and tells the AI to ask the user for it and call `set_exchange_rate` (planner) or, for
anyone else, to explain that the planner has to add it. Saved results include the total converted
to the base currency.

All tools declare `readOnlyHint` / `destructiveHint`, an `outputSchema`, and return structured
content. Errors come back as plain, actionable text so the AI can fix and retry: domain errors carry a
next step per error code, and the server instructions tell the AI that a failed call saved nothing
and that it must explain the problem to the user instead of stopping.

## Bill flow

The AI tool reads the photo and does the pairing in chat. TripMate only provides instructions, the
trip and its participants, and saves the final result.

```
User: [photo] "split this bill"
AI → get_active_trip(date = bill date, else user's local today)
     ├─ one      → participants returned, continue
     ├─ multiple → ask the user which trip (never guess)
     └─ none     → recent trips returned, ask the user
AI reads the bill, shows numbered items + participants, user pairs them (items may be shared)
AI asks who paid
AI → create_bill_expense(preview: true)  → shows server-computed per-person totals
AI → create_bill_expense(...)            → expense saved
```

`get_active_trip(date)`
- Trips where the user is a member, not archived/finalized, `start_date ≤ date ≤ end_date`.
- Output: `{ match: "one"|"multiple"|"none", trips: [{ code, name, start_date, end_date,
  base_currency, participants: [{ user_id, name }] }] }`.

`create_bill_expense`
```
trip_code, description, date, currency,
items:          [{ name, amount, user_ids: [...] }],   // item-level discounts already applied
tax, service_charge, discount,                          // bill-level, applied on the total
total,                                                  // as printed, used as the check
payers:         [{ user_id, amount }],
preview:        bool
```
- Maps to `expenseService.Create` with `SplitType: item`, `Items`, and
  `Extras = tax + service_charge − discount`, spread proportionally to each person's item subtotal.
- Calculator change: allow negative `Extras` (discount) as long as items total + extras ≥ 0
  (`money.SplitProportional` already handles negative totals).
- Server computes the split; the AI's arithmetic is never trusted. Mismatch → `SPLIT_SUM_MISMATCH`.
- Item lines written to the expense `note` so the breakdown is visible on the web.

Server `instructions` (also repeated in tool descriptions):
> To split a bill: (1) call `get_active_trip` with the bill's date; if more than one trip matches,
> ask the user which one — never guess. (2) Read the bill and list each item with its price; list
> the trip participants. (3) Ask the user to pair items with people; an item can be shared by
> several people. (4) Ask who paid. (5) Call `create_bill_expense` with `preview: true`, show the
> per-person totals, and save after the user confirms. Pass amounts as printed; pass tax, service
> charge and discount separately — the server spreads them proportionally.

## Invitations

- Existing account → added as participant immediately (current behaviour).
- New email → a password-less placeholder account is created and added to the trip, with a
  pending invitation. No credentials pass through the chat.
- The placeholder can be included in splits right away. Registering with that email claims the
  placeholder in place (same user id), and Google sign-in links it by email, so the person keeps
  every trip and expense they were added to.
- Nobody can sign in to a placeholder with a password until it is claimed.

## Frontend (tripmate-fe)

- Pass-through route handlers (`src/lib/server/passthrough.ts`) for the paths above.
- `/oauth/consent` page: redirect to `/login?callbackUrl=…` if signed out; show app name, logo,
  redirect host, requested access (read / create & edit, with read-only option), signed-in email;
  Allow / Deny via server action.
- Account page: "Connected apps" (client, scopes, last used, Revoke).
- Help section: "Use TripMate in Claude / ChatGPT" with the MCP URL.
- Ledger: "via <app>" badge for MCP-created entries.

## Phases

1. OAuth server + consent page + connected apps; verify with MCP Inspector.
2. MCP endpoint + read tools.
3. Bill flow: `get_active_trip`, `create_bill_expense`, calculator discount change.
4. Other write tools + invitation auto-join.
5. Later: MCP Apps pairing widget, planner tools, directory listings.

## Security checklist

- PKCE S256 required, exact redirect_uri match, single-use 60s codes.
- Resource-bound tokens; app JWTs and MCP tokens never interchangeable.
- Consent always explicit; never auto-approved.
- Rate limits on `/oauth/register`, `/oauth/token`, and per grant on `/mcp`.
- Tool output contains user-written text (descriptions, names) — treat as data; destructive
  actions excluded from v1.
