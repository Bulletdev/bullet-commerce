# PRD - bullet-commerce: Arquitetura de Capacidades Progressivas (benchmark Flamingo Commerce)

**Data:** 2026-07-22 · **v2** (reescrito) · **Base:** auditoria de `flamingo.me/flamingo-commerce/v3`
**Escopo:** transformar o bullet-commerce numa **API de e-commerce robusta e completa**, que suporta **múltiplos perfis de loja** e **escala progressivamente** conforme o negócio cresce - sem reescrita a cada salto.

> **Pivô em relação à v1:** a v1 tratou multi-delivery, sourcing, split de pagamento, promoções, PriceContext, multi-tenancy etc. como "overkill para single-store". **Isso estava errado para a visão do projeto.** A meta é uma API completa que se ajusta ao tamanho da operação. Portanto essas capacidades **não são descartadas - são projetadas desde já como portas plugáveis, com implementação default enxuta e adapters ricos ligados por perfil/config.**

---

## 1. Visão e princípio arquitetural

**Meta:** uma única base de código que atende desde o freela com um catálogo pequeno até uma operação com múltiplos depósitos, B2B, fidelidade e multi-loja - **habilitando capacidades por configuração, não por reescrita.**

O Flamingo prova que isso se consegue com **três mecanismos** (que vamos adotar de forma idiomática em Go, sem o peso do dingo/GraphQL):

1. **Ports & Adapters em todo lugar.** Toda capacidade não-trivial é uma **interface** (porta) com um **adapter default** simples e adapters ricos opcionais. O core depende da porta, nunca da implementação.
2. **Feature flags + estratégias por config.** Cada capacidade tem um flag e (quando aplicável) uma estratégia selecionável no wiring (`main.go`). Default = "off"/simples. Ligar = trocar o adapter e o flag, sem tocar no core.
3. **Padrão "default X" para transparência.** O modelo de dados suporta a forma rica desde já, mas expõe um **default implícito** para que o caso simples não sinta a complexidade. Já validamos isso com a **variante default** (produto de 1 variante é transparente). Estendemos o mesmo para **delivery default** e **source default**.

> **Regra de ouro:** *projete o teto, implemente o piso por default, suba o piso por perfil.* As costuras (portas + schema) acomodam a forma rica; a implementação default entrega a forma simples; o perfil sobe o nível ligando adapters.

### Onde já estamos à frente do Flamingo
- **Dinheiro:** `int64` centavos + `currency` (o `order` do Flamingo ainda usa `float64`). Mantemos - e o robustecemos com um Money value object (§4.3).
- **Estoque:** `Reserve/Claim/Release` atômico por variante - já é a primitiva de **compensação (saga)** e de **sourcing de 1 local**. Generaliza direto para multi-source (§4.5).
- **Variante default:** o padrão de transparência que replicamos para delivery/source.

---

## 2. Perfis de e-commerce (capability presets)

Perfis são **presets de capacidades** selecionáveis por config (`ECOMMERCE_PROFILE` + flags finos). Um perfil não trava nada - é o ponto de partida; qualquer flag pode ser ligado individualmente. Uma loja **migra de perfil sem reescrita**, só habilitando adapters.

| Capacidade | **Starter** | **Growth** | **Scale** | **Enterprise** |
|---|:--:|:--:|:--:|:--:|
| Catálogo (produto simples + variantes) | ✅ | ✅ | ✅ | ✅ |
| Estoque Reserve/Claim/Release (1 source) | ✅ | ✅ | ✅ | ✅ |
| Pagamento (1 provider, FlowStatus) | ✅ | ✅ | ✅ | ✅ |
| Frete (tabela) | ✅ | ✅ | ✅ | ✅ |
| Busca SQL | ✅ | ✅ | → engine | → engine |
| Cupons / desconto (`AppliedDiscount`) | – | ✅ | ✅ | ✅ |
| Emails transacionais (event bus) | – | ✅ | ✅ | ✅ |
| Pagamento misto / gift card (`Charge`, split) | – | ✅ | ✅ | ✅ |
| Bundles reais · atributos dinâmicos | – | ✅ | ✅ | ✅ |
| Frete real (Correios/Melhor Envio) | – | ✅ | ✅ | ✅ |
| Busca facetada (facets) | – | ✅ | ✅ | ✅ |
| Multi-delivery (envio + retirada) | – | – | ✅ | ✅ |
| Sourcing multi-armazém | – | – | ✅ | ✅ |
| Checkout saga com store distribuído (redis) | mem/pg | mem/pg | ✅ | ✅ |
| Fidelidade / loyalty (`ChargeType`) | – | – | ✅ | ✅ |
| Preço por contexto (customer group / B2B) | – | – | ✅ | ✅ |
| GraphQL (interface opcional) | – | – | ○ | ✅ |
| Multi-tenant (`tenant_id`) | – | – | ○ | ✅ |
| Multi-currency real | – | – | ○ | ✅ |

