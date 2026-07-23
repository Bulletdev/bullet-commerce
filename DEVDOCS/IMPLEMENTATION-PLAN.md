# Plano de Implementação — bullet-commerce (Capacidades Progressivas)

**Companion de** `DEVDOCS/PRD-flamingo-gap-analysis.md` (v2). Traduz o roadmap em **work items (WI)** concretos: objetivo, arquivos, migração, esboço de interface, critérios de aceite (Given/When/Then), testes e dependências.

## Como ler
- **WI-Fx** = fundação · **WI-Px** = pagamento · **WI-Sx** = costuras estruturais · **WI-Gx/Cx/Ex** = Growth/Scale/Enterprise.
- **Definition of Done (todos os WI):** `go build/vet/test ./...` verde · `gofmt` limpo · critérios de aceite viram teste (pgxmock/httptest) · comentários só WHY · config nova por ENV (12-Factor) · migração com `.up`/`.down`.
- **Padrão de porta:** interface + adapter **default** (piso) + flag/config. Porta só onde há ≥2 implementações plausíveis; senão, struct.
- **Padrão "default X":** schema aditivo com um default implícito (como a variante default) — o caminho simples não sente a estrutura rica.

## Grafo de dependências (ordem recomendada)
```
F1 Money VO ─┐
F2 Event bus ─┼─► P2 PaymentSelection/Charge ─► P3 Pay endpoint ─┐
             │   P1 FlowStatus/Action ─────────► P4 Webhook ─────┼─► P5 Saga place-order
S2 Sourcing/source_id ─┐                                          │
S1 Cart→Delivery ──────┴─────────────────────────────────────────┘
                                                                   └─► (Phase 2 fecha)
Growth (G*) depois da fundação · Scale (C*) depois das costuras S* · Enterprise (E*) por último
```
Sequência sugerida: **F1, F2** (paralelos) → **P1, P2** → **S2, S1** → **P3, P4** → **P5**. As costuras S* entram antes do saga porque o saga reconstrói o place-order e deve já conhecer delivery/source.

---

# FASE 2 — Fundação + Pagamento robusto (detalhado)

## WI-F1 · Money value object
**Objetivo:** dinheiro robusto sem `big.Float` — `int64` centavos + precisão por moeda + split sem perder centavo.
**Arquivos:** `internal/money/{money.go,currency.go,money_test.go}`
**Interface:**
```go
type Money struct { Cents int64; Currency string }
func New(cents int64, cur string) Money
func (m Money) Add(o Money) (Money, error)      // erro se moedas diferem (currency guard)
func (m Money) Sub(o Money) (Money, error)
func (m Money) MulQty(q int) Money
func (m Money) SplitInPayables(n int) []Money    // soma == m; distribui restos
func (m Money) Allocate(weights []int64) []Money  // rateio (desconto/frete por item)
func (m Money) Format() string                    // "R$ 12,45" por currency.Fraction
// currency.go: tabela {code -> fraction}: BRL=2, USD=2, JPY=0, BHD=3
```
**Migração:** nenhuma.
**Aceite:**
- Dado 1245 centavos / 6, Quando `SplitInPayables(6)`, Então soma == 1245 (2.07×2 + 2.08×4).
- Dado moedas diferentes, Quando `Add`, Então erro `ErrCurrencyMismatch`.
- `Allocate([]int64{3,1})` de 100 → [75,25]; soma preserva total.
**Adoção:** refatorar cálculo de total do cart/order para usar `Money` (incremental, não bloqueante).
**Deps:** nenhuma.

## WI-F2 · Event bus interno
**Objetivo:** desacoplar side-effects (email, tracking, NF-e, BI) do fluxo; publicar **após** commit.
**Arquivos:** `internal/events/{bus.go,events.go,bus_test.go}`; wiring em `cmd/main.go`
**Interface:**
```go
type Event interface { Name() string }
type Handler func(ctx context.Context, e Event)
type Bus interface { Subscribe(name string, h Handler); Publish(ctx, e Event) }
// Eventos: OrderPlacedEvent{OrderID,...}, PaymentConfirmedEvent{OrderID, ChargeRef}
```
Publisher in-process (default). `Publish` chamado **depois** de `tx.Commit`. Panic de um handler é isolado (recover + log), não derruba os outros. Escala: trocar por adapter NATS/Redis Streams.
**Aceite:**
- Publicado após commit → subscriber recebe.
- Handler que faz panic → logado, demais handlers ainda rodam.
- Order criada dispara `OrderPlacedEvent` (subscriber de email stub loga).
**Deps:** nenhuma.

