# PRD — Catálogo v2 (bullet-commerce)

**Data:** 2026-07-23 · **Status:** proposta (validação) · **Escopo:** completar o modelo de produto/variante para operar uma loja real no Brasil (mídia, fiscal, frete, merchandising, atributos facetáveis).

Gatilho: auditoria do JSON de produto atual. O catálogo cobre o básico (produto + variante + estoque por source + atributos JSONB) mas faltam campos **legalmente/operacionalmente obrigatórios** (imagem, peso/dimensão, NCM) e de **maturidade de catálogo** (slug, status, de/por, barcode, atributos normalizados).

---

## 1. Estado atual (corrigindo o que a auditoria supôs faltar)

Antes do gap analysis, três itens que a auditoria listou como faltantes **já existem**:

| Suposto gap | Realidade |
|---|---|
| `deleted_at` (soft delete) | **Existe** em `products` e `product_variants` (migration 000008/000010). Não aparece no JSON por ser `omitempty` quando null. |
| "estoque merece tabela própria p/ múltiplos depósitos" | **Feito** (migration 000020): `variant_stock(variant_id, source_id, stock, stock_reserved)` + `sources`. Os campos `stock`/`stock_reserved` na variante são **deprecated** (legados no read path de display). |
| "sem version p/ concorrência otimista na reserva" | A reserva é `UPDATE ... WHERE (stock - stock_reserved) >= qty` — compare-and-set atômico; a cláusula WHERE **é** o controle de concorrência. `version` não é necessário para o caso de estoque (ver §4.8 para o caso de edição concorrente geral). |

**Schema atual relevante:**
- `products`: id, name, description, **price_cents**, currency, category_id, **stock** (redundante — ver §4.6), featured, **type** (simple/configurable/bundle), **attributes** JSONB, **variant_variation_attributes** TEXT[], deleted_at, timestamps.
- `product_variants`: id, product_id, sku, attributes JSONB, **price_cents (nullable — herda)**, currency, stock/stock_reserved (deprecated), deleted_at, timestamps.
- `variant_stock(variant_id, source_id, stock, stock_reserved)` — estoque real, por depósito.
- `categories` (FK simples via `products.category_id`).

---

## 2. Princípios

1. **Aditivo com default.** Colunas novas entram com default seguro; nada quebra o caminho atual (padrão "default variant/source" já validado).
2. **Money = int64 centavos + currency** (mantém).
3. **A API não hospeda arquivos.** Mídia é **URL** (CDN/bucket externo). Upload com presigned-URL é fora deste PRD (backlog).
4. **Fiscal fica no catálogo só o que a NF-e precisa como cadastro** (NCM/CEST/origem). CFOP/ICMS/alíquota são calculados na emissão (provedor NF-e), não armazenados no produto.
5. **DDD:** invariante de estoque permanece na variante/source; catálogo v2 só acrescenta atributos ao agregado Product.

---

## 3. Gap analysis + design

### 3.1 Mídia / imagens 🔴 (Tier 1 — buraco nº 1: hoje NÃO existe imagem)

Nova tabela `product_media`:
```sql
product_media(
  id uuid PK, product_id uuid FK,
  variant_id uuid NULL FK,        -- NULL = imagem do produto; setado = imagem da variante (ex.: cor)
  url TEXT NOT NULL, alt TEXT,
  kind TEXT NOT NULL DEFAULT 'image' CHECK (kind IN ('image','video')),
  position INT NOT NULL DEFAULT 0,
  created_at, updated_at
)
```
- No JSON: produto ganha `media: [{url, alt, position, variant_id?}]`; variante expõe suas imagens (as com seu `variant_id`) ou um `image_url` primário derivado.
- **Decisão a validar:** imagem-por-variante via `product_media.variant_id` (flexível, N imagens/variante) vs `variant.image_id` FK (1 imagem primária). Recomendo `product_media.variant_id` + um helper `PrimaryImage()` (a de menor position).

### 3.2 Peso & dimensões 🔴 (Tier 1 — frete real)

`weight_grams INT` + `length_mm/width_mm/height_mm INT` em **product** (default) e **variant** (override — variantes pesam diferente). O cálculo de frete (`/shipping/calculate`) passa a usar as dimensões da variante quando presentes, senão do produto.
- **Decisão a validar:** dimensões como 4 colunas vs um JSONB `dimensions`. Recomendo colunas (query/validação simples).

### 3.3 Fiscal BR 🔴 (Tier 1 — NF-e obrigatória)

