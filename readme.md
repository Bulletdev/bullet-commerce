```
██████╗ ██╗   ██╗██╗     ██╗     ███████╗████████╗
██╔══██╗██║   ██║██║     ██║     ██╔════╝╚══██╔══╝
██████╔╝██║   ██║██║     ██║     █████╗     ██║
██╔══██╗██║   ██║██║     ██║     ██╔══╝     ██║
██████╔╝╚██████╔╝███████╗███████╗███████╗   ██║
╚═════╝  ╚═════╝ ╚══════╝╚══════╝╚══════╝   ╚═╝

 ██████╗ ██████╗ ███╗   ███╗███╗   ███╗███████╗██████╗  ██████╗███████╗
██╔════╝██╔═══██╗████╗ ████║████╗ ████║██╔════╝██╔══██╗██╔════╝██╔════╝
██║     ██║   ██║██╔████╔██║██╔████╔██║█████╗  ██████╔╝██║     █████╗
██║     ██║   ██║██║╚██╔╝██║██║╚██╔╝██║██╔══╝  ██╔══██╗██║     ██╔══╝
╚██████╗╚██████╔╝██║ ╚═╝ ██║██║ ╚═╝ ██║███████╗██║  ██║╚██████╗███████╗
 ╚═════╝ ╚═════╝ ╚═╝     ╚═╝╚═╝     ╚═╝╚══════╝╚═╝  ╚═╝ ╚═════╝╚══════╝

   E-commerce Backend · bullet-commerce
```

<div align="center">

