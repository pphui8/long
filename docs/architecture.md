# Project Architecture

## System Architecture

The system is designed as a distributed application with a clear separation between the frontend, backend, and external services, coordinated by a central reverse proxy.

### Deployment Overview

- **Reverse Proxy**: Nginx serves as the entry point for all incoming traffic on the domain.
- **Frontend**:
    - **Path**: `/`
    - **Container**: Dockerized Nginx hosting a React project (`long-web`).
    - **Internal Port**: 9000
- **Backend**:
    - **Path**: `/api`
    - **Container**: Dockerized Golang application (`long`).
    - **Dependency**: Communicates with external LLM platforms.

### Traffic Flow

1. **User Request** -> **Nginx (Reverse Proxy)**
2. **Routing Decision**:
    - If path is `/`: Proxied to **Frontend Container** (React).
    - If path is `/api`: Proxied to **Backend Container** (Golang).
3. **Backend Logic**:
    - Validates Authentication (JWT).
    - Executes business logic in **Service Layer**.
    - Interacts with **Postgres** (User data) and **Redis** (Token Store).
    - Communicates with **LLM Platform** for AI requests.

### Configuration

The backend application is configured via `env.yaml`, which specifies:
- **App**: Port and environment settings.
- **Postgres**: Connection details for the user database.
- **Redis**: Connection details for the token store.
- **LLM**: API keys and model configurations for external AI services.

---

## Backend Architecture

The backend is built using Golang, following a layered architecture for maintainability and scalability.

### Authentication & Security

The system uses JWT-based authentication with the following token lifecycle:
- **Access Token**: Valid for **30 minutes**. Signed with HMAC-SHA256. Audience: `long-api`.
- **Refresh Token**: Valid for **30 days**. Signed with HMAC-SHA256. Audience: `long-refresh`.
- **Token Rotation**: Every time a refresh token is used, a new pair of access and refresh tokens is issued.
- **Revocation & Reuse Detection**:
    - `TokenStore` (implemented via **Redis**) tracks active and revoked tokens.
    - If a revoked token is used, it indicates a potential reuse attack; the system invalidates all active sessions for that user.
- **CORS**: Restricted to `https://llm.pphui8.com` with credentials allowed.

### Package Structure

| Package | Responsibility |
| :--- | :--- |
| `auth` | JWT generation/validation, Redis-based token store. |
| `cmd/long` | Main entry point; initializes logger, config, and starts the server. |
| `db` | System-level database initialization and global instance management. |
| `domain` | Centralized data models, request/response structures, and interfaces. |
| `handler` | Gin-based HTTP handlers for request validation and response formatting. |
| `logger` | Global structured logging configuration using `uber-go/zap`. |
| `repository` | Data persistence layer for Postgres (users) and placeholder for LLM results. |
| `router` | Definition of API routes, middleware registration, and CORS setup. |
| `service` | Business logic implementation and orchestration (e.g., LLM processing). |

### API Documentation

#### Public Endpoints

| Method | Endpoint | Function | Description |
| :--- | :--- | :--- | :--- |
| `POST` | `/login` | `HandleLogin` | Authenticates user credentials, issues access token and refresh token (via HttpOnly cookie). |
| `POST` | `/refresh` | `HandleRefresh` | Rotates tokens; issues new access and refresh tokens. |
| `GET` | `/ping` | `HandlePing` | Health check verifying availability of Redis and Postgres. |

#### Protected Endpoints (Requires JWT)

| Method | Endpoint | Function | Description |
| :--- | :--- | :--- | :--- |
| `GET` | `/resource` | `HandleResource` | Returns user-specific information from the JWT context. |

### Component Roles

- **Auth Middleware**: Verifies the `Authorization: Bearer <token>` header, ensures the audience is `long-api`, and injects the username into the Gin context.
- **DB Module**: Provides a central location for the database connection pool (`db.Instance`).
- **LLM Service**: (`service/llm.go`) Encapsulates logic for interacting with external AI providers.
- **LLM Repository**: (`repository/llm.go`) Provides an interface for persisting AI interaction history.
- **User Repository**: (`repository/user.go`) Handles Postgres operations for user authentication.
