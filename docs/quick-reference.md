# Quick Architecture Reference

##  High-Level Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                        Go Cart API                              │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────┐    ┌──────────────┐    ┌─────────────────┐    │
│  │   Client    │───▶│   Router     │───▶│   Middleware    │    │
│  │ (HTTP/JSON) │    │(Gorilla Mux) │    │ (JWT Auth)      │    │
│  └─────────────┘    └──────────────┘    └─────────────────┘    │
│                                                   │             │
│                                                   ▼             │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                   Handlers                              │    │
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────────┐   │    │
│  │  │  Auth   │ │  User   │ │Product  │ │   Cart &    │   │    │
│  │  │Handler  │ │Handler  │ │Handler  │ │   Orders    │   │    │
│  │  └─────────┘ └─────────┘ └─────────┘ └─────────────┘   │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                   │             │
│                                                   ▼             │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                 Repositories                            │    │
│  │  ┌─────────┐ ┌─────────┐ ┌─────────┐ ┌─────────────┐   │    │
│  │  │  User   │ │Product  │ │Category │ │ Cart/Order  │   │    │
│  │  │  Repo   │ │  Repo   │ │  Repo   │ │    Repos    │   │    │
│  │  └─────────┘ └─────────┘ └─────────┘ └─────────────┘   │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                   │             │
│                                                   ▼             │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │               Database Layer                            │    │
│  │                 PostgreSQL                              │    │
│  │                (via Supabase)                          │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

##  Key Components

| Component | Technology | Purpose |
|-----------|------------|---------|
| **Router** | Gorilla Mux | HTTP routing and URL handling |
| **Auth** | JWT + bcrypt | User authentication and security |
| **Handlers** | Go stdlib | HTTP request/response processing |
| **Repositories** | pgx/v5 | Database abstraction layer |
| **Database** | PostgreSQL | Data persistence (via Supabase) |

##  Data Flow

```
Request → Router → Auth Middleware → Handler → Repository → Database
                                                      ↓
Response ← JSON ← Processing ← Query Result ← PostgreSQL
```

##  Deployment

- **Platform**: Docker (VPS/PaaS)
- **Database**: Supabase (PostgreSQL)
- **Port**: Auto-detected (4445+ range)
- **Health Check**: `GET /api/health`
