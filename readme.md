
# Bullet Cloud API: E-commerce Backend
[![CodeQL Advanced](https://github.com/Bulletdev/bullet-cloud-api/actions/workflows/codeql.yml/badge.svg)](https://github.com/Bulletdev/bullet-cloud-api/actions/workflows/codeql.yml)
[![Go](https://github.com/Bulletdev/bullet-cloud-api/actions/workflows/go.yml/badge.svg)](https://github.com/Bulletdev/bullet-cloud-api/actions/workflows/go.yml)
[![Quality Gate Status](https://sonarcloud.io/api/project_badges/measure?project=Bulletdev_Arremate-certo&metric=alert_status)](https://sonarcloud.io/summary/new_code?id=Bulletdev_Arremate-certo)
![Go Version](https://img.shields.io/github/go-mod/go-version/Bulletdev/go-cart-api?color=00ADD8&labelColor=000000)
![License](https://img.shields.io/badge/license-GPL--3.0-blue)

API RESTful de alta performance desenvolvida em Go, focada em segurança (IAM), escalabilidade e padrões de Clean Architecture. Ideal para ecossistemas de e-commerce que exigem auditoria e integridade de dados.

---

##  Arquitetura e Design System
O projeto implementa o padrão **Hexagonal / Clean Architecture**, garantindo total desacoplamento entre a lógica de negócio e os provedores de infraestrutura.



### Core Principles
- **DevSecOps Integrado:** CodeQL e análise estática SonarCloud no pipeline de CI.
- **Data Integrity:** Controle de transações ACID no PostgreSQL e migrações versionadas.
- **IAM:** Autenticação via JWT (v5) e hashing robusto com Bcrypt.
- **Scalability:** Design stateless e interfaces para repositórios facilitando substituição de DB ou Mocking em testes.

---

##  Guia de Uso (Exemplos Rápidos)

<details>
<summary><b>Autenticação e Registro</b></summary>

**Registrar novo usuário:**
```bash
curl -X POST http://localhost:4444/api/auth/register \
-H "Content-Type: application/json" \
-d '{"name":"Michael Bullet","email":"contato@michaelbullet.com","password":"senha_segura"}'

```

**Login e Obtenção de Token:**

```bash
TOKEN=$(curl -s -X POST http://localhost:4444/api/auth/login \
-H "Content-Type: application/json" \
-d '{"email":"contato@michaelbullet.com","password":"senha_segura"}' | jq -r .token)

```

</details>

<details>
<summary><b>Gerenciamento de Carrinho</b></summary>

**Adicionar Item:**

```bash
curl -X POST http://localhost:4444/api/cart/items \
-H "Authorization: Bearer $TOKEN" \
-H "Content-Type: application/json" \
-d '{"product_id":"UUID_DO_PRODUTO","quantity":2}'

```

**Checkout (Gerar Pedido):**

```bash
curl -X POST http://localhost:4444/api/orders -H "Authorization: Bearer $TOKEN"

```

</details>

---

##  Documentação da API

| Módulo | Método | Endpoint | Protegido |
| --- | --- | --- | --- |
| **Auth** | POST | `/api/auth/login` | No |
| **Users** | GET | `/api/users/me` | Yes |
| **Products** | GET | `/api/products` | No |
| **Cart** | POST | `/api/cart/items` | Yes |
| **Orders** | GET | `/api/orders` | Yes |

> [Consulte a Documentação Arquitetural Detalhada](https://www.google.com/search?q=./docs/README.md) para diagramas de ERD e fluxos de autenticação.

---

##  Configuração do Ambiente

### Pré-requisitos

* Go 1.22+
* PostgreSQL (ou Supabase)
* [Golang-migrate CLI](https://github.com/golang-migrate/migrate)

### Setup

1. **Instalação:**
```bash
git clone [https://github.com/bulletdev/go-cart-api.git](https://github.com/bulletdev/go-cart-api.git)
cd go-cart-api && go mod tidy

```


2. **Ambiente (.env):**
```env
DATABASE_URL="postgres://usuario:senha@host:5432/database"
JWT_SECRET="seu_segredo_jwt"

```


3. **Database Migrations:**
```bash
migrate -database ${DATABASE_URL} -path internal/database/migrations up

```


4. **Execução:**
```bash
go run cmd/main.go

```



---

##  Qualidade e Testes

Suíte de testes unitários focada em Handlers e Lógica de Negócio:

```bash
go test -v ./internal/handlers/...

```

---

## 📄 Licença e Contato

* **Licença:** GNU General Public License v3.0
* **Autor:** Michael Bullet - [contato@michaelbullet.com](mailto:contato@michaelbullet.com)
* **Web:** [michaelbullet.com](https://www.michaelbullet.com)

```
