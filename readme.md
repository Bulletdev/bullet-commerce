```
  ██████╗  ██████╗      ██████╗ █████╗ ██████╗ ████████╗      █████╗ ██████╗ ██╗
 ██╔════╝ ██╔═══██╗    ██╔════╝██╔══██╗██╔══██╗╚══██╔══╝     ██╔══██╗██╔══██╗██║
 ██║  ███╗██║   ██║    ██║     ███████║██████╔╝   ██║        ███████║██████╔╝██║
 ██║   ██║██║   ██║    ██║     ██╔══██║██╔══██╗   ██║        ██╔══██║██╔═══╝ ██║
 ╚██████╔╝╚██████╔╝    ╚██████╗██║  ██║██║  ██║   ██║        ██║  ██║██║     ██║
  ╚═════╝  ╚═════╝      ╚═════╝╚═╝  ╚═╝╚═╝  ╚═╝   ╚═╝        ╚═╝  ╚═╝╚═╝     ╚═╝
                         E-commerce Backend — go-cart-api
```

<div align="center">

[![CodeQL Advanced](https://github.com/Bulletdev/bullet-cloud-api/actions/workflows/codeql.yml/badge.svg)](https://github.com/Bulletdev/bullet-cloud-api/actions/workflows/codeql.yml)
[![Go](https://github.com/Bulletdev/bullet-cloud-api/actions/workflows/go.yml/badge.svg)](https://github.com/Bulletdev/bullet-cloud-api/actions/workflows/go.yml)
![Go Version](https://img.shields.io/github/go-mod/go-version/Bulletdev/go-cart-api?color=00ADD8&labelColor=000000)
![License](https://img.shields.io/badge/license-GPL--3.0-blue)

</div>

---

```
╔══════════════════════════════════════════════════════════════════════════════╗
║  go-cart-api — Go 1.23 · Clean Architecture · RESTful                        ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Production-ready e-commerce API. PIX + Stripe · NF-e · Correios/Melhor      ║
║  Envio · Argon2id · Circuit breaker · ACID transactions · LGPD compliant     ║
╚══════════════════════════════════════════════════════════════════════════════╝
```

---

<details>
<summary><kbd>▶ Key Features (click to expand)</kbd></summary>

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  [■] Argon2id Password Hashing  — OWASP: m=64MiB, t=3, p=4; no 72-byte cap  │
│  [■] JWT Authentication         — HS256, configurable TTL                   │
│  [■] RBAC                       — user | admin roles, endpoint guards       │
│  [■] Inventory / Stock Control  — atomic decrement inside order transaction │
│  [■] Soft Delete                — products + orders, LGPD compliant         │
│  [■] CPF Validation             — stored on profile, required at checkout   │
│  [■] PIX Payment                — ProPay + OpenPix, QR Code + copy-paste    │
│  [■] Credit Card                — Stripe server-side, Elements UI ready     │
│  [■] Coupon / Discount          — percent and fixed-value, per-order        │
│  [■] Transactional Emails       — order, payment, shipment, password reset  │
│  [■] Shipping Calculation       — Correios / Melhor Envio integration       │
│  [■] Nota Fiscal (NF-e)         — Focusnfe / eNotas, auto-emit on payment   │
│  [■] ACID Transactions          — order creation + stock + cart in one tx   │
│  [■] Circuit Breaker            — ProPay client: 3-state, in-process        │
│  [■] LRU Cache                  — products + categories, configurable TTL   │
│  [■] Pagination                 — all list endpoints, configurable limit    │
│  [■] Product Catalog            — search, by-category, featured flag        │
│  [■] Order Tracking             — public endpoint by tracking number        │
│  [■] Payment Webhooks           — PIX (HMAC-SHA256) + Stripe signature      │
│  [■] X-Request-ID               — log correlation across services           │
│  [■] Structured Logging         — slog, JSON output, all key fields         │
│  [■] Health Probes              — liveness + readiness (db.Ping)            │
│  [■] CORS                       — configurable origins via ENV              │
│  [■] Request Body Limit         — 1MB global middleware, DoS protection     │
│  [■] Connection Pooling         — pgx/v5 pool, configurable min/max conns   │
│  [■] CodeQL                     — security-extended, SARIF to GitHub        │
└─────────────────────────────────────────────────────────────────────────────┘
```

</details>

---

## Table of Contents

```
┌──────────────────────────────────────────────────────┐
│  01 · Quick Start                                    │
│  02 · Technology Stack                               │
│  03 · Architecture                                   │
│  04 · Setup                                          │
│  05 · API Documentation                              │
│  06 · Environment Variables                          │
│  07 · Testing                                        │
│  08 · Security                                       │
│  09 · Observability                                  │
│  10 · Deployment                                     │
│  11 · Contributing                                   │
│  12 · License                                        │
└──────────────────────────────────────────────────────┘
```

---

## 01 · Quick Start

```bash
# Clone and install
git clone https://github.com/bulletdev/go-cart-api.git
cd go-cart-api

# Configure environment
cp .env.example .env
# edit .env with your DATABASE_URL, JWT_SECRET, etc.

# Run migrations
migrate -database ${DATABASE_URL} -path internal/database/migrations up

# Start server
go run cmd/main.go
# → listening on :4445 (or $PORT)

# Health check
curl http://localhost:4445/health          # liveness
curl http://localhost:4445/ready           # readiness (db.Ping)

# Register and get token
TOKEN=$(curl -s -X POST http://localhost:4445/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"your_password"}' | jq -r .token)

# List products (paginated)
curl "http://localhost:4445/api/products?limit=20&offset=0"
```

<details>
<summary><kbd>▶ Docker (click to expand)</kbd></summary>

```bash
docker compose up -d

# Run migrations inside container
docker exec go-cart-api migrate -database ${DATABASE_URL} \
  -path internal/database/migrations up
```

| Container | Port | Role |
|---|---|---|
| go-cart-api | 4445 | Go API |
| postgres | 5432 | PostgreSQL 17 |

</details>

---

## 02 · Technology Stack

```
╔══════════════════════╦════════════════════════════════════════════════════╗
║  LAYER               ║  TECHNOLOGY                                        ║
╠══════════════════════╬════════════════════════════════════════════════════╣
║  Language            ║  Go 1.23 (toolchain go1.24.1)                      ║
║  Router              ║  gorilla/mux v1.8.1                                ║
║  Database            ║  PostgreSQL 17 · pgx/v5                            ║
║  Authentication      ║  JWT HS256 (golang-jwt/jwt v5) · Argon2id hashing  ║
║  Cache               ║  LRU in-process (hashicorp/golang-lru)             ║
║  Payment — PIX       ║  ProPay + OpenPix · HMAC-SHA256 webhook            ║
║  Payment — Card      ║  Stripe server-side · stripe-go                    ║
║  Email               ║  Resend SDK                                        ║
║  Shipping            ║  Correios API · Melhor Envio                       ║
║  Nota Fiscal         ║  Focusnfe / eNotas (REST)                          ║
║  Circuit Breaker     ║  In-process · 3-state (Closed/Open/HalfOpen)       ║
║  Config              ║  godotenv + typed helpers                          ║
║  Testing             ║  testify · handler mocks                           ║
║  CI                  ║  GitHub Actions · CodeQL security-extended         ║
╚══════════════════════╩════════════════════════════════════════════════════╝
```

---

## 03 · Architecture

Clean Architecture — total decoupling between business logic and infrastructure providers.

```
cmd/
└── main.go                  # wiring: config, db, repos, handlers, router

internal/
├── auth/                    # JWT mint/validate · Argon2id hasher · middleware
├── cache/                   # LRU memory cache with TTL + GC goroutine
├── circuit/                 # 3-state circuit breaker (Closed/Open/HalfOpen)
├── clients/
│   └── propay/              # ProPay HTTP client (JWT ServiceClaims + breaker)
├── config/                  # typed ENV loader (getEnv, getInt, getDuration…)
├── database/                # pgx pool + migrations
├── handlers/                # HTTP handlers (one per domain)
├── middleware/              # CORS · RequestID · BodyLimit · RateLimit
├── models/                  # domain structs (Order, Product, User…)
├── [domain]/
│   ├── repository.go        # interface + postgres implementation
│   └── repository_mock.go   # mock for unit tests
└── webutils/                # WriteJSON · ErrorJSON · RawJSON · ReadJSON
```

**Core principles:**
- Interface-based repositories — swap DB or inject mocks without touching handlers
- All mutations in ACID transactions — order + stock decrement + cart clear in one `pgx.Tx`
- Goroutine-based background jobs — `time.NewTicker` for cleanup tasks, no external scheduler
- In-process circuit breaker — no Redis dependency, shared state per server instance

### Request flow

```mermaid
graph LR
    Client -->|HTTP| CORS
    CORS --> BodyLimit
    BodyLimit --> RequestID
    RequestID --> Auth["Auth Middleware\n(JWT validate)"]
    Auth --> Handler
    Handler --> Cache["LRU Cache\n(read path)"]
    Handler --> Repository
    Repository --> PostgreSQL
    Handler --> ProPayClient["ProPay Client\n(circuit breaker)"]
    ProPayClient --> ProPay
    Handler --> Stripe
    Handler --> Resend["Email\n(Resend)"]
```

### Endpoint map

```mermaid
graph LR
    subgraph "Public"
        H["GET /health\nGET /ready"]
        Auth["POST /auth/register\nPOST /auth/login\nPOST /auth/forgot-password\nPOST /auth/reset-password"]
        Prod["GET /products\nGET /products/search\nGET /products/featured\nGET /products/category/{id}\nGET /products/{id}"]
        Cat["GET /categories\nGET /categories/{id}"]
        Ship["POST /shipping/calculate"]
        Track["GET /orders/tracking/{number}"]
        WH["POST /webhooks/payment\nPOST /webhooks/stripe"]
    end

    subgraph "Protected — any user"
        Me["GET /users/me\nPUT /users/me"]
        Addr["GET|POST|PUT|DELETE /users/{id}/addresses\nPATCH …/default"]
        Cart["GET /cart\nPOST /cart/items\nPUT|DELETE /cart/items/{pid}\nDELETE /cart\nPOST /cart/coupon"]
        Orders["POST /orders\nGET /orders\nGET /orders/{id}\nPATCH /orders/{id}/cancel\nPOST /orders/{id}/pay"]
    end

    subgraph "Admin only"
        ProdAdmin["POST|PUT|DELETE /products/{id}\nPATCH /products/{id}/stock"]
        CatAdmin["POST|PUT|DELETE /categories/{id}"]
        OrdAdmin["PATCH /orders/{id}/tracking"]
    end
```

---

## 04 · Setup

### Prerequisites

```
[✓] Go 1.23+
[✓] PostgreSQL 17+
[✓] golang-migrate CLI   — github.com/golang-migrate/migrate
```

<details>
<summary><kbd>▶ Installation (click to expand)</kbd></summary>

**1. Clone:**
```bash
git clone https://github.com/bulletdev/go-cart-api.git
cd go-cart-api
```

**2. Dependencies:**
```bash
go mod tidy
```

**3. Environment:**
```bash
cp .env.example .env
# fill in required vars — see §06 Environment Variables
```

**4. Migrations:**
```bash
migrate -database "${DATABASE_URL}" -path internal/database/migrations up
```

**5. Run:**
```bash
go run cmd/main.go
# API → http://localhost:4445
# Health → http://localhost:4445/health
```

**6. Seed (optional):**
```bash
go run scripts/seed.go
```

</details>

---

## 05 · API Documentation

<details>
<summary><kbd>▶ Full endpoint reference (click to expand)</kbd></summary>

### Authentication

All protected endpoints require:
```
Authorization: Bearer <jwt-token>
```

```
╔═══════════════╦══════════════════════════════╗
║  Algorithm    ║  HS256                       ║
║  TTL          ║  24h (JWT_TTL)               ║
║  Issued by    ║  POST /api/auth/login        ║
╚═══════════════╩══════════════════════════════╝
```

### Auth

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| POST | `/api/auth/register` | — | Register new user |
| POST | `/api/auth/login` | — | Login, returns JWT |
| POST | `/api/auth/forgot-password` | — | Send password reset email |
| POST | `/api/auth/reset-password` | — | Reset password with token |

### Users

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET | `/api/users/me` | user | Get current user profile |
| PUT | `/api/users/me` | user | Update name, email, CPF |
| GET | `/api/users/{id}/addresses` | user | List user addresses |
| POST | `/api/users/{id}/addresses` | user | Add address |
| PUT | `/api/users/{id}/addresses/{aid}` | user | Update address |
| DELETE | `/api/users/{id}/addresses/{aid}` | user | Remove address |
| PATCH | `/api/users/{id}/addresses/{aid}/default` | user | Set default address |

### Products

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET | `/api/products` | — | List products (paginated) |
| GET | `/api/products/search?q=` | — | Full-text search (paginated) |
| GET | `/api/products/featured` | — | Featured products |
| GET | `/api/products/category/{id}` | — | Products by category (paginated) |
| GET | `/api/products/{id}` | — | Product detail |
| POST | `/api/products` | admin | Create product |
| PUT | `/api/products/{id}` | admin | Update product |
| DELETE | `/api/products/{id}` | admin | Soft-delete product |
| PATCH | `/api/products/{id}/stock` | admin | Update stock |

### Categories

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET | `/api/categories` | — | List categories |
| GET | `/api/categories/{id}` | — | Category detail |
| POST | `/api/categories` | admin | Create category |
| PUT | `/api/categories/{id}` | admin | Update category |
| DELETE | `/api/categories/{id}` | admin | Delete category |

### Cart

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET | `/api/cart` | user | Get cart with items |
| POST | `/api/cart/items` | user | Add item (validates stock) |
| PUT | `/api/cart/items/{pid}` | user | Update quantity |
| DELETE | `/api/cart/items/{pid}` | user | Remove item |
| DELETE | `/api/cart` | user | Clear cart |
| POST | `/api/cart/coupon` | user | Apply coupon code |

### Orders

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| POST | `/api/orders` | user | Create order from cart (ACID: stock decrement + cart clear) |
| GET | `/api/orders` | user | List user orders (paginated) |
| GET | `/api/orders/{id}` | user | Order detail with items |
| PATCH | `/api/orders/{id}/cancel` | user | Cancel order |
| POST | `/api/orders/{id}/pay` | user | Initiate payment (PIX or credit card) |
| PATCH | `/api/orders/{id}/tracking` | admin | Set tracking number (triggers shipment email) |
| GET | `/api/orders/tracking/{number}` | — | Public order tracking by number |

### Shipping

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| POST | `/api/shipping/calculate` | — | Calculate shipping options by CEP + cart weight |

**Request:**
```json
{
  "cep_destino": "01310-100",
  "items": [
    { "product_id": "uuid", "quantity": 2 }
  ]
}
```

**Response:**
```json
{
  "options": [
    { "carrier": "PAC", "price_cents": 1850, "days": 7 },
    { "carrier": "SEDEX", "price_cents": 3200, "days": 2 }
  ]
}
```

### Payment

**PIX — `POST /api/orders/{id}/pay`**

```json
{ "method": "pix" }
```
Response:
```json
{
  "qr_code": "data:image/png;base64,...",
  "qr_code_url": "https://...",
  "txid": "abc123...",
  "expires_at": "2026-06-01T15:30:00Z"
}
```

**Credit Card — `POST /api/orders/{id}/pay`**

```json
{ "method": "credit_card" }
```
Response:
```json
{
  "client_secret": "pi_xxx_secret_yyy"
}
```
Use `client_secret` with Stripe Elements on the frontend to collect card details.

### Webhooks

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| POST | `/api/webhooks/payment` | HMAC (`X-Propay-Signature`) | PIX confirmation from ProPay |
| POST | `/api/webhooks/stripe` | Stripe-Signature | Card payment confirmation |

Both webhooks perform an atomic `UPDATE orders SET payment_status='paid', status='processing' WHERE id=$1 AND payment_status='pending_payment'` — idempotent by design.

### Health & Observability

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET | `/health` | — | Liveness — always 200 while process runs |
| GET | `/ready` | — | Readiness — 503 if `db.Ping()` fails |

</details>

---

## 06 · Environment Variables

<details>
<summary><kbd>▶ All environment variables (click to expand)</kbd></summary>

```bash
# ── Core (required) ───────────────────────────────────────────────────────────
DATABASE_URL=postgres://user:pass@host:5432/dbname
JWT_SECRET=your-256-bit-secret
JWT_TTL=24h

# ── Server ────────────────────────────────────────────────────────────────────
PORT=4445
ALLOWED_ORIGINS=https://yourstore.com,http://localhost:8880
LOG_LEVEL=info                         # debug | info | warn | error

# ── Cache ─────────────────────────────────────────────────────────────────────
CACHE_ENABLED=true
CACHE_L1_MAX_SIZE=1000                 # max LRU entries
CACHE_TTL_PRODUCTS=5m
CACHE_TTL_CATEGORIES=15m

# ── Payment — PIX (ProPay) ────────────────────────────────────────────────────
PROPAY_URL=https://propay.internal
GO_TO_PROPAY_SECRET=secret-for-jwt-go-to-propay
PROPAY_TO_GO_SECRET=secret-for-hmac-propay-to-go
PROPAY_TIMEOUT=5s

# ── Payment — Card (Stripe) ───────────────────────────────────────────────────
STRIPE_SECRET_KEY=sk_live_...
STRIPE_WEBHOOK_SECRET=whsec_...

# ── Email (Resend) ────────────────────────────────────────────────────────────
RESEND_API_KEY=re_...
MAILER_FROM_EMAIL=noreply@yourstore.com
FRONTEND_URL=https://yourstore.com

# ── Shipping ──────────────────────────────────────────────────────────────────
SHIPPING_PROVIDER=melhorenvio          # correios | melhorenvio
SHIPPING_API_KEY=...
SHIPPING_SENDER_CEP=01310100

# ── Nota Fiscal (NF-e) ────────────────────────────────────────────────────────
NFE_PROVIDER=focusnfe                  # focusnfe | enotas
NFE_API_KEY=...
NFE_CNPJ=00000000000100
NFE_AMBIENTE=producao                  # homologacao | producao

# ── Circuit Breaker (ProPay client) ──────────────────────────────────────────
CIRCUIT_BREAKER_THRESHOLD=5           # failures before open
CIRCUIT_BREAKER_TIMEOUT=60s           # window for counting failures
CIRCUIT_BREAKER_COOLDOWN=30s          # time open before half-open probe
```

</details>

---

## 07 · Testing

```bash
# All tests
go test ./...

# Handlers (with mocks)
go test -v ./internal/handlers/...

# Auth
go test -v ./internal/auth/...

# With coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

---

## 08 · Security

```
[✓] Argon2id — OWASP: m=64MiB, t=3, p=4 · no 72-byte truncation
[✓] SQL injection — pgx prepared statements throughout
[✓] Password never exposed — json:"-" tag on all hash fields
[✓] CORS — configurable allowlist, no wildcard in production
[✓] Request body limit — 1MB global middleware
[✓] JWT audience claim — service tokens scoped to target (aud: ["propay"])
[✓] Webhook HMAC-SHA256 — X-Propay-Signature + Stripe-Signature verified before processing
[✓] Secrets per direction — GO_TO_PROPAY_SECRET ≠ PROPAY_TO_GO_SECRET
[✓] RBAC — admin guards on all product/category mutation endpoints
[✓] ACID transactions — no partial state on concurrent order creation
[✓] Idempotent webhooks — UPDATE … WHERE payment_status='pending_payment'
[✓] Soft delete — no hard deletes on orders or products
[✓] CodeQL — security-extended, runs on every push + weekly
```

### Rate Limiting

| Rule | Limit | Window |
|---|---|---|
| `POST /auth/login` | 5 requests | 20 seconds per IP |
| `POST /auth/register` | 3 requests | 1 hour per IP |
| `POST /auth/forgot-password` | 5 requests | 1 hour per IP |
| Global per IP | 300 requests | configurable |

---

## 09 · Observability

### Health Probes

| Endpoint | Purpose | Returns |
|---|---|---|
| `GET /health` | Liveness — is the process alive? | Always 200 |
| `GET /ready` | Readiness — can the DB be reached? | 200 / 503 |

> Never point a liveness probe at an endpoint that checks the DB.
> A DB crash → liveness fail → restart loop → reconnection storm → worse incident.

### X-Request-ID

Every request receives an `X-Request-ID` header (read from client or generated as UUID v4). Propagated through context and included in all structured log lines — use it to correlate logs across go-cart-api, ProPay, and email service.

### Structured Logging

All log output is JSON via `slog`. Standard fields on every line:

```json
{
  "time": "2026-06-01T14:00:00Z",
  "level": "INFO",
  "msg": "order created",
  "request_id": "550e8400-e29b-41d4-a716-446655440000",
  "user_id": "...",
  "order_id": "...",
  "total": 149.90
}
```

### Circuit Breaker — ProPay Client

```
closed  (normal)    — requests pass through; failure count tracked per window
open    (tripped)   — POST /orders/{id}/pay returns 503 immediately, no upstream call
half-open (recovery)— one probe request; success closes, failure re-opens
```

| Parameter | Default | ENV |
|---|---|---|
| Failure threshold | 5 in 60s | `CIRCUIT_BREAKER_THRESHOLD` + `CIRCUIT_BREAKER_TIMEOUT` |
| Cooldown (open→half-open) | 30s | `CIRCUIT_BREAKER_COOLDOWN` |

---

## 10 · Deployment

### Ecosystem

go-cart-api is the BFF (Backend for Frontend) in the clubedojava stack. Services it integrates with:

| Service | Stack | Role |
|---|---|---|
| [ProPay](https://github.com/Bulletdev/ProPay) | Ruby · Roda · Sidekiq | Payment gateway — PIX via OpenPix + Efi Bank |
| clubedojava | React 18 · Vite · TypeScript | Frontend SPA — consumes this API |

### Payment flow

```mermaid
graph TD
    Frontend -->|POST /orders/{id}/pay| API["go-cart-api"]
    API -->|JWT ServiceClaims aud=propay| ProPay
    ProPay -->|QR Code| OpenPix
    OpenPix -->|HMAC webhook| ProPay
    ProPay -->|X-Propay-Signature| API
    API -->|UPDATE atomic| PostgreSQL
    API -->|payment email| Resend
    API -->|NF-e emit| NFe["Focusnfe / eNotas"]
```

### Environment

```
[✓] Stateless — no server-side session, horizontal scaling ready
[✓] Single binary — go build -o go-cart-api cmd/main.go
[✓] PORT env — compatible with Railway, Render, Fly.io, Coolify
[✓] Graceful shutdown — SIGTERM drains in-flight requests (30s timeout)
[✓] Docker — Dockerfile included
```

```bash
# Build
go build -o go-cart-api cmd/main.go

# Docker
docker build -t go-cart-api .
docker run -p 4445:4445 --env-file .env go-cart-api
```

### CI/CD

| Workflow | Trigger | What it does |
|---|---|---|
| `codeql.yml` | Push / PR + weekly | CodeQL security-extended · SARIF to GitHub Security tab |
| `go.yml` | Push / PR | `go test ./...` + `go vet` |

---

## 11 · Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make your changes following the patterns below
4. Add tests for new handlers and business logic
5. Ensure all tests pass: `go test ./...`
6. Submit a pull request

### Branch naming

- `feature/` — new features
- `fix/` — bug fixes
- `refactor/` — internal refactoring
- `docs/` — documentation
- `security/` — security fixes

### Patterns to follow

- New domain → add `internal/{domain}/repository.go` with interface + postgres impl + mock
- New endpoint → add handler method + register route in `setupRoutes` in `cmd/main.go`
- Every handler reads user identity via `getAuthenticatedUserID(r)` from context
- Mutations that touch multiple tables → use `pgx.Tx`, always `defer tx.Rollback(ctx)`
- Errors returned to client → `webutils.ErrorJSON(w, err, status)` only
- No comments explaining WHAT the code does — name things clearly instead

---

## 12 · License

```
╔══════════════════════════════════════════════════════════════════════════════╗
║  go-cart-api                                                                 ║
║  GNU General Public License v3.0 (GPLv3)                                     ║
╚══════════════════════════════════════════════════════════════════════════════╝
```

- **Author:** Michael Bullet — [michael@prostaff.gg](mailto:michael@prostaff.gg)
- **Web:** [michaelbullet.dev](https://www.michaelbullet.dev)
- **License:** [GNU General Public License v3.0](LICENSE)

---

<div align="center">

```
▓▒░ · go-cart-api · michaelbullet.com · ░▒▓
```

</div>