Legenda: ✅ ligado · – desligado (porta presente, adapter no-op/simples) · ○ opcional · → evolui.

**Starter ≈ onde estamos hoje.** O trabalho deste PRD é (1) instalar as **costuras** para que Growth→Enterprise sejam config, e (2) implementar os adapters por fase.

---

## 3. Costuras estruturais que precisam entrar CEDO

Estas são as mudanças que, se adiadas, forçam reescrita depois. Entram na fundação (mesmo com adapter simples), porque mexem no **modelo de dados** e nas **assinaturas do core**.

### 3.1 Hierarquia `Cart → Delivery → Item` (com delivery default)
Hoje `cart_items` pertencem direto ao cart. O Flamingo põe itens sob **deliveries** (`cart/domain/cart/delivery.go`), cada uma com seu endereço, método, carrier e subtotais. É o que habilita "parte enviada, parte retirada na loja".

**Ajuste:** introduzir `deliveries` (uma linha `delivery` por cart, com `code`, `method`, `carrier`, `location_type`, `address_id`). `cart_items.delivery_id` (NOT NULL). **Default delivery** criada implicitamente (como a variante default) → Starter/Growth nunca veem. Scale liga multi-delivery. Migração aditiva; o handler de cart resolve a delivery default quando o cliente não escolhe.

### 3.2 Porta de sourcing + `source_id` (com source default)
`sourcing/domain/service.go`: `StockProvider.GetStock(product, source)` + `AllocateItems` decide de qual local sai cada unidade. Nosso `Reserve/Claim/Release` é sourcing de 1 local implícito.

**Ajuste:** (1) tabela `sources` (locais de estoque) com um **source default**; (2) estoque da variante passa a ser **por source** (`variant_stock(variant_id, source_id, stock, stock_reserved)`) - com uma linha default; (3) `Reserve/Claim/Release(exec, variantID, sourceID, qty)`; (4) porta `Sourcing.Allocate(cart) []Allocation` com um `SingleSourceAllocator` default. Order_item ganha `source_id`. Scale injeta um allocator multi-source. **O modelo por-source desde já evita reescrever o estoque depois.**

### 3.3 Money value object (precisão por moeda + split)
`price/domain/price.go`: `SplitInPayables` (reparte um valor em N pagáveis cuja soma bate exato) + arredondamento por `currency.Fraction` (2 BRL, 0 JPY, 3 BHD).

**Ajuste:** `internal/money` com `Money{Cents int64, Currency string}` (mantemos int64 - superior ao `big.Float` para nós), `SplitInPayables(n) []Money`, `Allocate(weights) []Money` (rateio de desconto/frete por item), precisão derivada de tabela de moeda, e **guard de moeda** (erro em operação entre moedas diferentes). Multi-currency real vira só "popular a tabela".

### 3.4 `PaymentSelection` + `Charge{Type, Reference}`
`cart/domain/cart/paymentselection.go`: pagamento modelado como **seleção com splits** por método/tipo/charge, com `IdempotencyKey`. Gift card é `ChargeType`, não desconto.

**Ajuste:** modelar pagamento como `PaymentSelection{ Charges []Charge }` onde `Charge{Type: "main"|"giftcard"|"loyalty", Method, AmountCents, Reference}`. Default = 1 charge `main`. Isso destrava pagamento misto e fidelidade **sem refactor** - é a decisão de modelagem mais cara de adiar. Idempotency key no início do fluxo.

### 3.5 Event bus interno
`OrderPlacedEvent`, `PaymentConfirmedEvent` publicados **após** persistir (`DeferEvents`). Side-effects (email, tracking, NF-e, analytics, BI) assinam eventos.

**Ajuste:** `internal/events` com um publisher in-process (canal + handlers registrados no wiring). Emails/NF-e/tracking viram subscribers. Escala para NATS/Redis Streams trocando o adapter. Mantém o core do checkout limpo e é a base para as features de BI (Curva ABC, Sales Velocity - ver memória de inspiração).

