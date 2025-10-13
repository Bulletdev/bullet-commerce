# Diagrama de Arquitetura - Go Cart API

Esta documentação apresenta os diagramas de arquitetura do sistema Go Cart API, uma API RESTful em Go para e-commerce.

## 1. Arquitetura Geral do Sistema

```mermaid
graph TB
    subgraph "Cliente"
        Client[Cliente HTTP/API]
        WebApp[Aplicação Web]
        Mobile[App Mobile]
    end

    subgraph "Load Balancer/Reverse Proxy"
        LB[Load Balancer<br/>Nginx/Render]
    end

    subgraph "Go Cart API Application"
        subgraph "HTTP Layer"
            Router[Gorilla Mux Router<br/>:4445+]
            MW[Auth Middleware<br/>JWT Validation]
        end

        subgraph "Handler Layer"
            AuthH[Auth Handler<br/>Login/Register]
            UserH[User Handler<br/>Profile/Addresses]
            ProductH[Product Handler<br/>CRUD Products]
            CategoryH[Category Handler<br/>CRUD Categories]
            CartH[Cart Handler<br/>Shopping Cart]
            OrderH[Order Handler<br/>Order Management]
            HealthH[Health Handler<br/>System Status]
        end

        subgraph "Business Logic Layer"
            UserRepo[User Repository]
            ProductRepo[Product Repository]
            CategoryRepo[Category Repository]
            AddressRepo[Address Repository]
            CartRepo[Cart Repository]
            OrderRepo[Order Repository]
        end

        subgraph "Security & Utils"
            JWT[JWT Service<br/>Token Generation]
            BCrypt[BCrypt Hasher<br/>Password Security]
            WebUtils[Web Utils<br/>JSON Helpers]
        end

        subgraph "Configuration"
            Config[Config Loader<br/>ENV Variables]
            DB[Database Connection<br/>pgxpool]
        end
    end

    subgraph "External Services"
        subgraph "Database"
            PostgreSQL[(PostgreSQL<br/>via Supabase)]
        end
        
        subgraph "Environment"
            ENV[Environment Variables<br/>DATABASE_URL<br/>JWT_SECRET<br/>PORT]
        end
    end

    %% Connections
    Client --> LB
    WebApp --> LB
    Mobile --> LB
    
    LB --> Router
    
    Router --> MW
    MW --> AuthH
    MW --> UserH
    MW --> ProductH
    MW --> CategoryH
    MW --> CartH
    MW --> OrderH
    Router --> HealthH
    
    AuthH --> UserRepo
    AuthH --> JWT
    AuthH --> BCrypt
    
    UserH --> UserRepo
    UserH --> AddressRepo
    
    ProductH --> ProductRepo
    CategoryH --> CategoryRepo
    CartH --> CartRepo
    CartH --> ProductRepo
    OrderH --> OrderRepo
    OrderH --> CartRepo
    OrderH --> AddressRepo
    
    UserRepo --> DB
    ProductRepo --> DB
    CategoryRepo --> DB
    AddressRepo --> DB
    CartRepo --> DB
    OrderRepo --> DB
    
    DB --> PostgreSQL
    Config --> ENV
    
    %% Styling
    classDef handler fill:#e1f5fe
    classDef repo fill:#f3e5f5
    classDef security fill:#fff3e0
    classDef database fill:#e8f5e8
    classDef external fill:#fce4ec
    
    class AuthH,UserH,ProductH,CategoryH,CartH,OrderH,HealthH handler
    class UserRepo,ProductRepo,CategoryRepo,AddressRepo,CartRepo,OrderRepo repo
    class JWT,BCrypt,MW security
    class PostgreSQL,DB database
    class ENV,LB external
```

## 2. Fluxo de Autenticação