Em `products` (NCM é tipicamente por produto): `ncm CHAR(8)`, `cest CHAR(7) NULL`, `origem SMALLINT NOT NULL DEFAULT 0 CHECK (origem BETWEEN 0 AND 8)` (origem da mercadoria SEFAZ), `unit TEXT DEFAULT 'UN'` (unidade comercial).
- **Decisão a validar:** NCM/CEST por produto (99% dos casos) vs por variante (raro: variantes de material diferente com NCM distinto). Recomendo por produto, com override opcional por variante só se aparecer demanda.
- Não armazenar CFOP/CST/alíquota — vem do provedor NF-e na emissão.

### 3.4 Merchandising & publicação 🟡 (Tier 2)

- **`status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('draft','active','archived'))`** — separa "publicado" de `featured` (destaque) e de `deleted_at` (LGPD). Catálogo público filtra `status='active'`.
- **`slug TEXT UNIQUE`** — URL/SEO. Backfill: slugify(name)+sufixo. Público busca por slug.
- **SEO:** `meta_title TEXT`, `meta_description TEXT` (ou um JSONB `seo`). Recomendo colunas.
- **`brand_id uuid NULL FK`** → nova tabela `brands(id, name, slug, logo_url)`.
- **`compare_at_price_cents BIGINT NULL`** (de/por) em product e variant. Regra: exibe "de X por Y" quando `compare_at_price_cents > price_cents`.
- **Categorias N:N:** nova `product_categories(product_id, category_id, PK(product_id,category_id))`. **Decisão a validar:** manter `products.category_id` como "categoria primária" (breadcrumb/URL) + N:N secundária, OU migrar 100% pra N:N (mais limpo, mais refactor). Recomendo: manter `category_id` como primária + adicionar N:N.

### 3.5 Variante 🟡 (Tier 2)

- **`barcode TEXT` (GTIN/EAN)** — NF-e, marketplace, bipagem no estoque.
- **`active BOOL NOT NULL DEFAULT TRUE`** — desativar uma variante sem deletar.
- **`position INT NOT NULL DEFAULT 0`** — ordem de exibição.
- **`compare_at_price_cents BIGINT NULL`**.
- **Materializar preço (§4 estrutural).**

### 3.6 Estoque — estrutural

- **Remover `products.stock`** (redundante). O estoque real vive em `variant_stock` por source; mesmo produto "simple" tem variante default. Deixar os dois é fonte de divergência (o próprio JSON auditado mostra produto `stock:120` ≠ variante `stock:30`). Migração de remoção em duas etapas (deprecar → dropar após read path migrar).
- **Expor `stock_available`** no JSON da variante (= Σ `stock - stock_reserved` across sources; já calculado por `AvailableForVariant`, só falta serializar). Produto configurável expõe `total_available` = Σ variantes.
- **`stock_policy` por variante:** `TEXT DEFAULT 'deny' CHECK (IN ('deny','backorder'))` — `deny` (atual, sem oversell) vs `backorder` (permite vender sem estoque). Só muda a validação da reserva (`backorder` pula o guard de disponibilidade).

### 3.7 Atributos normalizados — a decisão estrutural mais importante 🟡

Hoje `attributes` é JSONB livre → **não faceta, não valida, não ordena** ("M" não vem antes de "G", cor não tem hex). Proposta **híbrida**:
```sql
attribute(id, code UNIQUE, label, kind TEXT CHECK (IN ('select','color','text')))     -- ex.: tamanho, cor
attribute_value(id, attribute_id FK, value, label, hex TEXT NULL, position INT)        -- M/G/GG; preto #000
variant_attribute_value(variant_id FK, attribute_value_id FK, PK(variant_id, av_id))   -- liga variante↔valores
```
- **`variant_variation_attributes`** do produto passa a referenciar `attribute.code` (validado), não string solta.
- **JSONB `attributes` fica** para metadados livres do produto (material, cuidados) — o que NÃO é navegável.
- Ganho: busca facetada real (`/search` hoje só faceta categoria/preço), validação, UI de seleção ordenada e com swatch de cor.
- **Decisão a validar:** é a migração mais cara (backfill dos JSONB atuais → tabelas normalizadas, com mapeamento ambíguo de chaves livres). Fazer agora (junto) ou como fase própria depois do Tier 1?

### 3.8 Concorrência otimista (`version`)

A reserva de estoque já é segura (§1). Falta optimistic locking para **edição concorrente de produto/order no admin** (dois admins editando o mesmo produto). Proposta: `version INT NOT NULL DEFAULT 0` em `products`/`orders`; UPDATE com `WHERE id=$1 AND version=$2` → RowsAffected 0 = conflito (412). Baixa prioridade; não bloqueia venda.

---