---

## 4. Capacidades - porta, default e caminho de escala

Para cada uma: **porta** · **adapter default (piso)** · **adapter de escala (teto)** · **flag/perfil**.

### 4.1 Pagamento - máquina de estados com `Action` (Growth+, costura desde já)
`payment/domain/payment.go`: `WebCartPaymentGateway` com `StartFlow → FlowStatus(polling) → Confirm → Cancel(reason)`; `FlowStatus{Status, Action, ActionData}` instrui o front (`display_pix`/`redirect`/`show_iframe`/`wallet`); estados `unapproved · waiting_for_customer · approved · completed · failed · cancelled`.
- **Porta:** estender `payment.Provider` com `StartFlow`/`FlowStatus`/`Cancel(reason)` e `FlowStatus{Status, Action, ActionData}`.
- **Default:** propay/openpix devolvendo `Action=display_pix`.
- **Escala:** cartão (iframe/SDK), boleto (redirect), wallets (`trigger_client_sdk`).
- Separar `approved` (gateway) de `paid` (liquidado) - crítico p/ PIX. Idempotency key. **Refina o `docs/payment-provider.md`.**

### 4.2 Checkout - saga com store plugável (todos os perfis)
`checkout/domain/placeorder/process/`: saga com rollback reverso; contexto persistível.
- **Porta:** `SagaStateStore` (persistência do contexto) + a saga `validar → reservar → cobrar → criar order → confirmar` com pilha de compensações (reutiliza Reserve/Release/Claim + Cancel de pagamento).
- **Default:** store **in-memory/Postgres** (síncrono; PIX/boleto retomam por `payment_reference`).
- **Escala:** store **redis** + `locker` distribuído quando houver múltiplas instâncias e redirects de 3DS/wallet.
- Idempotency key + `reserved_order_reference` gerada cedo e passada ao gateway.

### 4.3 Promoções / cupons / gift card (Growth+)
`cart/domain/cart/discount.go`, `giftcard.go`: `AppliedDiscount{Applied(negativo), Type, IsItemRelated, SortOrder, CampaignCode, CouponCode}` em item/delivery/cart/frete; cupons no cart; **cálculo atrás de um port `VoucherHandler` (default no-op)** - o core só agrega o resultado.
- **Porta:** `VoucherHandler.Apply(cart, code) []AppliedDiscount` + `PromotionEngine` (opcional).
- **Default:** no-op (sem promoção) - Starter.
- **Escala:** engine de regras (percent/fixed, item/cart/frete, min-cart, combos, por-influencer). **Nunca no core** - modelar só o resultado (`AppliedDiscount`).
- Item ganha `row_price_cents_with_discount` (rateio via `money.Allocate`). Gift card = `Charge{Type:"giftcard"}` (§3.4), separado de desconto.

### 4.4 Produto - tipos, atributos dinâmicos, bundles (Growth+)
`product/domain/*`: `BasicProduct` interface → Simple / **Configurable** (`VariantVariationAttributes`) / **Bundle** (`Choices` com min/max/required). Atributos dinâmicos (`map[string]Attribute`). Media com `Usage`.
- **Porta/model:** `product.type` discriminador; `attributes JSONB` (fim das migrations por atributo); `variant_variation_attributes` (quais chaves geram a UI de seleção); `product_bundle_choices` (kits reais); `media(usage)`.
- **Default:** simple + configurable (o que temos).
- **Escala:** bundle, `PriceInfo` contextual (§4.7).

### 4.5 Sourcing multi-armazém (Scale+) - costura em §3.2
- **Default:** `SingleSourceAllocator` (source default).
- **Escala:** `MultiSourceAllocator` (aloca por proximidade/CEP/prioridade, deduz o que já está no cart - `allocateFromSources`). Liga com a delivery (§3.1).

### 4.6 Busca - `Filter` port + facets (Growth+)
`search/domain/*`: `Filter` interface variádica (KeyValue/Sort/Query/Pagination); `Result` com `Facets` (List/Tree/Range, `Count`/`Selected`), `NumPages/NumResults`, `Suggestion`.
- **Porta:** `SearchService.Search(filter...) Result` com `Filter` polimórfico.
- **Default:** adapter **Postgres** (`ILIKE`/`tsvector`/`pg_trgm` + facets via `GROUP BY/COUNT`).
- **Escala:** adapter **Meilisearch/Elastic** (mesma porta, sem mudar o front).

