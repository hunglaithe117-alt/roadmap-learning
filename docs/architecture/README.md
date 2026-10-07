# Architecture Documentation

> Comprehensive C4 and engineering architecture documentation for Lang Learn App (`langapp`).

## Architecture Chapters

| Chapter | Title | Focus |
| --- | --- | --- |
| [01 — Context](01-context.md) | **System Context** | System scope, external actors, high-level dependencies, and trust boundaries. |
| [02 — Containers](02-containers.md) | **Containers & Deployment** | Docker Compose topology, ports, health checks, storage, and audio sidecars. |
| [03 — Components](03-components.md) | **Components & Modules** | Clean Architecture layers, vertical slices, application services, and inter-module adapters. |
| [04 — Request Lifecycle](04-request-lifecycle.md) | **Request Lifecycle** | Middleware execution, GraphQL dataloading, unit of work transactions, and error handling. |
| [05 — Data Model](05-data-model.md) | **Data Model (ERD)** | PostgreSQL relational schema, indexes, trigger functions, and synchronization columns. |
| [06 — Conventions](06-conventions.md) | **Engineering Conventions** | Coding standards, package boundaries, test database isolation, and wire format contracts. |

## Quick Links

- Application Source: [api/](file:///home/hung1/personal/roadmap-learning/api)
- Web Frontend: [web/](file:///home/hung1/personal/roadmap-learning/web)
- Deployment Config: [docker-compose.yml](file:///home/hung1/personal/roadmap-learning/docker-compose.yml)
- Deployment Guide: [DEPLOY.md](file:///home/hung1/personal/roadmap-learning/DEPLOY.md)