## WI-P1 · FlowStatus/Action no port de pagamento
**Objetivo:** unificar PIX/boleto/cartão numa resposta que instrui o front; separar `approved` de `paid`.
**Arquivos:** `internal/payment/provider.go` (estende), `internal/payment/propay/client.go` (+ testes), `docs/payment-provider.md` (atualiza)
**Interface (aditivo ao `payment.Provider`):**
```go
type FlowAction string
const ( ActionDisplayPix FlowAction="display_pix"; ActionRedirect="redirect";
        ActionShowIframe="show_iframe"; ActionNone="none" )
type FlowStatus struct { Status ChargeStatus; Action FlowAction; ActionData map[string]string }
// no Provider:
StartFlow(ctx, PixChargeRequest) (*PixCharge, FlowStatus, error)
FlowState(ctx, providerID string) (FlowStatus, error)   // polling/reconciliação
CancelCharge(ctx, providerID, reason string) error
```
propay: `StartFlow` devolve `Action=display_pix` com `ActionData{qr_code, copy_paste, expires_at}`. Estados: separar `approved` (gateway) de `paid` (liquidou).
**Aceite:**
- propay `StartFlow` → `Action=display_pix` + `ActionData.copy_paste` preenchido.
- `CancelCharge` chama o endpoint de cancelamento com motivo.
- assert compile-time `var _ payment.Provider` continua válido.
**Deps:** nenhuma (estende interface existente; atualizar assert + propay).

## WI-P2 · PaymentSelection + Charge
**Objetivo:** modelar pagamento como seleção de charges por tipo/método — destrava misto/gift card/loyalty sem refactor futuro.
**Arquivos:** `internal/models/payment_selection.go`; migração `000012_payment_charges`; `internal/orders/repository.go` (+ testes)
**Migração 000012:** `payment_charges(id, order_id FK, type TEXT CHECK IN ('main','giftcard','loyalty'), method TEXT, amount_cents BIGINT, reference TEXT, status TEXT, created_at)`. Índice por order_id.
**Model:**
```go
type Charge struct { ID, OrderID uuid.UUID; Type, Method string; AmountCents int64; Reference string; Status ChargeStatus }
type PaymentSelection struct { Charges []Charge }  // default: 1 charge main == total
```
Na criação da order: inserir 1 charge `main` com `amount_cents = total_cents`.
**Aceite:**
- Order criada tem exatamente 1 charge `main` = total.
- Suporta adicionar charge `giftcard` (soma dos charges == total).
**Deps:** WI-F1.

## WI-P3 · Endpoint de pagamento + idempotência + referência reservada
**Objetivo:** iniciar o pagamento de forma idempotente.
**Arquivos:** `internal/handlers/order_handler.go` (rota `POST /api/orders/{id}/pay`), `cmd/main.go` (rota + registry), migração `000013_order_payment_ref`
**Migração 000013:** `orders.idempotency_key TEXT UNIQUE`, `orders.reserved_order_reference TEXT` (ou reutilizar `payment_reference`).
**Fluxo:** valida ownership → gera `reserved_order_reference` (se ausente) + idempotency key → `registry.Get(cfg.PaymentProvider).StartFlow(...)` passando a referência → `payment_status=pending_payment` → grava `payment_reference=txid` + charge `main`.reference → devolve `FlowStatus` (QR/action).
**Aceite:**
- Chamado 2× no mesmo pedido → mesma charge (idempotente), não duplica cobrança.
- `reference_id` enviado ao gateway == `reserved_order_reference`.
- Pedido não-owner → 403.
**Deps:** WI-P1, WI-P2.

## WI-P4 · Webhook de pagamento + confirmação
**Objetivo:** confirmar liquidação e fazer Claim do estoque idempotentemente.
**Arquivos:** `internal/handlers/webhook_handler.go` (rota **pública** `POST /api/webhooks/payment`), `cmd/main.go`
**Fluxo:** `provider.VerifyWebhook(raw)` → assinatura inválida → 400; `EventUnknown` → 200 no-op; pago → `orderRepo.ConfirmOrderPayment(orderID)` (já existe: transição atômica + Claim) → publica `PaymentConfirmedEvent`.
**Aceite:**
- Assinatura válida + charge.paid → `payment_status=paid`, `status=processing`, estoque claimed.
- Assinatura inválida → 400, sem efeito.
- Webhook duplicado → 200 no-op (idempotente por `payment_status`).
**Deps:** WI-P1 (VerifyWebhook), WI-F2 (evento).