```mermaid
sequenceDiagram
    participant C as Cliente
    participant R as Router
    participant AH as Auth Handler
    participant UR as User Repository
    participant BC as BCrypt
    participant JWT as JWT Service
    participant DB as PostgreSQL

    %% Registro
    Note over C,DB: Fluxo de Registro
    C->>+R: POST /api/auth/register
    R->>+AH: Register()
    AH->>+BC: HashPassword()
    BC-->>-AH: hashedPassword
    AH->>+UR: Create(name, email, hash)
    UR->>+DB: INSERT INTO users
    DB-->>-UR: user created
    UR-->>-AH: User object
    AH->>+JWT: GenerateToken(userID)
    JWT-->>-AH: token
    AH-->>-R: {user, token}
    R-->>-C: 201 Created

    %% Login
    Note over C,DB: Fluxo de Login
    C->>+R: POST /api/auth/login
    R->>+AH: Login()
    AH->>+UR: FindByEmail(email)
    UR->>+DB: SELECT FROM users
    DB-->>-UR: user data
    UR-->>-AH: User object
    AH->>+BC: ValidatePassword()
    BC-->>-AH: valid/invalid
    AH->>+JWT: GenerateToken(userID)
    JWT-->>-AH: token
    AH-->>-R: {user, token}
    R-->>-C: 200 OK

    %% Autenticação
    Note over C,DB: Requests Autenticados
    C->>+R: GET /api/users/me (Bearer token)
    R->>+MW: Authenticate()
    MW->>+JWT: ValidateToken()
    JWT-->>-MW: claims
    MW->>+UR: FindByID(userID)
    UR-->>-MW: user exists
    MW-->>-R: context with userID
    R->>UH: GetMe()
    UH-->>R: user data
    R-->>-C: 200 OK
```

## 3. Modelo de Dados (ERD)

```mermaid
erDiagram
    USERS {
        uuid id PK
        string name
        string email UK
        string password_hash
        timestamp created_at
        timestamp updated_at
    }
    
    ADDRESSES {
        uuid id PK
        uuid user_id FK
        string street
        string city
        string state
        string zip_code
        string country
        boolean is_default
        timestamp created_at
        timestamp updated_at
    }
    
    CATEGORIES {
        uuid id PK
        string name UK
        timestamp created_at
        timestamp updated_at
    }
    
    PRODUCTS {
        uuid id PK
        string name
        string description
        decimal price
        uuid category_id FK "nullable"
        timestamp created_at
        timestamp updated_at
    }
    
    CARTS {
        uuid id PK
        uuid user_id FK
        timestamp created_at
        timestamp updated_at
    }
    
    CART_ITEMS {
        uuid id PK
        uuid cart_id FK
        uuid product_id FK
        integer quantity
        timestamp created_at
        timestamp updated_at
    }
    
    ORDERS {
        uuid id PK
        uuid user_id FK
        uuid address_id FK
        decimal total_amount
        string status
        timestamp created_at
        timestamp updated_at
    }
    
    ORDER_ITEMS {
        uuid id PK
        uuid order_id FK
        uuid product_id FK
        integer quantity
        decimal price_at_time
        timestamp created_at
        timestamp updated_at
    }

    %% Relationships
    USERS ||--o{ ADDRESSES : "has"
    USERS ||--|| CARTS : "owns"
    USERS ||--o{ ORDERS : "places"
    
    CATEGORIES ||--o{ PRODUCTS : "contains"
    
    CARTS ||--o{ CART_ITEMS : "contains"
    PRODUCTS ||--o{ CART_ITEMS : "referenced_by"
    
    ORDERS ||--o{ ORDER_ITEMS : "contains"
    PRODUCTS ||--o{ ORDER_ITEMS : "referenced_by"
    ADDRESSES ||--o{ ORDERS : "delivery_to"
```

## 4. Fluxo de API Requests