[![CodeQL Advanced](https://github.com/Bulletdev/bullet-commerce/actions/workflows/codeql.yml/badge.svg)](https://github.com/Bulletdev/bullet-commerce/actions/workflows/codeql.yml)
[![Go](https://github.com/Bulletdev/bullet-commerce/actions/workflows/go.yml/badge.svg)](https://github.com/Bulletdev/bullet-commerce/actions/workflows/go.yml)
[![Security Scan](https://github.com/Bulletdev/bullet-commerce/actions/workflows/security.yml/badge.svg)](https://github.com/Bulletdev/bullet-commerce/actions/workflows/security.yml)
![Go Version](https://img.shields.io/github/go-mod/go-version/Bulletdev/bullet-commerce?color=00ADD8&labelColor=000000)
![License](https://img.shields.io/badge/license-GPL--3.0-blue)

</div>

---

```
╔══════════════════════════════════════════════════════════════════════════════╗
║  bullet-commerce - Go 1.25 · Clean Architecture · RESTful                    ║
╠══════════════════════════════════════════════════════════════════════════════╣
║  Production-ready e-commerce API. PIX (ProPay/OpenPix) · Argon2id · JWT      ║
║  Per-variant stock · multi-warehouse · idempotent checkout · ACID · LGPD     ║
╚══════════════════════════════════════════════════════════════════════════════╝
```

---

<details>
<summary><kbd>▶ Key Features (click to expand)</kbd></summary>

```
┌─────────────────────────────────────────────────────────────────────────────┐
│  Argon2id Hashing   - OWASP params m=64MiB t=3 p=4, PHC format              │
│  JWT Auth + RBAC    - HS256, configurable TTL, user/admin guards            │
│  Product Catalog    - types, variants, N:N categories, brands               │
│  Faceted Search     - tsvector full-text + category/price facets            │
│  Variant Stock      - Reserve/Claim/Release/Restock in order tx             │
│  Multi-warehouse    - stock per (variant, source), default src              │
│  Cart by Variant    - per-variant lines, coupons, live pricing              │
│  Orders (ACID)      - reservation + charge + events in one tx               │
│  Idempotency Keys   - dedupe order create, never double-reserve             │
│  Refunds            - full/partial reversal + opt-in restock                │
│  PIX Payment        - ProPay/OpenPix, provider-neutral FlowStatus           │
│  Coupons / Discounts- percent or fixed, VoucherHandler port                 │
│  Product Reviews    - ratings 1-5 + aggregate, moderation                   │
│  Optimistic Locking - product version guard, 409 on stale write             │
│  Rate Limiting      - per-IP token bucket on auth + checkout                │
│  Media / Storage    - URL or presigned S3/R2/MinIO upload                   │
│  Shipping Calc      - table rates by BR region (pluggable port)             │
│  Order Tracking     - public by number + admin tracking update              │
│  AI Assistant (opt) - read-only JWT-scoped tools, off by default            │
│  Soft Delete        - products + orders, hidden from public reads           │
│  CPF Validation     - stored on profile, required at checkout               │
│  Observability      - slog JSON, X-Request-ID, health probes                │
│  Hardening          - CORS, 1 MiB body cap, pgx/v5 pool                     │
│  CI Security        - CodeQL, govulncheck, golangci-lint, gosec             │
└─────────────────────────────────────────────────────────────────────────────┘
```

</details>

---

## Table of Contents

```
┌------------------------------------------------------┐
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
└------------------------------------------------------┘
```

---

## 01 · Quick Start

```bash
# Clone and install
git clone https://github.com/bulletdev/bullet-commerce.git
cd bullet-commerce

# Configure environment
cp .env.example .env
# edit .env with your DATABASE_URL, JWT_SECRET, etc.

# Run migrations
migrate -database ${DATABASE_URL} -path internal/database/migrations up

# Start server
go run cmd/main.go
# → listening on :4444 (or $PORT)

# Health check
curl http://localhost:4444/health          # liveness
curl http://localhost:4444/ready           # readiness (db.Ping)

# Register and get token
TOKEN=$(curl -s -X POST http://localhost:4444/api/auth/login \
  -H "Content-Type: application/json" \
  -d '{"email":"you@example.com","password":"your_password"}' | jq -r .token)

# List products (paginated)
curl "http://localhost:4444/api/products?limit=20&offset=0"
```

<details>
<summary><kbd>▶ Docker (click to expand)</kbd></summary>

```bash
docker compose up -d

# Run migrations inside container
docker exec bullet-commerce migrate -database ${DATABASE_URL} \
  -path internal/database/migrations up
```

| Container | Port | Role |
|---|---|---|
| bullet-commerce | 4444 | Go API |
| postgres | 5432 | PostgreSQL 17 |

</details>

---

## 02 · Technology Stack

```
╔══════════════════════╦════════════════════════════════════════════════════╗
║  LAYER               ║  TECHNOLOGY                                        ║
╠══════════════════════╬════════════════════════════════════════════════════╣
║  Language            ║  Go 1.25 (pgx v5.9.2 requires it)                  ║
║  Router              ║  gorilla/mux v1.8.1                                ║
║  Database            ║  PostgreSQL 17 · pgx/v5 pool                       ║
║  Auth                ║  JWT HS256 (golang-jwt v5) · Argon2id · RBAC       ║
║  Payment             ║  ProPay/OpenPix (PIX) · PaymentProvider port       ║
║  Object Storage      ║  S3/R2/MinIO (minio-go) · optional, gated          ║
║  Shipping            ║  table rates by BR region · Shipping port          ║
║  AI (optional)       ║  Anthropic Claude · read-only JWT tools            ║
║  Config              ║  12-factor env · typed helpers                     ║
║  Testing             ║  testify · pgxmock · handler mocks                 ║
║  CI Security         ║  CodeQL · govulncheck · golangci-lint · gosec      ║
╚══════════════════════╩════════════════════════════════════════════════════╝
```

---

## 03 · Architecture

Domain-Driven Design over a Clean Architecture spine - the domain (aggregates,
invariants, ports) knows nothing about HTTP, the database driver, or any specific
payment/shipping provider.

### Invariants

The decisions the rest of the system is built to preserve:

- **`variant_stock` (per variant × source) is the single source of stock.** The product's
  top-level `stock` is a derived rollup; the legacy `products.stock` / `product_variants.stock`
  columns are deprecated and never read.
- **Reserve → Claim → Release / Restock** move inventory atomically inside the order transaction
  (`available = stock - stock_reserved`, never negative); the `WHERE available >= qty` guard *is*
  the concurrency control, so two checkouts can't take the last unit.
- **Money is int64 cents** at the API boundary (reais only for display) - convert once, at the boundary.
- **Checkout is idempotent:** `POST /orders` dedupes on a per-user `Idempotency-Key`, so a retried
  submit never creates a second order or double-reserves stock.
- **Products use optimistic locking:** an update carries the `version` it last read; a stale write
  is rejected with `409` and nothing is written.

```
cmd/
└- main.go                  # composition root: config → db → repos → providers → handlers → router

internal/
├── models/                  # domain entities & value objects (Order, Product, ProductVariant, Charge, AppliedDiscount)
├── money/                   # Money value object - int64 cents, per-currency rounding, SplitInPayables/Allocate
├── events/                  # in-process event bus (OrderPlaced, PaymentConfirmed) - decouples order side-effects (published after commit)
├── auth/  config/  database/  middleware/  handlers/  webutils/
│
│   # - Aggregate repositories (interface + postgres impl + mock) -
├── products/                # Product aggregate (simple/configurable/bundle · fiscal NCM/CEST · dims · slug/status · brands · N:N categories)
├── variants/                # ProductVariant - stock invariant (Reserve/Claim/Release) · materialized price · barcode
├── media/                   # product/variant media (image/video) - URL reference or presigned upload
├── attributes/              # normalized variation attributes (size/color w/ hex + order) + facet counts
├── cart/                    # Cart aggregate - line identity = variant · coupon application
├── orders/                  # Order aggregate - reservation + charge + events in one tx; SKIP LOCKED cleanup; deliveries
├── charges/                 # PaymentSelection + Charge (main/giftcard/loyalty)
├── coupons/                 # coupon codes (percent/fixed) behind the VoucherHandler port
├── sourcing/                # multi-warehouse: sources + variant_stock per location + Allocator (single-source default)
├── categories/  users/  addresses/   # addresses: independent default billing ≠ shipping
│
│   # - Ports & provider adapters (fork swaps by config) -
├── payment/                 # PaymentProvider port + Registry + FlowStatus/Action state machine
│   └- propay/              # ProPay/OpenPix adapter (JWT service token + HMAC webhook)
├── shipping/                # ShippingProvider port + TableProvider (Brazil regions)
├── storage/                 # object-storage port (S3/R2/MinIO) - presigned upload, gated
├── promotions/              # VoucherHandler port + AppliedDiscount value object
├── search/                  # Filter port + facets (tsvector + GROUP BY counts)
└── ai/                      # OPTIONAL AI assistant (Claude) - off unless key+flag set
    └- tools/               # read-only, JWT-scoped: search_catalog · check_variant_stock · get_my_order_status

# Not yet built (roadmap): internal/cache (LRU) · internal/circuit (breaker) ·
# payment/{efi,itau,pix_static} · emails · NF-e emission · drop deprecated products.stock.
```

### Domain-Driven Design

- **Aggregates & boundaries.** `Product`+`ProductVariant`, `Cart`+`CartItem`,
  `Order`+`OrderItem`. Each aggregate has one repository; cross-aggregate work
  (an order reserving variant stock) happens through the variant repository
  **inside the order's transaction**, never by reaching into another aggregate's tables.
- **Invariants live with the aggregate that owns them.** Stock is an invariant of
  `ProductVariant`, enforced atomically (`UPDATE … WHERE (stock - stock_reserved) >= qty`) -
  not in a handler, not in a service. The order layer only *triggers* Reserve/Claim/Release.
- **Value objects.** Money is `int64` minor units (cents) + `currency` everywhere -
  never float. Presentation formatting is a UI concern.
- **Ports & adapters (anti-corruption).** `payment.Provider` and `shipping.Provider`
  are ports; `propay` and `TableProvider` are adapters. The core depends on the
  interface, a `Registry` selects the implementation by config - so a fork swaps
  ProPay for Efí, or the table freight for Correios, without touching a handler.
- **Ubiquitous language.** Reserve (hold at checkout) · Claim (convert to sale at
  payment) · Release (free on cancel/expire) - the same words in code, SQL, and docs.

### The Twelve-Factor App

| Factor | How bullet-commerce applies it |
|---|---|
| I · Codebase | One repo, many deploys - instância-por-loja; each fork is a deploy. |
| II · Dependencies | `go.mod` vendored; no implicit system deps. |
| III · Config | Everything by env (`config.go`): DB URL, JWT secret, `PAYMENT_PROVIDER`, `SHIPPING_*`, secrets. Nothing hard-coded. |
| IV · Backing services | Postgres and the PSP are attached resources addressed by URL/secret - swappable without code change. |
| V · Build/release/run | `go build` → image → run; migrations are a separate release step. |
| VI · Processes | Stateless; no session affinity. State is in Postgres. |
| VII · Port binding | Self-contained HTTP server binds `$PORT`. |
| VIII · Concurrency | Horizontal via process model; background jobs are goroutines (`time.NewTicker`). |
| IX · Disposability | Fast boot; graceful shutdown (`srv.Shutdown` with timeout). |
| X · Dev/prod parity | Same binary, same migrations; only env differs. |
| XI · Logs | Structured `slog` JSON to stdout - the platform captures the stream. |
| XII · Admin processes | Migrations run as one-off commands against the same code/config. |

**Core principles:**
- Interface-based repositories - swap DB or inject mocks without touching handlers.
- Stock invariant enforced atomically inside the order `pgx.Tx` (Reserve/Claim/Release).
- Goroutine background jobs - two cleanup tickers (orphaned `pending_payment` 30 min · abandoned `unpaid` 15 min), no external scheduler.

### Request flow

```mermaid
graph LR
    Client -->|HTTP| CORS
    CORS --> BodyLimit
    BodyLimit --> RequestID
    RequestID --> RateLimit["Rate Limit\n(auth · checkout)"]
    RateLimit --> Auth["Auth Middleware\n(JWT validate)"]
    Auth --> Handler
    Handler --> Repository
    Repository --> PostgreSQL
    Handler --> Payment["PaymentProvider\n(port + registry)"]
    Payment --> ProPay["ProPay / OpenPix\n(PIX adapter)"]
    Handler --> Storage["Storage\n(S3/R2 · optional)"]
    Handler --> AI["AI Assistant\n(optional, gated)"]
```

### Endpoint map

```mermaid
graph LR
    subgraph "Public"
        H["GET /health\nGET /ready"]
        Auth["POST /auth/register\nPOST /auth/login\n(rate-limited 10/min)"]
        Prod["GET /products\nGET /products/search\nGET /products/featured\nGET /products/category/{id}\nGET /products/{id}\nGET /products/{id}/reviews"]
        Cat["GET /categories\nGET /categories/{id}"]
        Ship["POST /shipping/calculate\nGET /shipping/cep/{cep}"]
        Track["GET /orders/tracking/{number}\nGET /search"]
        WH["POST /webhooks/payment"]
    end

    subgraph "Protected - any user"
        Me["GET /users/me\nPUT /users/me"]
        Addr["GET|POST|PUT|DELETE /users/{id}/addresses\nPATCH …/default[/billing|/shipping]"]
        Cart["GET /cart\nPOST /cart/items\nPUT|DELETE /cart/items/{variantId}\nPOST /cart/coupon · DELETE /cart/coupon/{code}\nDELETE /cart"]
        Orders["POST /orders (Idempotency-Key)\nGET /orders · GET /orders/{id}\nPATCH /orders/{id}/cancel\nPOST /orders/{id}/pay\n(rate-limited 30/min)"]
        Rev["POST /products/{id}/reviews"]
        AI["POST /assistant/chat (when AI on)"]
    end

    subgraph "Admin only"
        ProdAdmin["POST|PUT|DELETE /products/{id}\nPATCH /products/{id}/variants/{vid}/stock\nPATCH /products/{id}/stock (deprecated)\nPOST /products/{id}/variants"]
        Media["POST /products/{id}/media\nPOST /media/upload-url · DELETE /media/{id}"]
        CatAdmin["POST|PUT|DELETE /categories/{id}"]
        OrdAdmin["PATCH /orders/{id}/tracking\nPOST /orders/{id}/refund\nPATCH /reviews/{id}/moderate"]
    end
```

---

## 04 · Setup

### Prerequisites

```
[✓] Go 1.25+
[✓] PostgreSQL 17+
[✓] golang-migrate CLI   - github.com/golang-migrate/migrate
```

<details>
<summary><kbd>▶ Installation (click to expand)</kbd></summary>

**1. Clone:**
```bash
git clone https://github.com/bulletdev/bullet-commerce.git
cd bullet-commerce
```

**2. Dependencies:**
```bash
go mod tidy
```

**3. Environment:**
```bash
cp .env.example .env
# fill in required vars - see §06 Environment Variables
```

**4. Migrations:**
```bash
migrate -database "${DATABASE_URL}" -path internal/database/migrations up
```

**5. Run:**
```bash
go run cmd/main.go
# API → http://localhost:4444
# Health → http://localhost:4444/health
```

</details>

---

## 05 · API Documentation

### Errors

Every error response is JSON with a single `error` field (from `webutils.ErrorJSON`); the HTTP
status carries the machine-readable meaning.

```json
{ "error": "human-readable message describing the failure" }
```

| Status | When it happens |
|---|---|
| `400` | Request validation - a field is missing or invalid |
| `401` | Missing token, or an invalid/expired JWT |
| `403` | Authenticated but not allowed (admin-only route) |
| `404` | Resource not found (includes an unknown `source_id` on a stock write) |
| `409` | Conflict: stock below what is reserved at the source · configurable product with no default variant (returns the candidate `variant_ids`) · duplicate review (one per user/product) · stale product `version` (optimistic lock) · order not payable in its current state |
| `422` | CEP outside the served range · invalid refund amount |
| `429` | Rate limit exceeded (returns a `Retry-After` header) |
| `501` | Capability disabled - media upload with storage off · refund when the PSP doesn't support it |
| `502` | Payment gateway failure (starting a charge or a refund) |
| `503` | `GET /ready` when the database is down |

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
| POST | `/api/auth/register` | - | Register new user (rate-limited per IP) |
| POST | `/api/auth/login` | - | Login, returns JWT (rate-limited per IP) |

### Users

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET | `/api/users/me` | user | Get current user profile |
| PUT | `/api/users/me` | user | Update name, email, CPF |
| GET | `/api/users/{id}/addresses` | user | List user addresses |
| POST | `/api/users/{id}/addresses` | user | Add address |
| PUT | `/api/users/{id}/addresses/{aid}` | user | Update address |
| DELETE | `/api/users/{id}/addresses/{aid}` | user | Remove address |
| PATCH | `/api/users/{id}/addresses/{aid}/default` | user | Set default address (both billing and shipping) |
| PATCH | `/api/users/{id}/addresses/{aid}/default/billing` | user | Set the default **billing** address only |
| PATCH | `/api/users/{id}/addresses/{aid}/default/shipping` | user | Set the default **shipping** address only |

### Products

| Method | Endpoint | Auth | Description |
|-------|---|---|---|
| GET   | `/api/products` | - | List products (paginated) |
| GET   | `/api/products/search?q=` | - | Full-text search (paginated) |
| GET   | `/api/products/featured` | - | Featured products |
| GET   | `/api/products/category/{id}` | - | Products by category (paginated) |
| GET   | `/api/products/{id}` | - | Product detail |
| POST  | `/api/products` | admin | Create product |
| PUT   | `/api/products/{id}` | admin | Update product - **requires `version`** (optimistic lock; `409` on stale write) |
| DELETE| `/api/products/{id}` | admin | Soft-delete product |
| PATCH | `/api/products/{id}/stock` | admin | **DEPRECATED** - resolves the product's default variant and writes its `variant_stock` at the default source. Sends `Deprecation`/`Sunset`/`Link` headers; `409` if the product has zero or several variants (use the per-variant endpoint) |
| POST  | `/api/products/{id}/variants` | admin | Create a variant (sku, attributes, price_cents?, stock)  |
| PATCH | `/api/products/{id}/variants/{variantId}/stock` | admin | Set a variant's **absolute** physical stock at a source (`{stock, source_id?}`; default source when omitted). UPSERTs `variant_stock`, leaves `stock_reserved` untouched, `409` if below what's reserved there, `404` for an unknown `source_id`. Returns the touched source's state + the variant's aggregate `stock_available` |
| POST  | `/api/products/{id}/media` | admin | Attach media by URL (`{url, alt?, position?, variant_id?}`) |
| POST  | `/api/media/upload-url` | admin | Presigned upload URL - **501 when storage is disabled** |
| DELETE| `/api/media/{id}` | admin | Remove a media item |
| GET   | `/api/products/{id}/reviews` | - | List approved reviews (paginated) |
| POST  | `/api/products/{id}/reviews` | user | Submit a review (`{rating 1-5, title?, body?}`) - one per user/product |
| PATCH | `/api/reviews/{id}/moderate` | admin | Approve/reject a review (`{status}`) - recomputes rating aggregate |

`GET /api/products/{id}` returns the full catalog shape: money in `*_cents` + `currency`,
`type`, `attributes` (JSONB) + `variant_variation_attributes`, fiscal (`ncm`/`cest`/`origem`/`unit`),
dimensions (`weight_grams`/`*_mm`), merchandising (`status`/`slug`/`brand_id`/`compare_at_price_cents`),
`category_ids[]` (N:N), `media[]`, and `variants[]` - each variant carrying `barcode`, `stock_available`
(summed across warehouse sources), `stock_policy`, and a materialized `price_cents`. It also carries
`rating_avg` (nullable) and `rating_count`, kept in sync from approved reviews. Public read paths
(detail, list, featured, category, search, and the AI catalog tools) return **only `status = 'active'`,
non-soft-deleted** products; drafts/archived are reachable exclusively through admin routes.

### Categories

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET | `/api/categories` | - | List categories |
| GET | `/api/categories/{id}` | - | Category detail |
| POST | `/api/categories` | admin | Create category |
| PUT | `/api/categories/{id}` | admin | Update category |
| DELETE | `/api/categories/{id}` | admin | Delete category |

### Cart

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET | `/api/cart` | user | Get cart with items |
| POST | `/api/cart/items` | user | Add item - body `{product_id, variant_id?, quantity}` |
| PUT | `/api/cart/items/{variantId}` | user | Update quantity of a variant line |
| DELETE | `/api/cart/items/{variantId}` | user | Remove a variant line |
| POST | `/api/cart/coupon` | user | Apply a coupon - body `{code}`; re-prices the cart |
| DELETE | `/api/cart/coupon/{code}` | user | Remove an applied coupon; re-prices the cart |
| DELETE | `/api/cart` | user | Clear cart |

The cart line identity is the **variant**, not the product: the same product in two
variants (e.g. size M vs G) are distinct lines. `POST /api/cart/items` accepts an optional
`variant_id`; when omitted, the product's default variant (`sku = "default-<product_id>"`,
or the sole variant when there is exactly one) is resolved automatically. The line price is
the variant's `price_cents` when set, otherwise the product's.

### Orders

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| POST | `/api/orders` | user | Create order from cart - body `{shipping_address_id, shipping_cost_cents?, shipping_method?}`. Reserves each variant's stock and clears the cart in one transaction. Accepts an optional **`Idempotency-Key`** header (see below). |
| GET | `/api/orders` | user | List user orders (paginated) |
| GET | `/api/orders/{id}` | user | Order detail with items (each item carries `variant_id`) |
| PATCH | `/api/orders/{id}/cancel` | user | Cancel order - releases the reservation for not-yet-paid orders |
| POST | `/api/orders/{id}/pay` | user | Initiate payment (PIX via ProPay/OpenPix) |
| POST | `/api/orders/{id}/refund` | admin | Refund (full or partial) - reverses the charge via the PSP and **opt-in per-item restock**; `501` if the provider can't refund |
| PATCH | `/api/orders/{id}/tracking` | admin | Set tracking number (emits an `OrderShipped` event on the in-process bus) |
| GET | `/api/orders/tracking/{number}` | - | Public order tracking by number |

**Idempotency & rate limits.** `POST /api/orders` honors an optional `Idempotency-Key`
header scoped to the user: the first request reserves the key in-flight and stores its
response; retries/double-clicks replay that response instead of reserving stock twice.
Auth (`10/min`) and checkout (`orders`, `30/min`) subrouters are rate-limited per IP
(`429` + `Retry-After` on refusal).

### Stock model - Reserve / Claim / Release

Stock is an invariant of the **variant**, not the product (`available = stock - stock_reserved`,
never negative). It moves through three states, each run inside the same DB transaction as the
order status change so inventory and order state can never drift:

| Operation | When | Effect |
|---|---|---|
| **Reserve** | order creation | `stock_reserved += qty` iff `available >= qty` (atomic guard makes concurrent checkouts safe; shortage aborts the order) |
| **Claim** | payment confirmation | `stock -= qty` and `stock_reserved -= qty` together - the reservation becomes a real sale |
| **Release** | cancel / expiry | `stock_reserved -= qty`, physical `stock` untouched - frees a not-yet-paid hold |
| **Restock** | refund | `stock += qty` at the `(variant, source)` the Claim consumed - the inverse of Claim, opt-in per refunded item |

Two background cleanup passes release and cancel abandoned orders: `pending_payment` older than
30 min and `unpaid` older than 15 min (both only when no `payment_reference` is set). Payment
confirmation (`ConfirmOrderPayment`, which Claims stock) is invoked by the payment webhook in
Phase 2.

**`stock_policy`** (per variant) governs sale-past-zero: `deny` (default) refuses to sell once
`available` hits 0; `backorder` is a recorded intent to allow overselling. **V1 enforces `deny`
unconditionally** - the atomic `available >= qty` guard on Reserve fires regardless of policy, so
`backorder` is persisted but not yet honored in the reserve path (documented follow-up).

**Single source of truth.** `variant_stock` (per variant × source) is the ONLY table the
Reserve/Claim/Release/Restock path and both admin stock endpoints read and write. The legacy
`products.stock` and `product_variants.stock`/`stock_reserved` columns are **deprecated and no
longer read** (kept until a later migration drops them): the product's top-level `stock` field is
now a live rollup - `Σ (stock - stock_reserved)` across the product's variants - not a stored
column. An admin correction (`SetStock`) is an absolute UPSERT into `variant_stock`, so it
immediately affects real availability; it never lowers stock below the units reserved at that
source.

### Shipping

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET  | `/api/shipping/cep/{cep}` | - | Look up a Brazilian address by CEP (`422` if out of the served range) |
| POST | `/api/shipping/calculate` | - | Quote freight for a destination CEP |

**Request:**
```json
{
  "dest_cep": "01310-100",
  "weight_grams": 500,
  "subtotal_cents": 9990
}
```
`weight_grams` and `subtotal_cents` are optional. Malformed CEP → `400`; a well-formed CEP
outside every rule → `422`.

**Response:**
```json
{
  "cost_cents": 1500,
  "estimated_days": 3,
  "method": "table-sudeste"
}
```

### Payment

`POST /api/orders/{id}/pay` starts the payment flow via the configured `PaymentProvider`
(the live adapter is **ProPay/OpenPix - PIX**). The response is a provider-neutral **FlowStatus**
state machine, so the frontend renders whatever the PSP needs without knowing which PSP it is:

```json
{
  "Status": "pending",
  "Action": "display_pix",
  "ActionData": {
    "qr_code": "00020126...5802BR6304AB12",
    "expires_at": "2026-06-01T15:30:00Z"
  }
}
```

`Action` is one of `display_pix` (render the QR + copy-and-paste, then poll), `redirect`
(send the browser to `ActionData["url"]`), `show_iframe` (embed `ActionData["url"]`), or `none`
(terminal / nothing for the shopper to do). Card and redirect-based PSPs plug in behind the same
port and reuse `redirect`/`show_iframe` - only the adapter is new, not the frontend contract.
Settlement is confirmed asynchronously by the payment webhook, never by this response.

### Webhooks

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| POST | `/api/webhooks/payment` | HMAC (`X-Propay-Signature`) | Payment confirmation from the PSP (ProPay/OpenPix) |

The webhook performs an atomic `UPDATE orders SET payment_status='paid', status='processing' WHERE id=$1 AND payment_status='pending_payment'` and Claims stock - idempotent by design.

### Search

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET | `/api/search` | - | Faceted product search - `tsvector` full-text + category/price facets with counts, sort & pagination |

### AI Assistant (optional)

Shopping & support assistant powered by Claude, **off by default**. The route exists **only**
when `FEATURE_AI_ASSISTANT=true` **and** `ANTHROPIC_API_KEY` is set; otherwise it is never
registered and the service boots normally (no key → no endpoint).

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| POST | `/api/assistant/chat` | user | Streaming (SSE) chat. Read-only tools (`search_catalog`, `check_variant_stock`, `get_my_order_status`) are **JWT-scoped** - a user can never read another user's order. The assistant never asserts price/stock/status without a tool call. |

Routing: `claude-haiku-4-5` default → `claude-sonnet-5` for hard intents. Semantic RAG, evals and
model routing details live in `DEVDOCS/PRD-ai-assistant.md` (RAG/evals are TODO v2).

### Health & Observability

| Method | Endpoint | Auth | Description |
|---|---|---|---|
| GET | `/health` | - | Liveness - always 200 while process runs |
| GET | `/ready` | - | Readiness - 503 if `db.Ping()` fails |

</details>

---

## 06 · Environment Variables

<details>
<summary><kbd>▶ All environment variables (click to expand)</kbd></summary>

```bash
# - Core (required) -----------------------------─
DATABASE_URL=postgres://user:pass@host:5432/dbname
JWT_SECRET=your-256-bit-secret
JWT_TTL=24h

# - Server ----------------------------------
PORT=4444
ALLOWED_ORIGINS=https://yourstore.com,http://localhost:8880
LOG_LEVEL=info                         # debug | info | warn | error

# - Payment - PIX (ProPay) --------------------------
PAYMENT_PROVIDER=propay                 # selects the provider registered in the payment registry
PROPAY_URL=https://propay.internal      # when empty, the propay provider is not registered
GO_TO_PROPAY_SECRET=secret-for-jwt-go-to-propay
PROPAY_TO_GO_SECRET=secret-for-hmac-propay-to-go
PROPAY_TIMEOUT=5s

# - Shipping ---------------------------------
SHIPPING_PROVIDER=table                # V1: only `table` (built-in BR region rules) is implemented; correios/melhorenvio are roadmap
SHIPPING_SENDER_CEP=01310100           # origin CEP used by the quoting provider
SHIPPING_API_KEY=...                   # only for external carriers

# - Roadmap (not yet wired - configure nothing today): card PSP (efi/itau/stripe) · email (Resend) · NF-e (focusnfe/eNotas) · circuit breaker · LRU cache

# - AI Assistant (optional - stays OFF unless BOTH are set) ---------
FEATURE_AI_ASSISTANT=false            # gate flag; true + a key ⇒ POST /api/assistant/chat is registered
ANTHROPIC_API_KEY=                    # Claude API key; empty ⇒ assistant disabled, service boots normally
AI_MODEL_DEFAULT=claude-haiku-4-5     # high-volume default (no effort/thinking on Haiku)
AI_MODEL_HARD=claude-sonnet-5         # escalation tier for hard intents

# - Object Storage (optional - presigned media upload; URL-reference works without it) ---
STORAGE_ENABLED=false                 # gate; false ⇒ /api/media/upload-url returns 501, URL-register still works
STORAGE_BUCKET=                        # S3/R2/MinIO bucket
STORAGE_ENDPOINT=                      # e.g. s3.amazonaws.com or a MinIO/R2 endpoint
STORAGE_REGION=us-east-1
STORAGE_ACCESS_KEY=
STORAGE_SECRET=
STORAGE_PUBLIC_BASE_URL=               # public base for the returned media URL (CDN/bucket)
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

**Acceptance criteria as executable spec.** Each domain package states its
acceptance criteria as `Given / When / Then` in the package doc comment and covers
them with tests - e.g. `internal/variants` (Reserve/insufficient/Release/Claim),
`internal/shipping` (valid CEP / malformed → 400 / out-of-range → 422),
`internal/payment/propay` (JWT aud+exp, HMAC valid/invalid, 5xx no-panic), and the
order flow (`TestCreateOrderFromCart_ReservesStock`,
`TestCancelOrder_ReleasesReservation`, `TestConfirmOrderPayment_Claims`). The
payment port contract is documented in `docs/payment-provider.md`.

---

## 08 · Security

```
[✓] Argon2id - OWASP: m=64MiB, t=3, p=4 · no 72-byte truncation
[✓] SQL injection - pgx prepared statements throughout
[✓] Password never exposed - json:"-" tag on all hash fields
[✓] CORS - configurable allowlist, no wildcard in production
[✓] Request body limit - 1MB global middleware
[✓] JWT audience claim - service tokens scoped to target (aud: ["propay"])
[✓] Webhook HMAC-SHA256 - X-Propay-Signature verified before processing
[✓] Secrets per direction - GO_TO_PROPAY_SECRET ≠ PROPAY_TO_GO_SECRET
[✓] RBAC - admin guards on all product/category mutation endpoints
[✓] ACID transactions - no partial state on concurrent order creation
[✓] Idempotent webhooks - UPDATE … WHERE payment_status='pending_payment'
[✓] Idempotency keys - POST /orders dedupe per user, no double stock reservation
[✓] Rate limiting - per-IP token buckets: auth 10/min, checkout 30/min (429 + Retry-After)
[✓] Soft delete - no hard deletes on orders or products
[✓] Action SHA pinning - all GitHub Actions pinned to commit SHA (supply chain hardening)
[✓] golangci-lint - govet, staticcheck, errcheck, revive, gocritic, gosec, ineffassign, unused
[✓] gosec - SAST: SQL injection, hardcoded secrets, path traversal, weak crypto
[✓] Semgrep - p/golang + p/secrets, runs on every push + weekly
[✓] govulncheck - CVE scan on all Go dependencies, blocks PR on findings
[✓] Dependency review - blocks PRs with deps of severity ≥ moderate
[✓] CodeQL - security-extended + security-and-quality, runs on every push + weekly
[✓] Qodana - static analysis, Codacy-compatible reports
```

### Rate Limiting

Per-IP token buckets (in-memory), `429` + `Retry-After` on refusal. The client IP is taken from
`RemoteAddr` (X-Forwarded-For is deliberately not trusted).

| Scope | Limit | Applies to |
|---|---|---|
| Auth | 10 requests / min | `POST /auth/register`, `POST /auth/login` |
| Checkout | 30 requests / min | `POST/GET /orders`, `GET /orders/{id}`, `PATCH /orders/{id}/cancel`, `POST /orders/{id}/pay` |

---

## 09 · Observability

### Health Probes

| Endpoint | Purpose | Returns |
|---|---|---|
| `GET /health` | Liveness - is the process alive? | Always 200 |
| `GET /ready` | Readiness - can the DB be reached? | 200 / 503 |

> Never point a liveness probe at an endpoint that checks the DB.
> A DB crash → liveness fail → restart loop → reconnection storm → worse incident.

### X-Request-ID

Every request receives an `X-Request-ID` header (read from client or generated as UUID v4). Propagated through context and included in all structured log lines - use it to correlate logs across bullet-commerce and ProPay.

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

---

## 10 · Deployment

### Ecosystem

bullet-commerce is the BFF (Backend for Frontend) in the clubedojava stack. Services it integrates with:

| Service | Stack | Role |
|---|---|---|
| [ProPay](https://github.com/Bulletdev/ProPay) | Ruby · Roda · Sidekiq | Payment gateway - PIX via OpenPix + Efi Bank |
| clubedojava | React 18 · Vite · TypeScript | Frontend SPA - consumes this API |

### Payment flow

```mermaid
graph TD
    Frontend -->|POST /orders/:id/pay| API["bullet-commerce"]
    API -->|JWT ServiceClaims| ProPay
    ProPay -->|QR Code| OpenPix
    OpenPix -->|HMAC webhook| ProPay
    ProPay -->|X-Propay-Signature| API
    API -->|UPDATE atomic| PostgreSQL
```

### Environment

```
[✓] Stateless - no server-side session, horizontal scaling ready
[✓] Single binary - go build -o bullet-commerce cmd/main.go
[✓] PORT env - compatible with Railway, Fly.io, Coolify
[✓] Graceful shutdown - SIGTERM drains in-flight requests (30s timeout)
[✓] Docker - Dockerfile included
```

```bash
# Build
go build -o bullet-commerce cmd/main.go

# Docker
docker build -t bullet-commerce .
docker run -p 4444:4444 --env-file .env bullet-commerce
```

### CI/CD

| Workflow | Trigger | What it does |
|---|---|---|
| `go.yml` | Push / PR | `lint` (golangci-lint) → `vuln` (govulncheck) → `build + test -race` · lint gates build |
| `security.yml` | Push / PR + weekly Mon | Semgrep `p/golang` + `p/secrets` · results saved as artifact |
| `codeql.yml` | Push / PR + weekly Tue | CodeQL `security-extended,security-and-quality` · SARIF to GitHub Security tab |
| `dependency-review.yml` | Pull Request | Blocks PRs with dependencies of severity ≥ moderate |
| `qodana_code_quality.yml` | Push / PR | Qodana static analysis · Codacy-compatible reports |

All actions pinned to commit SHA (supply chain hardening). To update a pin: `git ls-remote --tags https://github.com/<owner>/<action>.git` and update the SHA + version comment.

---

## 11 · Contributing

1. Fork the repository
2. Create a feature branch: `git checkout -b feature/my-feature`
3. Make your changes following the patterns below
4. Add tests for new handlers and business logic
5. Ensure all tests pass: `go test ./...`
6. Submit a pull request

### Branch naming

- `feature/` - new features
- `fix/` - bug fixes
- `refactor/` - internal refactoring
- `docs/` - documentation
- `security/` - security fixes

### Patterns to follow

- New domain → add `internal/{domain}/repository.go` with interface + postgres impl + mock
- New endpoint → add handler method + register route in `setupRoutes` in `cmd/main.go`
- Every handler reads user identity via `getAuthenticatedUserID(r)` from context
- Mutations that touch multiple tables → use `pgx.Tx`, always `defer tx.Rollback(ctx)`
- Errors returned to client → `webutils.ErrorJSON(w, err, status)` only
- No comments explaining WHAT the code does - name things clearly instead

---
<div align="center">

## 12 · License

```
╔══════════════════════════════════════════════════════════════════════════════╗
║  bullet-commerce                                                                 ║
║  GNU General Public License v3.0 (GPLv3)                                     ║
╚══════════════════════════════════════════════════════════════════════════════╝
```

- **Author:** Michael Bullet - [michael@prostaff.gg](mailto:michael@prostaff.gg)
- **Web:** [michaelbullet.dev](https://www.michaelbullet.dev)
- **License:** [GNU General Public License v3.0](LICENSE)

---


```
▓▒░ · michaelbullet.dev · ░▒▓
```

</div>