## WI-P5 · Saga de place-order com compensação
**Objetivo:** robustez transacional na parte que sai do banco (gateway, estoque, email) — rollback reverso.
**Arquivos:** `internal/checkout/{saga.go,state_store.go,saga_test.go}`; integra `orders`/`payment`/`variants`
**Modelo:**
```go
type Step struct { Name string; Run func(ctx)(Compensation,error) }
type Compensation func(ctx) error
type Saga struct { steps []Step; store StateStore }  // store: Postgres default (redis no Scale)
```
Sequência: `validar cart → reservar estoque (Reserve) → criar order (unpaid) → iniciar pagamento (StartFlow) → [assíncrono: webhook confirma → Claim]`. Se um passo síncrono falha, roda as compensações empilhadas em ordem reversa (`Release` estoque, `CancelCharge`, marcar order failed). `StateStore` persiste o contexto para retomar após retorno do gateway.
**Aceite:**
- Falha no `StartFlow` após reservar → `Release` do estoque + order `failed` (nada meio-reservado).
- Retomada: order em `pending_payment` sobrevive a restart (store Postgres) e é reconciliada por `FlowState`.
- Idempotency key evita place duplicado em retry.
**Deps:** WI-P1..P4, WI-F2. **Coordenar com S1/S2** (o saga conhece delivery/source).

## WI-S1 · Cart → Delivery → Item (delivery default)
**Objetivo:** costura de multi-delivery (envio + retirada) transparente para o caso simples.
**Arquivos:** migração `000014_deliveries`; `internal/models/delivery.go`; `internal/cart`, `internal/orders` (+ handlers, testes)
**Migração 000014:** `deliveries(id, cart_id NULL, order_id NULL, code, method, carrier, location_type TEXT DEFAULT 'address', address_id, shipping_cost_cents BIGINT DEFAULT 0)`. `cart_items.delivery_id` / `order_items.delivery_id` (backfill → delivery default por cart/order, depois NOT NULL).
**Comportamento:** cart cria 1 **delivery default** implícita; `AddItem` associa à default se o cliente não escolher. Frete/subtotais passam a ser por delivery (soma no cart/order).
**Aceite:**
- Cart de 1 delivery: API/UX inalterada (default transparente).
- Schema aceita 2 deliveries com métodos/endereços distintos.
- Total da order = Σ subtotais das deliveries + Σ frete.
**Deps:** toca cart/order — **antes do WI-P5**.

## WI-S2 · Sourcing port + estoque por source (source default)
**Objetivo:** generalizar estoque de 1 local para N; costura de multi-armazém.
**Arquivos:** migração `000015_sources`; `internal/models/source.go`; `internal/variants/repository.go` (+ mock/testes); `internal/sourcing/{allocator.go,allocator_test.go}`
**Migração 000015:** `sources(id, code, name, is_default BOOL)` (+ 1 default); `variant_stock(variant_id, source_id, stock, stock_reserved, PRIMARY KEY(variant_id,source_id))` migrando o estoque atual da variante para o source default. (Colunas `stock`/`stock_reserved` da variante ficam deprecated.)
**Interface:** `Reserve/Release/Claim(ctx, exec, variantID, sourceID, qty)`; `Sourcing.Allocate(cart) []Allocation{VariantID, SourceID, Qty}` com `SingleSourceAllocator` default. `order_items.source_id`.
**Aceite:**
- Reserve/Claim/Release por (variant, source) atômico; single-source transparente.
- `SingleSourceAllocator` aloca tudo do source default; interface pronta p/ multi-source.
**Deps:** toca variants/order — **antes do WI-P5**.

**➡ Fim da Phase 2:** PIX robusto (FlowStatus/saga/idempotência), fundação (Money/eventos), costuras (delivery/source) instaladas com defaults. Emails via event bus.

---

# FASE 5 — Growth (promoções, catálogo, busca)

## WI-G1 · Promoções / cupons (`AppliedDiscount` + port `VoucherHandler`)
Migração `discounts` (ou `applied_discounts(order_id/cart_id, level, type, applied_cents NEG, is_item_related, sort_order, campaign_code, coupon_code)`) + `cart.applied_coupon_codes`. Model `AppliedDiscount`. Port `VoucherHandler.Apply(cart, code) []AppliedDiscount` (**default no-op**). Item ganha `row_price_cents_with_discount` (rateio via `money.Allocate`). Core só agrega (`MergeDiscounts`); **nenhuma regra no core**. Aceite: cupom 10% aplica desconto cart-level rateado por item; cupom inválido → erro; desconto congelado no order.

