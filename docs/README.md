# Lang Learn App — Documentation Index

> Complete engineering and pedagogical documentation for **Lang Learn App** (`langapp`), a local-first bilingual English and Chinese learning application built with Go, PostgreSQL, GraphQL, Vue 3, and gRPC audio services.

---

## 1. System Architecture

The technical architecture follows Clean Architecture and C4 model specifications:

- [01 — System Context](architecture/01-context.md) — System boundaries, external actors, datastores, and trust model.
- [02 — Containers & Deployment](architecture/02-containers.md) — Docker Compose topology, ports, health checks, and process boundaries.
- [03 — Components & Modules](architecture/03-components.md) — Clean Architecture layers, vertical slices, and DI wiring.
- [04 — Request Lifecycle](architecture/04-request-lifecycle.md) — Gin middleware, GraphQL execution, Unit of Work transactions, and error pipeline.
- [05 — Data Model (ERD)](architecture/05-data-model.md) — PostgreSQL schema, relational tables, indexes, triggers, and sync columns.
- [06 — Engineering Conventions](architecture/06-conventions.md) — Import rules, layering invariants, wire format, and test database isolation.
- [Architecture Overview](architecture/README.md) — Architecture index and reading guide.

---

## 2. Feature Specifications

Detailed domain designs and implementation details for each bounded context:

- [Roadmap & Interactive Map](features/roadmap.md) — 5-level curriculum hierarchy, landscape SVG canvas, and bookmarks.
- [Spaced Repetition System (SRS)](features/srs.md) — Flashcards, FSRS memory scheduling, fallback intervals (1-3-7-14-30), and review logs.
- [Language Content & Dictionaries](features/content.md) — FTS dictionaries, HSK 1–4, stroke lookups, tone grading, English stress, chunking, and 8-axis THIEU checklist.
- [Practice & Error Book](features/practice.md) — Shadowing audio player, token-level speech transcript diffing (LCS), and mistake logs.
- [Learning Insights & Analytics](features/insight.md) — Dashboard metrics, retention accuracy, consecutive UTC day streak tracking, and top error frequency.
- [Audio Subsystem & Speech Services](features/audio.md) — Out-of-process gRPC audio service, Piper neural TTS, Faster-Whisper proxy, and sine stub fallbacks.
- [Offline Peer Synchronization](features/sync.md) — Snapshot JSON packaging, Last-Write-Wins (LWW) conflict resolution, and soft tombstones.
- [Features Overview](features/README.md) — Features index and navigation.

---

## 3. Requirements Specifications

Formal functional and architectural requirement specifications:

- [01 — Interactive Gamified Roadmap Map](requirements/01-roadmap-map.md) — Functional and non-functional requirements for the horizontal game map.
- [02 — Audio Pipeline & Sidecar Isolation](requirements/02-audio-pipeline.md) — Requirements for gRPC speech synthesis and recognition sidecar isolation.
- [03 — Knowledge Graph DAG & Dependency Engine](requirements/03-knowledge-graph-dag.md) — Non-linear learning paths, stage clusters, prerequisite edges, and topological unlock engine.
- [04 — Multi-Modal Resource Center](requirements/04-multimodal-resources.md) — Video timestamps, PDF page-level tracking, markdown notebooks, and granular progress.
- [05 — OpenAI-Compatible AI Agent Gateway](requirements/05-openai-ai-agent.md) — Automated DAG synthesis, active recall quiz generation, and Socratic copilot.
- [06 — WebGPU & Wasm Graph Canvas](requirements/06-webgpu-wasm-canvas.md) — Rust-to-Wasm graph engine, WebGPU WGSL compute physics, and 60-120 FPS constellation view.
- [Requirements Overview](requirements/README.md) — Requirements index.

---

## 4. Curriculum & Learning Roadmaps

Curated learning tracks, pedagogical methods, and reference materials:

- **English Curriculum Track**:
  - [Overview & Roadmap](learn_english/docs/01-lo-trinh-tong-quan.md)
  - [Listening & Speaking Backbone](learn_english/docs/02-ngong-nghe-noi-backbone.md)
  - [Vocabulary & Transition Collocations (TAP & PVO)](learn_english/docs/03-tap-tu-vung-chuyen-y.md)
  - [Academic Writing & THIEU Rubric](learn_english/docs/04-thieu-viet-hoc-thuat.md)
  - [Long-term Learning Loops](learn_english/docs/05-knowledge-sharing-va-loop-dai-han.md)
  - [References](learn_english/docs/06-tai-lieu-tham-khao.md)
- **Chinese Curriculum Track**:
  - [Overview & Roadmap](learn_chinese/docs/01-lo-trinh-tong-quan.md)
  - [Pinyin & Tones Backbone](learn_chinese/docs/02-nghe-noi-thanh-dieu-backbone.md)
  - [Hanzi Characters, Vocabulary & SRS](learn_chinese/docs/03-chu-han-tu-vung-srs.md)
  - [Grammar, Reading & Writing](learn_chinese/docs/04-ngu-phap-doc-viet.md)
  - [Long-term Learning Loops](learn_chinese/docs/05-loop-dai-han-va-tai-lieu.md)
  - [References](learn_chinese/docs/06-tai-lieu-tham-khao.md)
