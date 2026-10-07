# Go Style Code & AI Coding Rules

This directory contains the complete offline collection of the **Google Go Style Guide**, accompanying foundational Go documents, an exhaustive directory of all referenced links, and an actionable ruleset tailored for AI-assisted Go development.

---

## Directory Index

| File | Description | Source / Reference |
| :--- | :--- | :--- |
| [**`AI_GO_CODING_RULES.md`**](file:///home/hung1/personal/roadmap-learning/ai_coding_rules/AI_GO_CODING_RULES.md) | **Actionable AI Ruleset:** High-density system instructions covering naming, errors, concurrency, interfaces, and testing. | Synthesized from Google Go Style & Community Best Practices |
| [**`ALL_LINKS.md`**](file:///home/hung1/personal/roadmap-learning/ai_coding_rules/ALL_LINKS.md) | **Link Catalog:** Categorized directory of all links, anchors, blogs, talks, and packages referenced in the guide. | [Google Go Style Links](https://google.github.io/styleguide/go/) |
| [**`00_overview.md`**](file:///home/hung1/personal/roadmap-learning/ai_coding_rules/00_overview.md) | **Overview & Definitions:** Scope, principles, definitions of *Canonical*, *Normative*, and *Idiomatic*. | [Google Style Guide - Overview](https://google.github.io/styleguide/go/) |
| [**`01_style_guide.md`**](file:///home/hung1/personal/roadmap-learning/ai_coding_rules/01_style_guide.md) | **Core Style Guide (Canonical):** Core foundation: Clarity, Simplicity, Concision, Maintainability, Consistency. | [Google Style Guide - Guide](https://google.github.io/styleguide/go/guide) |
| [**`02_decisions.md`**](file:///home/hung1/personal/roadmap-learning/ai_coding_rules/02_decisions.md) | **Style Decisions (Normative):** Detailed decisions on naming, commentary, imports, error flow, goroutines, interfaces. | [Google Style Guide - Decisions](https://google.github.io/styleguide/go/decisions) |
| [**`03_best_practices.md`**](file:///home/hung1/personal/roadmap-learning/ai_coding_rules/03_best_practices.md) | **Best Practices (Patterns):** Practical guidance on table-driven tests, `cmp.Diff`, contexts, time, error wrapping `%w`. | [Google Style Guide - Best Practices](https://google.github.io/styleguide/go/best-practices) |
| [**`04_effective_go.md`**](file:///home/hung1/personal/roadmap-learning/ai_coding_rules/04_effective_go.md) | **Effective Go:** The baseline guide for writing clear, idiomatic Go code across the Go community. | [Effective Go](https://go.dev/doc/effective_go) |
| [**`05_code_review_comments.md`**](file:///home/hung1/personal/roadmap-learning/ai_coding_rules/05_code_review_comments.md) | **Go Code Review Comments:** Standard checklist of common code review comments from the Go team. | [Go Code Review Comments](https://go.dev/wiki/CodeReviewComments) |

---

## How AI Assistants Should Use This Folder

1. **For Prompt Context & Generation:**
   - Inject or read [**`AI_GO_CODING_RULES.md`**](file:///home/hung1/personal/roadmap-learning/ai_coding_rules/AI_GO_CODING_RULES.md) into the prompt context when generating, reviewing, or refactoring Go code.
2. **For Resolving Specific Idiom Questions:**
   - Refer to [**`02_decisions.md`**](file:///home/hung1/personal/roadmap-learning/ai_coding_rules/02_decisions.md) for concrete rules on receivers, naming, package layout, and error conventions.
   - Refer to [**`03_best_practices.md`**](file:///home/hung1/personal/roadmap-learning/ai_coding_rules/03_best_practices.md) for robust test design, context handling, and concurrency safety.
3. **For Navigating External References:**
   - Use [**`ALL_LINKS.md`**](file:///home/hung1/personal/roadmap-learning/ai_coding_rules/ALL_LINKS.md) to locate original specifications, TotT articles, and standard library documentation.