## WI-G2 · Gift card via `Charge{Type:"giftcard"}`
Reusa WI-P2. Pagamento parcial (não desconto): `PaymentSelection` com charge `main` + `giftcard`; split via `money.SplitInPayables`. Aceite: order de R$100 com gift card R$30 → charge giftcard 30 + main 70; soma == total.

## WI-G3 · Produto: bundles reais, atributos dinâmicos, media usage
Migração: `products.type` (simple|configurable|bundle), `products.attributes JSONB`, `products.variant_variation_attributes TEXT[]`, `product_bundle_choices(product_id, min_qty, max_qty, required)` + `product_bundle_options`, `product_media(usage)`. Aceite: bundle valida seleção min/max/required; atributo novo sem migração; variante seleciona por atributos declarados.

## WI-G4 · Busca: `Filter` port + facets Postgres
`internal/search/{service.go,filter.go}`: `Filter` interface variádica (KeyValue/Sort/Query/Pagination); `Result{Hits, Facets(List/Tree/Range com Count), NumPages, NumResults, Suggestion}`. Adapter Postgres (`tsvector`/`pg_trgm` + facets via `GROUP BY/COUNT`). Aceite: busca "camiseta" com facet categoria (contagem) + faixa de preço + sort; paginação com NumResults. Escala: adapter Meilisearch (mesma porta).

## WI-G5 · Frete real + endereços billing/shipping
`ShippingProvider` adapter Correios/Melhor Envio (porta já existe). `addresses.is_default_billing`/`is_default_shipping`. Emails transacionais (order/pagamento/despacho) como subscribers do event bus.

---

# FASE 7 — Scale (multi-delivery, sourcing, B2B)

- **WI-C1 · Multi-delivery ativo:** UX/endpoints para 2+ deliveries (envio + retirada na loja), `location_type` (address/store/pickup-point), carrier/prazo por delivery. Usa a costura WI-S1.
- **WI-C2 · Sourcing multi-armazém:** `MultiSourceAllocator` (proximidade/CEP/prioridade, deduz cart), `StockProvider` por source, restrição por source. Usa a costura WI-S2.
- **WI-C3 · Preço por contexto (B2B):** `PriceResolver.Resolve(product, ctx) PriceInfo`; tabela de preço por `customer_group` (varejo/atacado/B2B); janelas promocionais.
- **WI-C4 · Loyalty:** `Charge{Type:"loyalty"}` + acúmulo/resgate de pontos por pedido.
- **WI-C5 · Saga store distribuído:** adapter redis + locker para múltiplas instâncias e redirects 3DS/wallet.

---

# FASE 8 — Enterprise (multi-tenant, multi-currency, GraphQL)

- **WI-E1 · Multi-tenant:** `tenant_id` (default single) em todas as tabelas de negócio + middleware de resolução de tenant + RBAC por loja. Costura já prevista; ativar quando SaaS multi-loja.
- **WI-E2 · Multi-currency real:** popular tabela de moeda (WI-F1), preço/estoque por moeda, guard já pronto.
- **WI-E3 · GraphQL:** camada `interfaces/graphql` opcional sobre os application services REST (não substitui REST).
- **WI-E4 · Adapters ERP/CRM:** `Customer`/`Cart`/`StockProvider` apontando para sistemas externos.

---

## Marcos (Definition of Done por fase)
- **Phase 2:** checkout PIX de ponta a ponta (pay → QR → webhook → paid → estoque claimed), com idempotência, saga com rollback e emails por evento. Costuras delivery/source presentes com defaults. Frente frontend: `PIXCheckout` consumindo `FlowStatus`.
- **Phase 5 (Growth):** cupom + gift card + bundle + busca facetada + frete real funcionando; perfil "Growth" ligável por config.
- **Phase 7 (Scale):** multi-delivery + multi-armazém + B2B pricing ligáveis; saga em redis.
- **Phase 8 (Enterprise):** multi-tenant + multi-currency + GraphQL opcionais.

## Convenções de execução
- Cada WI é uma branch/PR isolado com testes. Migrações sequenciais (próxima livre após 000011 = 000012).
- Costuras (S1/S2) e modelo de pagamento (P2) são **aditivos** (default implícito) — não quebram o caminho atual.
- Feature flags por ENV: `ECOMMERCE_PROFILE`, `FEATURE_PROMOTIONS`, `FEATURE_MULTI_DELIVERY`, `FEATURE_MULTI_SOURCE`, `PAYMENT_PROVIDER`, `SEARCH_BACKEND`, `SAGA_STORE`.
- Nada de porta sem ≥2 implementações plausíveis; nada de flag sem default seguro.