### 4.7 Preço por contexto - customer group / B2B (Scale+)
`product` `PriceContext` (canal/customer-group/locale) + `PriceInfo` (default + discounted + campanha + janela temporal).
- **Porta:** `PriceResolver.Resolve(product, ctx) PriceInfo`.
- **Default:** preço único.
- **Escala:** tabela de preço por `customer_group` (atacado/varejo/B2B), janelas promocionais.

### 4.8 Multi-tenant (`tenant_id`) e multi-currency (Enterprise) - costura opcional
Sob a lente "escala conforme expande", multi-tenancy deixa de ser "só com 3+ clientes" e passa a ser uma **capacidade projetada**: um `tenant_id` opcional (default `NULL`/single) em todas as tabelas de negócio + um middleware de resolução de tenant. **Não obriga** o Starter a nada (default single-tenant), mas evita a reescrita de queries quando o SaaS multi-loja chegar. Multi-currency = popular a tabela de moeda do §3.3.

### 4.9 Cliente / endereços (Growth+)
- `is_default_billing` / `is_default_shipping` separados (cobrança ≠ entrega); `PersonData` (birthday p/ restrição de idade) opcional.

### 4.10 Arquitetura / DI idiomática (contínuo)
- Separar **porta de leitura** (repositório) da **porta de mutação** (operações de carrinho - `ModifyBehaviour`).
- **Estratégias por config** no wiring (merge de cart no login, política de reserva, allocator).
- `RestrictionService` multi-valor (limites de compra plugáveis: qty máx por item/pedido, por-cliente).
- Span de trace por operação; mocks via `//go:generate`.

---

## 5. Roadmap (arquitetura-primeiro)

A ordem prioriza **instalar as costuras** antes de encher de features, para que cada capacidade posterior seja aditiva.

- **Phase 2 - Pagamento robusto + fundação (em andamento, reforçada):**
  costuras §3.3 (Money VO), §3.4 (PaymentSelection/Charge), §3.5 (event bus); capacidade §4.1 (FlowStatus/Action) + §4.2 (saga com store Postgres) + idempotency/reserved ref. Emails viram subscribers do event bus.
- **Phase 5 - Growth (promo + catálogo + busca):**
  §4.3 (AppliedDiscount + VoucherHandler + gift card via Charge), §4.4 (bundles/atributos dinâmicos/media), §4.6 (Filter port + facets Postgres), §4.9 (endereços billing/shipping). Frete real (Correios).
- **Phase 7 - Scale (multi-delivery + sourcing + B2B):**
  costuras §3.1 (Cart→Delivery→Item) e §3.2 (sources) ativadas; §4.5 (multi-source allocator), §4.7 (preço por contexto), saga com store redis, loyalty via ChargeType.
- **Phase 8 - Enterprise:** §4.8 (tenant_id + multi-currency), GraphQL como interface opcional, adapters ERP/CRM, engine de promoção avançado.

> As **costuras** §3.1–§3.5 podem entrar cedo (schema aditivo + porta) mesmo que o adapter rico venha na fase da capacidade. Quanto antes a costura, menor o custo de ligar a capacidade.

---

## 6. Decisões arquiteturais

