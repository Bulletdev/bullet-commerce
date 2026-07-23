# PaymentProvider - contrato consolidado

Documento canônico da abstração de pagamento do bullet-commerce. Fonte de verdade do
contrato: `internal/payment/provider.go`. Este arquivo explica o **porquê** do
desenho e como cada PSP do ecossistema se encaixa - para que qualquer fork
(freela ou projeto próprio) troque de provedor **por config**, sem tocar no core.

## Princípios (destilados do pay-rails, sem os Rails-ismos)

1. **Provider é adaptador stateless de I/O.** Fala com o PSP e **não conhece o
   banco**. Persistência e transições de estado ficam na camada de Order. (Corrige
   o maior acoplamento do `pay`, que enfia lógica de gateway no model ActiveRecord.)
2. **Core-fields tipados + `Raw json.RawMessage`** para o específico de cada PSP -
   equivalente tipado ao `jsonb data` do `pay`, sem schema-less.
3. **Dinheiro é `int64` centavos + `Currency` sempre junto.** Nunca float.
4. **`verify` (por-PSP, no ingress) separado de `process` (genérico, idempotente).**
   A verificação de assinatura é específica do PSP; o processamento é genérico e
   idempotente por chave natural (`ReferenceID` = order id).
5. **Seleção por `Registry` (`map[Name]Provider`)** - substitui o `constantize` de
   string do Rails, populado no startup a partir de `PAYMENT_PROVIDER`.
6. **Capabilities segregadas** (`Refunder`, `CardCharger`) - um provider PIX-only
   satisfaz só `Provider`; refund/cartão entram por type-assertion, sem tocar o core.

## Contrato

```go
type Provider interface {
    Name() Name
    CreatePixCharge(ctx, PixChargeRequest) (*PixCharge, error)
    GetCharge(ctx, providerID string) (*PixCharge, error)   // reconciliação / fallback de webhook
    VerifyWebhook(ctx, RawWebhook) (*WebhookEvent, error)   // ErrInvalidSignature -> 400; EventUnknown -> 200 no-op
}

type Refunder    interface { Refund(ctx, providerID string, amount Money) (*Refund, error) }   // opcional
type CardCharger interface { CreateCardCharge(ctx, CardChargeRequest) (*CardCharge, error) }    // opcional (Phase 3)
```

Idempotência em duas camadas (padrão do `pay` + do PRD): `ReferenceID` (chave
natural) para upsert no core **e** verificação HMAC/mTLS por-PSP no ingress.

## Registry de providers - escolhido por `PAYMENT_PROVIDER`

| `Name` | PSP | PIX | Boleto | Cartão | Confirma (webhook) | Estado |
|---|---|---|---|---|---|---|
| `propay` | ProPay/OpenPix via HTTP | ✅ | ❌ | ❌ | ✅ HMAC `X-Propay-Signature` | **implementado** (`internal/payment/propay`) |
| `efi` | Efí Bank (reimplementar em Go) | ✅ | ✅ | ✅ | ✅ mTLS (cert cliente) | Phase 2 |
| `itau_pix` | gem Rails PIX Automático (como serviço) | ✅ recorrente | ❌ | ❌ | ✅ mTLS | Phase 2 (recorrência/assinatura) |
| `pix_static` | go-pixgen embutido (só gera QR) | ✅ QR | ❌ | ❌ | ❌ manual/polling | Phase 2 (loja sem PSP) |

### Mapeamento por provider

- **`propay`** - `CreatePixCharge` assina JWT de serviço HS256 (`aud:["propay"]`,
  `exp:+5min`, `GO_TO_PROPAY_SECRET`) e chama `POST {PROPAY_URL}/v1/service/charges`.
  `VerifyWebhook` valida HMAC-SHA256 do body cru com `PROPAY_TO_GO_SECRET` contra
  `X-Propay-Signature: sha256=<hex>`. ⚠️ **Formato de request/response é assumido**
  (não há spec do ProPay no repo) - validar contra o projeto ProPay real antes do
  go-live e ajustar tags JSON. **TODO(phase2):** portar circuit breaker do
  prostaff-riot-gateway (5 falhas/60s → abre 30s).
- **`efi`** - mesma interface; `CreatePixCharge` = `POST /v2/cob` (OAuth2
  client_credentials + cert `.pem`); `VerifyWebhook` termina **mTLS na borda**
  (nginx/`tls.Config.ClientAuth`) + reconsulta idempotente por `txid` via
  `GetCharge` (`GET /v2/cob/:txid`). Boleto/cartão entram por `CardCharger` e uma
  futura capability de boleto. Preferir quando o fork precisar de boleto/cartão.
- **`itau_pix`** - roda como serviço Ruby (a gem `pix_automatico_itau`); o provider
  Go é um cliente HTTP para esse serviço. Sweet spot: assinatura/mensalidade.
- **`pix_static`** - usa `github.com/thiagozs/go-pixgen` para gerar copia-e-cola/QR
  em `CreatePixCharge`. `VerifyWebhook` não existe (sem confirmação): a transição
  para `paid` vem de ação manual do admin ou de polling do banco. Documentar o
  limite no fork que o adotar.

## Fluxo (idêntico para qualquer provider)

```
POST /api/orders            -> cria order (status=pending, payment_status=unpaid); Reserve do estoco da variante
POST /api/orders/{id}/pay   -> registry.Get(PAYMENT_PROVIDER).CreatePixCharge(...); payment_status=pending_payment; devolve QR/copia-e-cola/txid   [Phase 2]
webhook do PSP              -> POST /api/webhooks/payment -> provider.VerifyWebhook -> ConfirmOrderPayment (payment_status=paid, status=processing, Claim do estoque)   [Phase 2]
```

`ConfirmOrderPayment` (já existe no repositório de Order) é o ponto único de Claim;
o webhook da Phase 2 apenas o chama após `VerifyWebhook`.

## Configuração (12-Factor - tudo por env, segredo nunca no código)

| Env | Uso |
|---|---|
| `PAYMENT_PROVIDER` | nome no registry (`propay` default) |
| `PROPAY_URL` | base URL do serviço ProPay |
| `GO_TO_PROPAY_SECRET` | assina o JWT de serviço go→ProPay |
| `PROPAY_TO_GO_SECRET` | valida o HMAC do webhook ProPay→go |
| `PROPAY_TIMEOUT` | timeout do http.Client (default 5s) |

`efi`/`itau_pix`/`pix_static` adicionam suas próprias envs quando implementados
(ex.: `EFI_CLIENT_ID`, `EFI_CERT_PATH`, `EFI_SANDBOX`).

## Pendências antes do go-live de pagamento (Phase 2)

- [ ] Validar o formato real do ProPay (request/response/webhook) e ajustar `propay`.
- [ ] Portar circuit breaker para a chamada `propay`.
- [ ] `POST /api/orders/{id}/pay` e `POST /api/webhooks/payment` (fora do subrouter protegido).
- [ ] Implementar `efi` em Go (OAuth2 + mTLS) quando um fork precisar de boleto/cartão.
