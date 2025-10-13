<p align="center">
  
[![CodeQL Advanced](https://github.com/Bulletdev/bullet-cloud-api/actions/workflows/codeql.yml/badge.svg)](https://github.com/Bulletdev/bullet-cloud-api/actions/workflows/codeql.yml)
[![Go](https://github.com/Bulletdev/bullet-cloud-api/actions/workflows/go.yml/badge.svg)](https://github.com/Bulletdev/bullet-cloud-api/actions/workflows/go.yml)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=Bulletdev_Arremate-certo&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=Bulletdev_Arremate-certo)
[![Bugs](https://sonarcloud.io/api/project_badges/measure?project=Bulletdev_Arremate-certo&metric=bugs)](https://sonarcloud.io/summary/new_code?id=Bulletdev_Arremate-certo)
<img src="https://img.shields.io/badge/status-Em%20Desenvolvimento-Orange"> 
</p>     
   
# API RESTful em Go para E-commerce
 
<p align="center"> 
  <img alt="GitHub top language" src="https://img.shields.io/github/languages/top/Bulletdev/bullet-cloud-api?color=04D361&labelColor=000000">  
  
  <a href="https://www.linkedin.com/in/Michael-Bullet/">
    <img alt="Made by" src="https://img.shields.io/static/v1?label=made%20by&message=Michael%20Bullet&color=04D361&labelColor=000000">
  </a>  
  
  <img alt="Repository size" src="https://img.shields.io/github/repo-size/bulletdev/bullet-cloud-api?color=04D361&labelColor=000000">
  
  <a href="https://github.com/Bulletdev/go-cart-api/commits/master">
    <img alt="GitHub last commit" src="https://img.shields.io/github/last-commit/bulletdev/bullet-cloud-api?color=04D361&labelColor=000000">
  </a>
</p>

# ✨ Recursos Atuais e Planejados
 
- Autenticação e Gerenciamento de Usuários (Registro, Login, Dados do Usuário, Endereços)
- Gerenciamento de Produtos e Categorias
- Carrinho de Compras
- Gerenciamento de Pedidos (Criação e Listagem)
- Armazenamento de dados com PostgreSQL (via Supabase)
- Autenticação segura com JWT e Hashing de Senha (bcrypt)
- Endpoints RESTful com prefixo `/api`
Health check
- Testes Unitários para Handlers (Auth, User/Address, Product, Category, Cart)

## Planejado:
>>> Testes para OrderHandler, Testes de Integração, Lógica de Frete, Paginação, Filtros, Validação Avançada, Permissões (Admin), Documentação Swagger completa.


##  Exemplo de uso

<details>

(Veja a seção Endpoints Atuais para mais detalhes)

**Registrar um novo usuário:**

*Windows (PowerShell):*
```powershell
Invoke-RestMethod -Uri http://localhost:4444/api/auth/register -Method POST -ContentType "application/json" -Body '{"name":"Nome Sobrenome","email":"email@exemplo.com","password":"senha123"}'
```
*Linux/macOS (curl):*
```bash
curl -X POST http://localhost:4444/api/auth/register \\
-H "Content-Type: application/json" \\
-d '{"name":"Nome Sobrenome","email":"email@exemplo.com","password":"senha123"}'
```

**Fazer login:**

*Windows (PowerShell):*
```powershell
$response = Invoke-RestMethod -Uri http://localhost:4444/api/auth/login -Method POST -ContentType "application/json" -Body '{"email":"email@exemplo.com","password":"senha123"}'
$token = $response.token
Write-Host "Token JWT: $token"
# Você precisará extrair o USER_ID do token ou de /api/users/me para os próximos exemplos
```
*Linux/macOS (curl) (requer `jq` para extrair o token):*
```bash
TOKEN=$(curl -s -X POST http://localhost:4444/api/auth/login \\
-H "Content-Type: application/json" \\
-d '{"email":"email@exemplo.com","password":"senha123"}' | jq -r .token)
echo "Token JWT: $TOKEN"
# Você precisará extrair o USER_ID do token ou de /api/users/me para os próximos exemplos
# Ex: USER_ID=$(curl -s -H "Authorization: Bearer $TOKEN" http://localhost:4444/api/users/me | jq -r .id)
```

**Adicionar um endereço (requer token):**

*Linux/macOS (curl) (assumindo que USER_ID e TOKEN estão definidos):*
```bash
curl -X POST http://localhost:4444/api/users/$USER_ID/addresses \\
-H "Authorization: Bearer $TOKEN" \\
-H "Content-Type: application/json" \\
-d '{"street":"Rua Exemplo, 123","city":"Cidade","state":"SP","postal_code":"12345-678","country":"Brasil","is_default":true}'
```

**Adicionar item ao carrinho (requer token):**

*Linux/macOS (curl) (assumindo que PRODUCT_ID e TOKEN estão definidos):*
```bash
curl -X POST http://localhost:4444/api/cart/items \\
-H "Authorization: Bearer $TOKEN" \\
-H "Content-Type: application/json" \\
-d '{"product_id":"'$PRODUCT_ID'","quantity":2}'
```

**Ver carrinho (requer token):**

*Linux/macOS (curl) (assumindo que TOKEN está definido):*
```bash
curl -H "Authorization: Bearer $TOKEN" http://localhost:4444/api/cart
```

**Criar pedido do carrinho (requer token):**

*Linux/macOS (curl) (assumindo que TOKEN está definido):*
```bash
curl -X POST http://localhost:4444/api/orders \\
-H "Authorization: Bearer $TOKEN"
```

</details>

## Documentação da API (Planejada)

(A documentação Swagger existente (`swagger.yaml`) está desatualizada. Será atualizada conforme a API evolui.)

[Link para Documentação Swagger Antiga (Desatualizada)](https://app.swaggerhub.com/apis-docs/bulletcloud/Estoque/1.1) 


## 🛠 Tecnologias

<div>
Golang
</div> 
<div>  
Gorilla Mux
</div> 
<div>
PostgreSQL (via Supabase)
</div>
<div>
pgx/v5 (Driver PostgreSQL)
</div>
<div>
golang-jwt/jwt/v5 (Autenticação JWT)
</div>
<div>
golang.org/x/crypto/bcrypt (Hashing de Senha)
</div>
<div>
golang-migrate/migrate (Migrações de Banco de Dados)
</div>
<div>
stretchr/testify (Testes Unitários)
</div>


## 📦 Instalação

**Pré-requisitos**

*   Go (versão especificada no `go.mod`, ex: 1.22+)
*   Git
*   Docker (Opcional, para rodar banco localmente se não usar Supabase)
*   [golang-migrate/migrate CLI](https://github.com/golang-migrate/migrate/tree/master/cmd/migrate) (Instalada e no PATH)

**Passos**

1.  **Clonar o repositório:**
    ```bash
    git clone https://github.com/bulletdev/go-cart-api.git
    cd go-cart-api
    ```
2.  **Configurar Variáveis de Ambiente:**
    *   Crie um arquivo chamado `.env` na raiz do projeto.
    *   Adicione as seguintes variáveis, substituindo pelos seus valores:
        ```env
        # URL de conexão do seu banco PostgreSQL (ex: Supabase)
        DATABASE_URL="postgres://usuario:senha@host:porta/database?sslmode=require"
        
        # Segredo para assinar os tokens JWT (obtenha do Supabase ou gere um seguro)
        JWT_SECRET="seu_segredo_super_seguro_aqui"
        
        # Tempo de expiração do token JWT (opcional, padrão 1h)
        # JWT_EXPIRY_HOURS=1 

        # Porta da API (opcional, padrão 4444)
        # API_PORT=4444 
        ```
    *   **Importante:** Adicione `.env` ao seu `.gitignore` (já deve estar feito).
3.  **Instalar Dependências:**
    ```bash
    go mod tidy
    go mod vendor # Opcional, se estiver usando vendoring
    ```
4.  **Aplicar Migrações do Banco:**
    *   Certifique-se que a CLI `migrate` está instalada.
    *   Execute (substitua a URL se não estiver usando `.env` diretamente):
        ```bash
        # No Linux/macOS (se .env carregado no shell)
        # migrate -database ${DATABASE_URL} -path internal/database/migrations up
        
        # No Windows PowerShell (se .env carregado no shell)
        # migrate -database $env:DATABASE_URL -path internal/database/migrations up
        
        # Passando a URL diretamente (mais seguro se .env não carregado)
        migrate -database 'SUA_DATABASE_URL_COMPLETA_AQUI' -path internal/database/migrations up 
        ```
5.  **Rodar Aplicação:**
    ```bash
    go run cmd/main.go
    ```

## 🔍 Endpoints Atuais

<details>
 
**Saúde**
*   `GET /api/health`: Verifica status da aplicação.

**Autenticação**
*   `POST /api/auth/register`: Registra um novo usuário.
    *   **Corpo:** `{"name": "...", "email": "...", "password": "..."}`
    *   **Sucesso (201):** Objeto `User` (sem senha).
    *   **Erros:** `400` (inválido), `409` (email existe), `500`.
*   `POST /api/auth/login`: Autentica um usuário.
    *   **Corpo:** `{"email": "...", "password": "..."}`
    *   **Sucesso (200):** `{"token": "jwt_token"}`.
    *   **Erros:** `400`, `401` (inválido), `500`.

**Usuários**
*   `GET /api/users/me` (Protegido): Retorna informações do usuário autenticado (obtido do token).
    *   **Sucesso (200):** Objeto `User` (sem senha).
    *   **Erros:** `401` (sem token/inválido), `500`.

**Endereços** (Rotas aninhadas sob `/api/users/{userId}`)
*   `GET /api/users/{userId}/addresses` (Protegido): Lista endereços do usuário `{userId}`. *Requer que `{userId}` seja o mesmo do token.*
    *   **Sucesso (200):** Array de objetos `Address`.
    *   **Erros:** `401`, `403` (outro usuário), `404` (usuário inválido na URL), `500`.
*   `POST /api/users/{userId}/addresses` (Protegido): Adiciona um novo endereço para o usuário `{userId}`. *Requer que `{userId}` seja o mesmo do token.*
    *   **Corpo:** `{"street": "...", "city": "...", "state": "...", "postal_code": "...", "country": "...", "is_default": boolean (opcional)}`
    *   **Sucesso (201):** Objeto `Address` criado.
    *   **Erros:** `400` (inválido), `401`, `403`, `404`, `500`.
*   `PUT /api/users/{userId}/addresses/{addressId}` (Protegido): Atualiza o endereço `{addressId}` do usuário `{userId}`. *Requer que `{userId}` seja o mesmo do token.*
    *   **Corpo:** `{"street": "...", "city": "...", "state": "...", "postal_code": "...", "country": "...", "is_default": boolean (opcional)}`
    *   **Sucesso (200):** Objeto `Address` atualizado.
    *   **Erros:** `400`, `401`, `403`, `404` (usuário/endereço inválido ou não encontrado), `500`.
*   `DELETE /api/users/{userId}/addresses/{addressId}` (Protegido): Remove o endereço `{addressId}` do usuário `{userId}`. *Requer que `{userId}` seja o mesmo do token.*
    *   **Sucesso (204):** Sem conteúdo.
    *   **Erros:** `401`, `403`, `404`, `500`.
*   `POST /api/users/{userId}/addresses/{addressId}/default` (Protegido): Define o endereço `{addressId}` como padrão para o usuário `{userId}`. *Requer que `{userId}` seja o mesmo do token.*
    *   **Sucesso (200):** Sem conteúdo explícito (OK).
    *   **Erros:** `401`, `403`, `404`, `500`.

**Produtos**
*   `GET /api/products`: Lista todos os produtos.
    *   **Sucesso (200):** Array de objetos `Product`.
*   `GET /api/products/{id}`: Busca um produto específico pelo ID.
    *   **Sucesso (200):** Objeto `Product`.
    *   **Erros:** `400` (ID inválido), `404` (não encontrado), `500`.
*   `POST /api/products` (Protegido): Cria um novo produto.
    *   **Corpo:** `{"name": "...", "description": "..." (opcional), "price": 123.45, "category_id": "uuid" (opcional)}`
    *   **Sucesso (201):** Objeto `Product` criado.
    *   **Erros:** `400` (inválido), `401`, `500`.
*   `PUT /api/products/{id}` (Protegido): Atualiza um produto existente.
    *   **Corpo:** `{"name": "...", "description": "..." (opcional), "price": 123.45, "category_id": "uuid" (opcional)}`
    *   **Sucesso (200):** Objeto `Product` atualizado.
    *   **Erros:** `400`, `401`, `404`, `500`.
*   `DELETE /api/products/{id}` (Protegido): Deleta um produto.
    *   **Sucesso (204):** Sem conteúdo.
    *   **Erros:** `401`, `404`, `500`.

**Categorias**
*   `GET /api/categories`: Lista todas as categorias.
    *   **Sucesso (200):** Array de objetos `Category`.
*   `GET /api/categories/{id}`: Busca uma categoria específica pelo ID.
    *   **Sucesso (200):** Objeto `Category`.
    *   **Erros:** `400`, `404`, `500`.
*   `POST /api/categories` (Protegido): Cria uma nova categoria.
    *   **Corpo:** `{"name": "..."}`
    *   **Sucesso (201):** Objeto `Category` criado.
    *   **Erros:** `400`, `401`, `409` (nome existe), `500`.
*   `PUT /api/categories/{id}` (Protegido): Atualiza uma categoria existente.
    *   **Corpo:** `{"name": "..."}`
    *   **Sucesso (200):** Objeto `Category` atualizado.
    *   **Erros:** `400`, `401`, `404`, `409`, `500`.
*   `DELETE /api/categories/{id}` (Protegido): Deleta uma categoria.
    *   **Sucesso (204):** Sem conteúdo.
    *   **Erros:** `401`, `404`, `500`.

**Carrinho de Compras** (Operações no carrinho do usuário autenticado)
*   `GET /api/cart` (Protegido): Recupera o carrinho atual do usuário (cria um se não existir).
    *   **Sucesso (200):** Objeto `{"cart": {...}, "items": [{...}]}` (Items pode ser vazio).
    *   **Erros:** `401`, `500`.
*   `POST /api/cart/items` (Protegido): Adiciona um item ao carrinho (ou incrementa quantidade se já existir).
    *   **Corpo:** `{"product_id": "uuid", "quantity": int}`
    *   **Sucesso (200):** Objeto `{"cart": {...}, "items": [{...}]}` atualizado.
    *   **Erros:** `400` (inválido/qtde<=0), `401`, `404` (produto não existe), `500`.
*   `PUT /api/cart/items/{productId}` (Protegido): Atualiza a quantidade de um item específico (`productId`) no carrinho. *Se quantidade for 0 ou menor, remove o item.*
    *   **Corpo:** `{"quantity": int}`
    *   **Sucesso (200):** Objeto `{"cart": {...}, "items": [{...}]}` atualizado.
    *   **Erros:** `400`, `401`, `404` (item/produto não encontrado), `500`.
*   `DELETE /api/cart/items/{productId}` (Protegido): Remove um item específico (`productId`) do carrinho.
    *   **Sucesso (200):** Objeto `{"cart": {...}, "items": [{...}]}` atualizado.
    *   **Erros:** `401`, `404` (item/produto não encontrado), `500`.
*   `DELETE /api/cart` (Protegido): Limpa *todos* os itens do carrinho do usuário.
    *   **Sucesso (200):** Objeto `{"cart": {...}, "items": []}` (Carrinho vazio).
    *   **Erros:** `401`, `500`.

**Pedidos**
*   `POST /api/orders` (Protegido): Cria um novo pedido a partir dos itens no carrinho atual do usuário. *Limpa o carrinho após criar o pedido.*
    *   **Sucesso (201):** Objeto `{"order": {...}, "items": [{...}]}` do pedido criado.
    *   **Erros:** `400` (carrinho vazio), `401`, `500`.
*   `GET /api/orders` (Protegido): Lista os pedidos do usuário autenticado.
    *   **Sucesso (200):** Array de objetos `Order`.
    *   **Erros:** `401`, `500`.
*   `GET /api/orders/{id}` (Protegido): Busca os detalhes de um pedido específico (`id`). *Só permite buscar próprios pedidos.*
    *   **Sucesso (200):** Objeto `{"order": {...}, "items": [{...}]}`.
    *   **Erros:** `401`, `403` (não é dono), `404` (pedido não encontrado/ID inválido), `500`.

</details>

*(Funcionalidades de Pedidos como cancelamento e atualização de status foram implementadas no repositório mas não expostas em rotas ainda).*


## 🧪 Testes

Para rodar os testes unitários dos handlers:
```bash
go test -v ./internal/handlers/...
```

## 🏗️ Arquitetura

Para visualizar a arquitetura completa do sistema, consulte nossa documentação técnica:

### 📋 [Documentação Arquitetural Completa](./docs/README.md)

Inclui diagramas detalhados de:
- **Arquitetura Geral do Sistema**: Visão macro com todas as camadas
- **Fluxo de Autenticação**: Processo de login/registro com JWT
- **Modelo de Dados (ERD)**: Estrutura do banco PostgreSQL
- **Fluxo de Requisições**: Como as requests são processadas
- **Padrões Arquiteturais**: Clean Architecture e Repository Pattern

### 🎯 Resumo Arquitetural

**Padrão Principal**: Clean Architecture / Hexagonal
- **Camadas bem separadas**: Handlers → Repositories → Database
- **Inversão de dependências**: Interfaces para testabilidade
- **Tecnologias**: Go + Gorilla Mux + PostgreSQL + JWT

```
Cliente → Router → Middleware → Handler → Repository → Database
```

### 📄 Licença

GNU-General-Public-License-v3.0

