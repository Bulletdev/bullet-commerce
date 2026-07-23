# Coverage Report - bullet-commerce

**Total atual: 71.4%** | Meta: ≥ 80% (produção) / 100% (completo)

---

## Estado por package

| Package | Coverage | Status |
|---|---|---|
| `middleware` | 100% | ✅ Completo |
| `models` | 100% | ✅ Completo |
| `webutils` | 100% | ✅ Completo |
| `products/repository` | 87.4% | 🔶 Próximo |
| `users/repository` | 88.6% | 🔶 Próximo |
| `auth` | 91.7% | 🔶 Próximo |
| `config` | 88.6% | 🔶 Próximo |
| `categories` | 85.7% | 🔶 Próximo |
| `addresses` | 83.5% | 🔶 Próximo |
| `handlers` | 71.5% | 🔴 Pendente |
| `cart/repository` | 75.4% | 🔴 Pendente |
| `orders/repository` | 50.6% | 🔴 Pendente |
| `database` | 0% | ⛔ Requer infra |
| `cmd` | 6.5% | ⛔ Requer infra |

---

## Gaps por categoria

### 1. Handlers - 0% (nunca cobertos)

| Função | Arquivo | Como cobrir |
|---|---|---|
| `GetFeaturedProducts` | `product_handler.go:90` | Adicionar test com `MockProductRepository.FindFeatured` |
| `GetProductsByCategory` | `product_handler.go:99` | Adicionar test com `FindByCategoryID` + UUID de categoria |
| `SearchProducts` | `product_handler.go:116` | Adicionar test com query param `q=` |
| `UpdateStock` | `product_handler.go:211` | Adicionar test admin com body `{"stock": N}` |
| `UpdateMe` | `user_handler.go:84` | Adicionar test com body `{"name":"...","email":"..."}` |
| `LookupCep` | `shipping_handler.go:29` | Usar `httptest.NewServer` para mockar ViaCEP |

### 2. Handlers - parcialmente cobertos (< 80%)

| Função | Coverage | Paths faltando |
|---|---|---|
| `CreateOrder` | 33.3% | Address not found, cart DB error, insufficient stock |
| `CancelOrder` | 48.0% | FindOrderByID DB error, order not found after found |
| `parsePagination` | 55.6% | Valores fora dos limites (limit > 100, offset negativo) |
| `getAuthenticatedUserID` | 66.7% | Context sem userID |
| `checkUserAuthorization` | 66.7% | UUID inválido no path |
| `GetMe` | 75.0% | DB error path |
| `GetCategory` | 75.0% | DB error path |
| `DeleteCategory` | 75.0% | DB error path |

### 3. Repositórios - paths de erro faltando

| Função | Coverage | Falta |
|---|---|---|
| `orders/CreateOrderFromCart` | 0% | Transação com `pgx.Batch` - `pgxmock` não suporta `SendBatch`; requer testcontainers ou refatoração |
| `orders/FindOrderByID` | 70.6% | Error na query de items |
| `addresses/SetDefault` | 80.0% | Erro no Exec de unset dentro da transação |
| `cart/GetCartItems` | 91.7% | Erro no `rows.Scan` |
| `config/Load` | 63.6% | Paths com `os.Exit(1)` - intestável sem mock de os.Exit |

### 4. Mock files - função pattern não coberta

Os mocks usam um pattern de `returnFn` (`func(ctx) *T`) para o primeiro return. Esse path não é exercitado nos testes de mock. Coverage ~81% em vez de 100%.

**Fix:** Adicionar em cada `TestMock*_AllMethods` um caso que retorna uma função em vez de um valor direto:
```go
m.On("FindByID", mock.Anything, id).Return(
    func(ctx context.Context, id uuid.UUID) *models.Product { return p },
    func(ctx context.Context, id uuid.UUID) error { return nil },
)
m.FindByID(ctx, id)
```

### 5. Dead code nos mocks do cart

`cart/repository_mock.go` tem dois métodos (`UpdateItem`, `DeleteItem`) que **não existem** na interface `CartRepository`. São 0% porque nunca são chamados e não fazem parte do contrato.

**Fix:** Remover `UpdateItem` e `DeleteItem` de `cart/repository_mock.go`.

---

## Itens genuinamente não testáveis sem infra

| Função | Motivo | Solução alternativa |
|---|---|---|
| `database/NewConnection` | Requer PostgreSQL real | Testcontainers ou integration test tag |
| `cmd/main` | Requer servidor completo + banco | Excluir do coverage via build tag |
| `cmd/setupRoutes` | Depende de todos os handlers e auth | Teste de smoke com mocks de todos os repos |
| `cmd/runOrderCleanup` | Goroutine com ticker infinito | Refatorar para aceitar `clock` injetável |
| `orders/CreateOrderFromCart` | `pgx.Batch` não suportado pelo pgxmock | Testcontainers + banco real |

---

## Roadmap para 80%+ (próxima sprint)

**1. Remover dead code** (30min)
- Remover `UpdateItem` e `DeleteItem` de `cart/repository_mock.go` - eliminam 2 funções a 0%

**2. Handler tests faltantes** (3h)
- Adicionar `product_handler_test.go` para `GetFeaturedProducts`, `GetProductsByCategory`, `SearchProducts`, `UpdateStock`
- Adicionar `user_handler_test.go` para `UpdateMe`
- Cobrir error paths em `CreateOrder`, `CancelOrder`, `parsePagination`

**3. LookupCep com httptest.Server** (1h)
```go
srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    json.NewEncoder(w).Encode(map[string]string{
        "logradouro": "Rua Teste", "bairro": "Centro",
        "localidade": "SP", "uf": "SP",
    })
}))
// Injetar srv.URL via ENV VIACEP_URL em vez de hardcode
```
Requer que `LookupCep` leia a URL base de ENV - hoje está hardcoded.

**4. Mock returnFn paths** (1h)
- Adicionar casos com função como return value em cada `TestMock*_AllMethods`

**5. Remover `os.Exit` do `config.Load`** (30min)
- Extrair para `loadOrFatal(exitFn func(int))` - permite injetar um mock de exit

---

## Roadmap para 100% (requer infraestrutura)

**Testcontainers** - adicionar `github.com/testcontainers/testcontainers-go`

Cobre: `database/NewConnection`, `orders/CreateOrderFromCart`, todos os repositórios com paths de erro de banco real.

Custo: ~1 semana de setup + 30-60s por test run.

**Build tags para exclusão** - `cmd/main` e `cmd/runOrderCleanup` são convencionalmente excluídos de coverage em Go. Adicionar um Makefile target:

```makefile
coverage:
    go test -coverprofile=coverage.out \
        -coverpkg=./internal/... \
        ./internal/...
    go tool cover -html=coverage.out
```

Excluindo `cmd` e `database` do denominador, a cobertura real dos packages testáveis fica em **~85%**.
