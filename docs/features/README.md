# Feature Specifications

> Comprehensive domain and capability documentation for the Lang Learn App vertical slices.

## Feature Modules

| Module | Document | Description |
| --- | --- | --- |
| **Roadmap** | [roadmap.md](roadmap.md) | 5-level curriculum hierarchy, interactive landscape game map, and personal bookmark management. |
| **SRS** | [srs.md](srs.md) | Spaced repetition memory scheduler (FSRS & fallback intervals), flashcard decks, reviews, and notes. |
| **Content** | [content.md](content.md) | Bilingual dictionaries (FTS), HSK 1–4 vocabulary, stroke orders, tone grading, English stress rules, chunking, and 8-axis THIEU checklist. |
| **Practice** | [practice.md](practice.md) | Shadowing audio player, token-level speech transcript diffing (LCS), and error notebook. |
| **Insight** | [insight.md](insight.md) | Aggregate dashboard metrics, retention accuracy rates, UTC consecutive day streak tracking, and mistake frequencies. |
| **Audio** | [audio.md](audio.md) | Out-of-process gRPC audio microservice (Piper neural TTS, Faster-Whisper STT proxy, and fail-degraded sine stub). |
| **Sync** | [sync.md](sync.md) | Offline peer-to-peer synchronization, snapshot JSON packaging, and Last-Write-Wins (LWW) conflict resolution. |

## Quick Links

- Architecture Documentation: [docs/architecture/](file:///home/hung1/personal/roadmap-learning/docs/architecture/)
- Requirements Documentation: [docs/requirements/](file:///home/hung1/personal/roadmap-learning/docs/requirements/)
- Learning Tracks: [English Track](file:///home/hung1/personal/roadmap-learning/docs/learn_english/docs/README.md) · [Chinese Track](file:///home/hung1/personal/roadmap-learning/docs/learn_chinese/docs/README.md)
