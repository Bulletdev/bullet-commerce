
# Bullet Cloud API: E-commerce Backend
[![CodeQL Advanced](https://github.com/Bulletdev/bullet-cloud-api/actions/workflows/codeql.yml/badge.svg)](https://github.com/Bulletdev/bullet-cloud-api/actions/workflows/codeql.yml)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=Bulletdev_Arremate-certo&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=Bulletdev_Arremate-certo)
![Go Version](https://img.shields.io/github/go-mod/go-version/Bulletdev/go-cart-api?color=00ADD8&labelColor=000000)
![License](https://img.shields.io/badge/license-GPL--3.0-blue)

Uma API RESTful de alta performance desenvolvida em Go, projetada sob princípios de Clean Architecture. Focada em segurança (IAM), escalabilidade e gestão de ecossistemas de e-commerce modernos.

---

##  Stack 

| Camada | Tecnologia |
| :--- | :--- |
| **Linguagem** | Go (1.22+) |
| **Roteamento** | Gorilla Mux |
| **Persistência** | PostgreSQL (via pgx/v5) |
| **Segurança** | JWT (v5) & Bcrypt |
| **Migrações** | Golang-migrate |
| **Observabilidade** | Prometheus & Grafana (Planned) |

---

##  Arquitetura e Design
A aplicação implementa o padrão **Hexagonal / Clean Architecture**, isolando a lógica de negócio (Domain/Repositories) da infraestrutura e dos transportes (Handlers/Middlewares).



### Destaques do Projeto
- **DevSecOps Ready:** CodeQL e análise estática via SonarCloud integrados ao CI/CD.
- **Data Integrity:** Controle rigoroso de transações e migrações versionadas.
- **RESTful Design:** Endpoints padronizados com prefixo `/api` e status codes semânticos.
- **High Testability:** Handlers desacoplados via interfaces para facilitação de mocks e testes unitários.

---

## Visão Geral da API

<details>
<summary><b> Autenticação e Gestão de Identidade (IAM)</b></summary>

| Método | Endpoint | Descrição |
| :--- | :--- | :--- |
| POST | `/api/auth/register` | Registro de novo usuário |
| POST | `/api/auth/login` | Autenticação e emissão de JWT |
| GET | `/api/users/me` | Recupera perfil do portador do token |
</details>

<details>
<summary><b> Carrinho de Compras e Pedidos</b></summary>

| Método | Endpoint | Descrição |
| :--- | :--- | :--- |
| GET | `/api/cart` | Visualiza o estado atual do carrinho |
| POST | `/api/cart/items` | Adiciona ou incrementa itens |
| DELETE | `/api/cart/items/{id}` | Remove item específico |
| POST | `/api/orders` | Checkout e conversão de carrinho em pedido |
| GET | `/api/orders` | Lista histórico de pedidos |
</details>

<details>
<summary><b> Catálogo de Produtos</b></summary>

| Método | Endpoint | Descrição |
| :--- | :--- | :--- |
| GET | `/api/products` | Listagem geral com filtros (Planned) |
| GET | `/api/products/{id}` | Detalhes técnicos do produto |
| POST | `/api/products` | Criação de novos SKUs (Admin Only) |
| GET | `/api/categories` | Listagem de categorias ativas |
</details>

---

## Configuração e Execução

### Pré-requisitos
- Go 1.22+
- PostgreSQL
- [Golang-migrate CLI](https://github.com/golang-migrate/migrate)

### Setup Rápido
1. **Clone & Install:**
   ```bash
   git clone [https://github.com/bulletdev/go-cart-api.git](https://github.com/bulletdev/go-cart-api.git)
   cd go-cart-api && go mod tidy

```

2. **Environment:**
Crie um arquivo `.env` baseado nas variáveis abaixo:
```env
DATABASE_URL="postgres://user:pass@host:5432/db?sslmode=require"
JWT_SECRET="sua_chave_secreta_aqui"
API_PORT=4444

```


3. **Database Migrations:**
```bash
migrate -database ${DATABASE_URL} -path internal/database/migrations up

```


4. **Run:**
```bash
go run cmd/main.go

```



---

##  Qualidade de Código

Execução de suíte de testes unitários:

```bash
go test -v ./internal/handlers/...

```

---

## 📄 Licença

Distribuído sob a licença **GNU General Public License v3.0**. Veja `LICENSE` para mais informações.

##  Contato

Michael Bullet 

[contato@michaelbullet.com](mailto:contato@michaelbullet.com)
[michaelbullet.com](https://www.michaelbullet.com)

```
