# Data Model (ERD)

> One-sentence description of the persisted schema, its shared mixins, and the areas the entities fall into.

## 1. Shared mixins

> One row per mixin every entity may compose: the columns it contributes and where it is defined.

| Mixin     | Columns                    | Source             |
| --------- | -------------------------- | ------------------ |
| `<Mixin>` | `<columns it contributes>` | `<path/to/source>` |

\<State the shared base all entities subclass and any exceptions to the mixin defaults — for example non-UUID primary keys.>

## 2. Identity & access context

> One `erDiagram` for the identity, credential, and grant entities, then cite the source of each entity and call out any missing foreign keys or partial indexes.

```mermaid
erDiagram
    accTitle: <Area> entities
    accDescr: <One sentence describing the identity, credential, and grant entities and their relationships.>
    PRINCIPAL {
        uuid id PK
        string email
        string hashed_secret
    }
    SESSION {
        string id PK
        uuid principal_id FK
        datetime expires_at
    }
    ROLE {
        int id PK
        string name
    }
    PERMISSION {
        int id PK
        string resource
        string action
    }
    ROLE_PERMISSION {
        int role_id PK
        int permission_id PK
    }
    PRINCIPAL_ROLE {
        uuid principal_id PK
        int role_id PK
    }

    PRINCIPAL ||--o{ SESSION : "owns"
    PRINCIPAL ||--o{ PRINCIPAL_ROLE : "assigned"
    ROLE ||--o{ PRINCIPAL_ROLE : "assigned to"
    ROLE ||--o{ ROLE_PERMISSION : "grouped by"
    PERMISSION ||--o{ ROLE_PERMISSION : "attached to"
```

Sources: `<path/to/source>` (`<entity>`), `<path/to/source>` (`<entity>`).

> \<Call out any column that looks like a foreign key but is not constrained, and any partial or conditional index.>

## 3. \<Area> configuration context

> One `erDiagram` for the tenant and its per-tenant configuration entities, then cite the sources and call out unconstrained references or cascade behavior.

```mermaid
erDiagram
    accTitle: <Area> configuration entities
    accDescr: <One sentence describing the tenant and the per-tenant configuration entities and their relationships.>
    TENANT {
        string id PK
        string name
        int status
    }
    CONFIG_ITEM {
        uuid id PK
        string tenant_id FK
        string key
        jsonb value
    }
    CATALOG_ITEM {
        uuid id PK
        string tenant_id
        int category
    }
    RULE {
        uuid id PK
        string tenant_id
        string action
        bigint points
    }

    TENANT ||--o{ CONFIG_ITEM : "defines"
    TENANT ||--o{ CATALOG_ITEM : "catalogs"
    TENANT ||--o{ RULE : "rules"
```

Sources: `<path/to/source>` (`<entity>`).

> \<Call out any unconstrained tenant reference and any cascade or logical-link behavior.>

## 4. \<Area> & \<Area> context

> One `erDiagram` for the end-user, participation, and ranking entities, then cite the sources and call out unconstrained references or uniqueness constraints.

```mermaid
erDiagram
    accTitle: <Area> and ranking entities
    accDescr: <One sentence describing the end-user, participation, and ranking entities and their relationships.>
    TENANT {
        string id PK
    }
    END_USER {
        uuid id PK
        string email
        string tenant_id FK
    }
    PARTICIPATION {
        uuid user_id PK
        string tenant_id PK
        datetime first_seen_at
    }
    BOARD {
        string id PK
        string tenant_id
        int status
    }
    BOARD_PERIOD {
        string id PK
        string board_id FK
        datetime start_at
        datetime end_at
    }
    BOARD_ENTRY {
        bigint id PK
        string period_id FK
        string user_id
        bigint amount
    }

    END_USER ||--o{ PARTICIPATION : "participates"
    TENANT ||--o{ PARTICIPATION : "played by"
    BOARD ||--o{ BOARD_PERIOD : "resolves"
    BOARD_PERIOD ||--o{ BOARD_ENTRY : "contains"
```

Sources: `<path/to/source>` (`<entity>`).

> \<Call out any unconstrained reference and any uniqueness constraint.>

## 5. Table index

> One row per persisted table: its owning module and its purpose.

| Table     | Owning module     | Purpose     |
| --------- | ----------------- | ----------- |
| `<table>` | `<owning module>` | `<purpose>` |

## 6. Verification note

> State how the entity and table names above were verified against the code, and how registration is forced, so a later reader can re-check.

\<Prose: describe the check performed and its result, and how models are registered.>

## 7. References

> Link the mixins/base and the sibling chapters.

- Mixins/base: `<path/to/source>`
- Related docs: [03 — Components](03-components.md), [06 — Conventions](06-conventions.md)