## 4. Materializar preço da variante (decisão estrutural)

Hoje `variant.price_cents` é nullable e herda do produto — obriga fallback (`variant.price ?? product.price`) em todo consumidor. **Proposta: materializar** (variant.price_cents NOT NULL; backfill com o preço do produto onde null). Tradeoff: mudar o preço do produto exige **fan-out** para as variantes que "herdavam" — resolver com um flag `price_inherited BOOL` (quando true, updates do produto propagam; quando o admin edita a variante, vira false). Elimina fallback no read path e alinha com o resto (dinheiro sempre explícito na linha vendável).

---

## 5. Plano de migrations (aditivo, numeração a partir de 000021)

| Migration | Conteúdo | Tier |
|---|---|---|
| `000021_product_media` | tabela `product_media` (+ variant_id) | 1 |
| `000022_shipping_dims` | `weight_grams`/`*_mm` em products + variants | 1 |
| `000023_fiscal_br` | `ncm`/`cest`/`origem`/`unit` em products | 1 |
| `000024_merchandising` | `status`/`slug`/`meta_*`/`brand_id`/`compare_at_price_cents` + `brands` + `product_categories` N:N | 2 |
| `000025_variant_catalog` | `barcode`/`active`/`position`/`compare_at_price_cents`/`stock_policy` + materializar `price_cents` | 2 |
| `000026_attributes_normalized` | `attribute`/`attribute_value`/`variant_attribute_value` + backfill | 2 (fase própria) |
| `000027_drop_product_stock` | remove `products.stock` após read path migrar | 2 (última) |

---

## 6. Roadmap

- **Fase 1 — "vender legal" (Tier 1):** media, dims/peso, fiscal (000021-23). Sem isso não emite NF-e nem calcula frete real. Paralelizável (tabelas/campos disjuntos: media / dims / fiscal).
- **Fase 2 — catálogo maduro (Tier 2):** merchandising, variante rica, remover stock redundante (000024-25, 27).
- **Fase 3 — atributos normalizados (000026):** habilita busca facetada por tamanho/cor; migração mais delicada (backfill), fase isolada.

---

## 7. Decisões arquiteturais

1. **Mídia é URL, não upload** (CDN externo); upload/presigned = backlog.
2. **Fiscal mínimo no cadastro** (NCM/CEST/origem/unit); o resto é da emissão.
3. **`category_id` primária + N:N secundária** (não migrar 100% agora).
4. **Materializar preço da variante** com flag `price_inherited` para o fan-out.
5. **Remover `products.stock`** — fonte única = `variant_stock`.
6. **Atributos híbridos:** normalizados para variação (size/color), JSONB para metadado livre.
7. **`version` só onde há edição concorrente** (product/order admin), não no estoque.

---

## 8. Questões abertas (validar antes de codar)

- **Mídia:** a loja vai subir arquivo pela API (precisa storage + presigned URL) ou só referenciar URLs de um CDN que ela já gerencia? Isso muda o escopo da mídia.
- **NCM por produto ou variante?** (recomendo produto; override por variante só sob demanda.)
- **Atributos normalizados agora ou fase 3?** O backfill dos JSONB livres atuais tem mapeamento ambíguo — precisa de uma tabela de-para manual ou heurística.
- **Categorias:** manter `category_id` primária ou migrar tudo pra N:N?
- **`stock_available` no JSON:** por variante e um agregado no produto configurável — confirmar o shape esperado pelo frontend.
- **Simple products:** com `products.stock` removido, o produto "simple" lê estoque da variante default — confirmar que nenhum read path (ex.: `/products` listagem) quebra.

---

## 9. Riscos

| Risco | Mitigação |
|---|---|
| Remover `products.stock` quebra listagem/display | Migração em 2 etapas (deprecar → dropar) + atualizar read path pra `variant_stock` antes |
| Backfill de atributos normalizados ambíguo | Fase 3 isolada; começar só com `tamanho`/`cor` conhecidos; JSONB continua pra o resto |
| Materializar preço exige fan-out no update do produto | Flag `price_inherited` controla a propagação |
| Slug colidindo no backfill | `slug UNIQUE` + sufixo incremental no backfill |
| Escopo grande de uma vez | Faseamento Tier 1 (legal) → Tier 2 → Tier 3 (atributos) |

---

## 10. Referência
- Schema atual: migrations `000002/000008/000009/000010/000012/000020`.
- Estoque por source: `internal/variants` + `internal/sourcing` (`AvailableForVariant`).
- Padrão de default transparente: variante default (000010), delivery default (000019), source default (000020).
- Busca facetada (a habilitar com §3.7): `internal/search`.