1. **Capacidade = porta + adapter default + flag.** Nada de capacidade sem porta; nada de "off" sem um default no-op/simples. O core nunca conhece o adapter concreto.
2. **Padrão "default X" para transparência.** Variante default (feito) → delivery default → source default. O caso simples não sente a estrutura rica.
3. **Modelar resultado, não engine.** Promoção e sourcing: o core carrega/agrega `AppliedDiscount`/`Allocation`; o *cálculo* vive num port plugável. Zero regra de negócio de campanha no core.
4. **`int64` centavos + Money VO** (não `big.Float`): precisão por moeda + `SplitInPayables`. Multi-currency = dados, não reescrita.
5. **`Charge{Type,Reference}` e `PaymentSelection` desde a Phase 2** - a modelagem mais cara de adiar; destrava misto/gift card/loyalty sem refactor.
6. **Saga com store plugável** - robustez (rollback reverso) sempre; persistência (mem/pg → redis) por perfil.
7. **Event bus desde a Phase 2** - side-effects (email/NF-e/BI) desacoplados; escala trocando o transporte.
8. **Schema aditivo com defaults** - `delivery_id`/`source_id`/`tenant_id` entram com um default implícito, nunca quebrando o caminho simples.
9. **Filas/workers no Postgres com `FOR UPDATE SKIP LOCKED` + `LIMIT` + CTE.** Para todo processamento concorrente de *linhas-tarefa* - cleanup de orders órfãs (`pending_payment` 30 min) e abandonadas (`unpaid` 15 min), outbox do event bus (§3.5), e qualquer job queue futura (perfil Scale, múltiplas instâncias) - o padrão é:
   - **`SKIP LOCKED`** - o worker ignora linhas já travadas por outra transação; workers **não se bloqueiam** mutuamente, eliminando gargalo em alta concorrência.
   - **`LIMIT` (bounded rows)** - impede um worker de pegar a fila inteira; distribui a carga de forma **justa** entre N workers.
   - **CTE (obrigatória)** - o `SELECT ... FOR UPDATE SKIP LOCKED LIMIT n` fica dentro de uma CTE e o `UPDATE` referencia a CTE. **Sem a CTE**, `LIMIT` + `SKIP LOCKED` dentro de um `UPDATE` pode, na fase de planejamento, **travar mais linhas do que o `LIMIT`** especifica - bug conhecido do Postgres.

   ```sql
   WITH batch AS (
     SELECT id FROM orders
     WHERE payment_status = 'pending_payment' AND payment_reference IS NULL
       AND created_at < NOW() - INTERVAL '30 minutes' AND deleted_at IS NULL
     ORDER BY created_at
     FOR UPDATE SKIP LOCKED
     LIMIT 100
   )
   UPDATE orders o
     SET status = 'cancelled', payment_status = 'failed', updated_at = NOW()
     FROM batch WHERE o.id = batch.id
   RETURNING o.id;   -- os ids liberam a reserva de estoque na mesma tx
   ```

   **Distinção crítica:** SKIP LOCKED é para **filas/workers** (N consumidores disputando um conjunto de linhas), **não** para a reserva de estoque. A reserva continua `UPDATE product_variants SET stock_reserved = stock_reserved + qty WHERE id = $1 AND (stock - stock_reserved) >= qty` numa **única linha** - atômico por construção, sem lock explícito (decisão do `PRD-desacoplamento`: um-campo, sem linha-por-unidade). SKIP LOCKED entra quando **múltiplas goroutines/instâncias** varrem uma tabela de tarefas - o que já vale hoje para o cleanup (evita duas goroutines processarem a mesma order) e é obrigatório no Scale com várias instâncias do serviço.

---

## 7. Riscos

| Risco | Mitigação |
|---|---|
| Complexidade sobe o custo do caso simples | Padrão "default X" (delivery/source default) + perfis: Starter só vê o piso |
| Costura adiada força reescrita (estoque, cart, pagamento) | §3 entra cedo (schema aditivo + porta), mesmo com adapter simples |
| Perde/cria centavo em rateio | Money VO com `SplitInPayables`/`Allocate` (§3.3) antes de desconto/frete multi-item |
| "Cobrou mas não criou pedido" | Saga com rollback reverso (§4.2) sobre Reserve/Release/Claim |
| Engine de promoção incha o core | `VoucherHandler` port; core só agrega `AppliedDiscount` |
| Multi-tenant tarde reescreve queries | `tenant_id` opcional com default single desde a fundação (§4.8) |
| Over-abstração prematura de portas pouco usadas | Porta só onde há ≥2 implementações plausíveis (pagamento, frete, sourcing, busca, voucher, price, saga-store); resto é struct |

---

## 8. Referência - arquivos-fonte do Flamingo

- Pagamento: `payment/domain/payment.go`, `payment/interfaces/webpayment.go`
- Checkout saga: `checkout/domain/placeorder/process/{process,state}.go`, `checkout/module.go`
- Preço/split: `price/domain/price.go` (`SplitInPayables:387`, rounding:303)
- Desconto/cupom/giftcard: `cart/domain/cart/{discount,giftcard,paymentselection}.go`
- Cart / delivery / totals: `cart/domain/cart/{cart,delivery,item}.go`, portas `cartServicePorts.go` (`ModifyBehaviour:52`)
- Order snapshot + eventos: `order/domain/{order,events}.go`
- Produto: `product/domain/{productTypeConfigurables,productTypeBundle,productBasics}.go`
- Busca: `search/domain/{service,filter}.go`
- Sourcing: `sourcing/domain/service.go` (`allocateFromSources:401`)
- DI/estratégias/multibind: `*/module.go`