```mermaid
graph TD
    A[Cliente HTTP Request] --> B{Rota Pública?}
    
    B -->|Sim| C[Handler Direto]
    B -->|Não| D[Auth Middleware]
    
    D --> E{Token Válido?}
    E -->|Não| F[401 Unauthorized]
    E -->|Sim| G[Adicionar UserID ao Context]
    
    G --> H[Handler Específico]
    C --> H
    
    H --> I[Validar Request]
    I --> J{Dados Válidos?}
    J -->|Não| K[400 Bad Request]
    J -->|Sim| L[Repository Layer]
    
    L --> M[Database Query]
    M --> N{Query Sucesso?}
    N -->|Não| O[500 Internal Error]
    N -->|Sim| P[Processar Resultado]
    
    P --> Q[JSON Response]
    Q --> R[Cliente]
    
    F --> R
    K --> R
    O --> R
    
    %% Styling
    classDef error fill:#ffcdd2
    classDef success fill:#c8e6c9
    classDef process fill:#e1f5fe
    
    class F,K,O error
    class Q,R success
    class D,G,H,I,L,P process
```

## 5. Estrutura de Pastas e Responsabilidades

```mermaid
graph TD
    subgraph "Projeto Go Cart API"
        subgraph "cmd/"
            MainGo[main.go<br/>• Entry point<br/>• Server setup<br/>• Route configuration]
        end
        
        subgraph "internal/"
            subgraph "handlers/"
                Handlers[• HTTP request handling<br/>• Input validation<br/>• Response formatting<br/>• Error handling]
            end
            
            subgraph "models/"
                Models[• Data structures<br/>• Business entities<br/>• JSON tags<br/>• Database mappings]
            end
            
            subgraph "repositories/"
                Repos[users/<br/>products/<br/>categories/<br/>addresses/<br/>cart/<br/>orders/<br/>• Database operations<br/>• Query implementation<br/>• Error handling]
            end
            
            subgraph "auth/"
                Auth[• JWT handling<br/>• Password hashing<br/>• Middleware<br/>• Authentication logic]
            end
            
            subgraph "config/"
                Config[• Environment variables<br/>• Configuration loading<br/>• Application settings]
            end
            
            subgraph "database/"
                Database[• Connection management<br/>• Pool configuration<br/>• Migration handling]
            end
            
            subgraph "webutils/"
                WebUtils[• JSON helpers<br/>• HTTP utilities<br/>• Common functions]
            end
        end
        
        subgraph "docs/"
            Docs[• API documentation<br/>• Architecture diagrams<br/>• Usage examples]
        end
    end
    
    MainGo --> Handlers
    MainGo --> Config
    MainGo --> Database
    
    Handlers --> Models
    Handlers --> Repos
    Handlers --> Auth
    Handlers --> WebUtils
    
    Repos --> Models
    Repos --> Database
    
    Auth --> Repos
    
    %% Styling
    classDef entry fill:#fff3e0
    classDef business fill:#e8f5e8
    classDef data fill:#e1f5fe
    classDef security fill:#fce4ec
    classDef config fill:#f3e5f5
    
    class MainGo entry
    class Handlers,Models business
    class Repos,Database data
    class Auth security
    class Config,WebUtils,Docs config
```

## 6. Tecnologias e Dependências

### Core Dependencies
- **Go 1.23+**: Linguagem principal
- **Gorilla Mux**: Roteamento HTTP
- **pgx/v5**: Driver PostgreSQL
- **UUID**: Identificadores únicos
- **JWT**: Autenticação baseada em tokens
- **bcrypt**: Hash de senhas

### External Services
- **PostgreSQL**: Banco de dados principal (via Supabase)
- **Render/Heroku**: Hospedagem da aplicação

### Environment Variables
- `DATABASE_URL`: Connection string do PostgreSQL
- `JWT_SECRET`: Chave secreta para tokens JWT
- `PORT`: Porta do servidor (auto-detectada se não definida)

## 7. Padrões Arquiteturais

### Clean Architecture
- **Separation of Concerns**: Cada camada tem responsabilidade específica
- **Dependency Inversion**: Repositories como interfaces, implementações injetadas
- **Independence**: Regras de negócio independentes de frameworks

### Repository Pattern
- Abstração do acesso a dados
- Interfaces para testabilidade
- Implementações específicas para PostgreSQL

### Middleware Pattern
- Autenticação centralizada
- Interceptação de requests
- Context enrichment

### RESTful API Design
- Recursos bem definidos
- Métodos HTTP semânticos
- Status codes apropriados
- JSON como formato padrão