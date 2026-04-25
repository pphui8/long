# Project Architecture

## System Architecture

The system is designed as a distributed application with a clear separation between the frontend, backend, and external services, coordinated by a central reverse proxy.

### Deployment Overview

- **Reverse Proxy**: Nginx serves as the entry point for all incoming traffic on the domain.
- **Frontend**:
    - **Path**: `/`
    - **Container**: Dockerized Nginx hosting a React project.
    - **Internal Port**: 9000
- **Backend**:
    - **Path**: `/api`
    - **Container**: Dockerized Golang application.
    - **Dependency**: Communicates with external LLM platforms.

### Traffic Flow

`User Request` -> `Nginx (Reverse Proxy)`
- If path is `/`: Proxied to **Frontend Container** (React).
- If path is `/api`: Proxied to **Backend Container** (Golang).
- Backend executes logic and interacts with **LLM Platform**.

---

## Backend Architecture

The backend is built using Golang, following a layered architecture for maintainability and scalability.

### Package Structure

| Package | Responsibility |
| :--- | :--- |
| `auth` | JWT token generation, validation, and authentication middleware. |
| `cmd/long` | Main entry point for the application. |
| `domain` | Centralized data models, request/response structures, and interfaces. |
| `handler` | HTTP request handling, input validation, and response formatting. |
| `logger` | Global logging configuration and utility. |
| `repository` | Data persistence layer (e.g., database interactions). |
| `router` | Definition of API routes and middleware registration. |
| `service` | Business logic implementation and orchestration. |

### API Documentation

#### Public Endpoints

| Method | Endpoint | Function | Description |
| :--- | :--- | :--- | :--- |
| `POST` | `/login` | `HandleLogin` | Authenticates user credentials and returns access/refresh tokens. |
| `POST` | `/refresh` | `HandleRefresh` | Implements refresh token rotation, returning a new access token and a new refresh token. |
| `GET` | `/ping` | `HandlePing` | Health check endpoint to verify service availability. |

#### Protected Endpoints (Requires JWT)

| Method | Endpoint | Function | Description |
| :--- | :--- | :--- | :--- |
| `GET` | `/resource` | `HandleResource` | A sample protected route that returns user-specific information. |

### Component Functions

- **Auth Middleware**: Intercepts requests to protected routes to verify the `Authorization` header and injects identity into the context.
- **LLM Service**: Handles the logic for processing prompts, interacting with LLM APIs, and managing the business flow of AI requests.
- **LLM Repository**: Responsible for persisting LLM interaction results for auditing or history tracking.
