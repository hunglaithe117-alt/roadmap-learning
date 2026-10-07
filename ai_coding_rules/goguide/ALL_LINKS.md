# Comprehensive Directory of Go Style Code Links

This document aggregates and categorizes **every link, anchor, and external reference** from the [Google Go Style Guide](https://google.github.io/styleguide/go/) and its accompanying canonical documents.

---

## Table of Contents

1. [Official Google Go Style Guide Sections](#1-official-google-go-style-guide-sections)
   - [Overview & Foundations](#overview--foundations)
   - [Core Style Guide (Canonical)](#core-style-guide-canonical)
   - [Style Decisions (Normative Specifics)](#style-decisions-normative-specifics)
   - [Best Practices (Patterns & Idioms)](#best-practices-patterns--idioms)
2. [Official Go Documentation & Specifications](#2-official-go-documentation--specifications)
3. [Official Go Blogs, Articles & Proposals](#3-official-go-blogs-articles--proposals)
4. [Google Code Health & Testing on the Toilet (TotT)](#4-google-code-health--testing-on-the-toilet-tott)
5. [Standard Library & Tooling Documentation](#5-standard-library--tooling-documentation)
6. [External Packages & Libraries](#6-external-packages--libraries)
7. [Talks, Proverbs, and Thought Leadership](#7-talks-proverbs-and-thought-leadership)

---

## 1. Official Google Go Style Guide Sections

### Overview & Foundations
*Web: [https://google.github.io/styleguide/go/](https://google.github.io/styleguide/go/) | Local: [00_overview.md](file:///home/hung1/personal/roadmap-learning/ai_coding_rules/00_overview.md)*

| Topic / Section | Online URL | Summary & AI Rule |
| :--- | :--- | :--- |
| **About the Style Guide** | [Overview](https://google.github.io/styleguide/go/#about) | Outlines the scope and purpose of the style guides across Google. |
| **Canonical Definition** | [Canonical](https://google.github.io/styleguide/go/#canonical) | Prescriptive, enduring rules meeting a high standard for all code. |
| **Normative Definition** | [Normative](https://google.github.io/styleguide/go/#normative) | Agreed-upon style elements to keep reviewers and code consistent. |
| **Idiomatic Definition** | [Idiomatic](https://google.github.io/styleguide/go/#idiomatic) | Prevalent patterns familiar to Go readers across the community. |
| **External References** | [References](https://google.github.io/styleguide/go/#references) | Core recommended external reading before readability reviews. |

---

### Core Style Guide (Canonical)
*Web: [https://google.github.io/styleguide/go/guide](https://google.github.io/styleguide/go/guide) | Local: [01_style_guide.md](file:///home/hung1/personal/roadmap-learning/ai_coding_rules/01_style_guide.md)*

| Topic / Section | Online URL | Summary & AI Rule |
| :--- | :--- | :--- |
| **Style Principles** | [Principles](https://google.github.io/styleguide/go/guide#principles) | Core hierarchy: Clarity > Simplicity > Concision > Maintainability > Consistency. |
| **Clarity: Purpose & Rationale** | [Clarity](https://google.github.io/styleguide/go/guide#clarity) | Code must make its purpose and reasons immediately clear to the reader. |
| **Simplicity & Least Mechanism** | [Simplicity](https://google.github.io/styleguide/go/guide#simplicity) | Accomplish goals in the simplest way; avoid clever abstractions. |
| **Concision** | [Concision](https://google.github.io/styleguide/go/guide#concision) | High signal-to-noise ratio; minimize boilerplate that distracts from core logic. |
| **Maintainability** | [Maintainability](https://google.github.io/styleguide/go/guide#maintainability) | Write code that is safe and easy to modify, test, and scale over time. |
| **Consistency** | [Consistency](https://google.github.io/styleguide/go/guide#consistency) | Follow existing codebase conventions unless there is a strong reason to diverge. |

---

### Style Decisions (Normative Specifics)
*Web: [https://google.github.io/styleguide/go/decisions](https://google.github.io/styleguide/go/decisions) | Local: [02_decisions.md](file:///home/hung1/personal/roadmap-learning/ai_coding_rules/02_decisions.md)*

#### Naming
| Topic / Anchor | Online URL | Summary & AI Rule |
| :--- | :--- | :--- |
| **Underscores** | [#underscores](https://google.github.io/styleguide/go/decisions#underscores) | Never use underscores in Go names (except generated code or test names). |
| **Package Names** | [#package-names](https://google.github.io/styleguide/go/decisions#package-names) | Lowercase, single word, no underscores, matches directory, describes contents. |
| **Receiver Names** | [#receiver-names](https://google.github.io/styleguide/go/decisions#receiver-names) | 1-2 letters, abbreviation of type; never use `this` or `self`; keep consistent. |
| **Constant Names** | [#constant-names](https://google.github.io/styleguide/go/decisions#constant-names) | Use `MixedCaps`; never `ALL_CAPS` or `kConstant`. |
| **Initialisms** | [#initialisms](https://google.github.io/styleguide/go/decisions#initialisms) | Keep consistent casing for acronyms: `URL`, `HTTP`, `ID`, `JSON`, `xmlAPI`. |
| **Getters** | [#getters](https://google.github.io/styleguide/go/decisions#getters) | Do not prefix getters with `Get`: use `user.Name()`, not `user.GetName()`. |
| **Variable Names** | [#variable-names](https://google.github.io/styleguide/go/decisions#variable-names) | Short lifetime = short name; longer scope = descriptive name. |
| **Repetition in Naming** | [#repetition](https://google.github.io/styleguide/go/decisions#repetition) | Avoid stutter: prefer `user.Name` over `user.UserName`, `client.New()` over `client.NewClient()`. |

#### Commentary & Documentation
| Topic / Anchor | Online URL | Summary & AI Rule |
| :--- | :--- | :--- |
| **Comment Length** | [#comment-line-length](https://google.github.io/styleguide/go/decisions#comment-line-length) | Aim for line length <= 80 characters for readability. |
| **Doc Comments** | [#doc-comments](https://google.github.io/styleguide/go/decisions#doc-comments) | Every exported symbol must have a doc comment explaining what and why. |
| **Comment Sentences** | [#comment-sentences](https://google.github.io/styleguide/go/decisions#comment-sentences) | Start with symbol name, write complete sentences, end with punctuation. |
| **Package Comments** | [#package-comments](https://google.github.io/styleguide/go/decisions#package-comments) | Begins with `// Package <name> ...` above package clause or in `doc.go`. |
| **Named Return Params** | [#named-result-parameters](https://google.github.io/styleguide/go/decisions#named-result-parameters) | Only name return parameters when clarifying meaning or needed for defer closures. |

#### Imports
| Topic / Anchor | Online URL | Summary & AI Rule |
| :--- | :--- | :--- |
| **Import Grouping** | [#import-grouping](https://google.github.io/styleguide/go/decisions#import-grouping) | Standard library first, then third-party; separated by a blank line. |
| **Import Renaming** | [#import-renaming](https://google.github.io/styleguide/go/decisions#import-renaming) | Only rename to avoid collisions or clarify obscure package names. |
| **Blank Imports** | [#import-blank](https://google.github.io/styleguide/go/decisions#import-blank) | Use `import _` only in main or test packages for explicit side effects (drivers). |
| **Dot Imports** | [#import-dot](https://google.github.io/styleguide/go/decisions#import-dot) | `import .` is banned; makes symbol origins ambiguous. |

#### Errors
| Topic / Anchor | Online URL | Summary & AI Rule |
| :--- | :--- | :--- |
| **Returning Errors** | [#returning-errors](https://google.github.io/styleguide/go/decisions#returning-errors) | Errors are values; return `(zero, err)` on failure; check errors immediately. |
| **Error Strings** | [#error-strings](https://google.github.io/styleguide/go/decisions#error-strings) | Lowercase, no trailing punctuation (e.g., `"failed to open file"`). |
| **Handling Errors** | [#handle-errors](https://google.github.io/styleguide/go/decisions#handle-errors) | Handle errors once; do not log and return the same error. |
| **In-band Errors** | [#in-band-errors](https://google.github.io/styleguide/go/decisions#in-band-errors) | Avoid sentinel values like `-1` or `""`; return explicit error or `(val, bool)`. |
| **Indent Error Flow** | [#indent-error-flow](https://google.github.io/styleguide/go/decisions#indent-error-flow) | Keep happy path un-indented on the left; return early on error (`if err != nil`). |

#### Language & Types
| Topic / Anchor | Online URL | Summary & AI Rule |
| :--- | :--- | :--- |
| **Literal Formatting** | [#literal-formatting](https://google.github.io/styleguide/go/decisions#literal-formatting) | Always include field names when initializing structs with 2+ fields. |
| **Nil Slices** | [#nil-slices](https://google.github.io/styleguide/go/decisions#nil-slices) | Prefer `var s []T` (nil slice) over `s := []T{}` unless empty non-nil is needed. |
| **Copying & Mutexes** | [#copying](https://google.github.io/styleguide/go/decisions#copying) | Never copy structs containing `sync.Mutex` or pointers by value. |
| **Don't Panic** | [#dont-panic](https://google.github.io/styleguide/go/decisions#dont-panic) | Never panic for expected errors; panic only on programmer bug at initialization. |
| **Goroutine Lifetimes** | [#goroutine-lifetimes](https://google.github.io/styleguide/go/decisions#goroutine-lifetimes) | Every goroutine must have a clean termination condition; avoid leaks. |
| **Interfaces** | [#interfaces](https://google.github.io/styleguide/go/decisions#interfaces) | Define interfaces where consumed, not produced. Keep them small (1-2 methods). |
| **Pass Values** | [#pass-values](https://google.github.io/styleguide/go/decisions#pass-values) | Pass small, immutable values by value; pass pointers for mutation or large structs. |
| **Synchronous Functions** | [#synchronous-functions](https://google.github.io/styleguide/go/decisions#synchronous-functions) | Functions should do their work synchronously; let the caller invoke via `go func()`. |

#### Testing
| Topic / Anchor | Online URL | Summary & AI Rule |
| :--- | :--- | :--- |
| **Useful Failures** | [#useful-test-failures](https://google.github.io/styleguide/go/decisions#useful-test-failures) | Format failure messages as `got X, want Y` so differences are immediately clear. |
| **Table-Driven Tests** | [#table-driven-tests](https://google.github.io/styleguide/go/decisions#table-driven-tests) | Use slice of test structs with descriptive names and `t.Run`. |
| **Test Helpers** | [#test-helpers](https://google.github.io/styleguide/go/decisions#test-helpers) | Always invoke `t.Helper()` at the start of test helper functions. |
| **Equality & Diffs** | [#equality-comparison-and-diffs](https://google.github.io/styleguide/go/decisions#equality-comparison-and-diffs) | Use `cmp.Diff(want, got)` for comparing complex structs instead of manual checks. |

---

### Best Practices (Patterns & Idioms)
*Web: [https://google.github.io/styleguide/go/best-practices](https://google.github.io/styleguide/go/best-practices) | Local: [03_best_practices.md](file:///home/hung1/personal/roadmap-learning/ai_coding_rules/03_best_practices.md)*

| Topic / Anchor | Online URL | Summary & AI Rule |
| :--- | :--- | :--- |
| **Package Size** | [#package-size](https://google.github.io/styleguide/go/best-practices#package-size) | Group by cohesion, not size; keep types together with their operations. |
| **Error Wrapping & %w** | [#placement-of-w-in-errors](https://google.github.io/styleguide/go/best-practices#placement-of-w-in-errors) | Use `%w` only when exposing the underlying error as part of the public API contract. |
| **Sentinel Placement** | [#sentinel-error-placement](https://google.github.io/styleguide/go/best-practices#sentinel-error-placement) | Place sentinels (`ErrNotFound`) at the package level; use `errors.Is` to check. |
| **When to Panic** | [#when-to-panic](https://google.github.io/styleguide/go/best-practices#when-to-panic) | Use `Must...` pattern only during package `init` or static setup that cannot fail. |
| **Context Conventions** | [#contexts](https://google.github.io/styleguide/go/best-practices#contexts) | `ctx context.Context` is the first parameter; never store context in a struct. |
| **Time & Durations** | [#time](https://google.github.io/styleguide/go/best-practices#time) | Use `time.Duration` for elapsed time/intervals; `time.Time` for instant in time. |
| **Struct Tags** | [#struct-tags](https://google.github.io/styleguide/go/best-practices#struct-tags) | Keep tags consistent; specify `json:",omitempty"` appropriately. |
| **Option Functions** | [#variadic-options](https://google.github.io/styleguide/go/best-practices#variadic-options) | Use functional options pattern for extensible constructor configurations. |
| **String Concatenation** | [#string-concatenation](https://google.github.io/styleguide/go/best-practices#string-concatenation) | Use `+` for simple cases, `fmt.Sprintf` for formatting, `strings.Builder` for loops. |
| **Global State** | [#global-state](https://google.github.io/styleguide/go/best-practices#global-state) | Avoid package-level mutable global state; pass dependencies explicitly. |

---

## 2. Official Go Documentation & Specifications

| Resource | Link | Description |
| :--- | :--- | :--- |
| **Effective Go** | [https://go.dev/doc/effective_go](https://go.dev/doc/effective_go) | Foundation of Go style, idioms, and baseline patterns across the Go community. *(Local: [04_effective_go.md](file:///home/hung1/personal/roadmap-learning/ai_coding_rules/04_effective_go.md))* |
| **Go Code Review Comments** | [https://go.dev/wiki/CodeReviewComments](https://go.dev/wiki/CodeReviewComments) | Standard checklist for Go code reviewers; concise style rules. *(Local: [05_code_review_comments.md](file:///home/hung1/personal/roadmap-learning/ai_coding_rules/05_code_review_comments.md))* |
| **Go Language Specification** | [https://go.dev/ref/spec](https://go.dev/ref/spec) | The definitive technical specification of Go syntax, semantics, and types. |
| **Go Memory Model** | [https://go.dev/ref/mem](https://go.dev/ref/mem) | Defines the conditions under which reads of a variable in one goroutine see writes by another. |
| **Go FAQ** | [https://go.dev/doc/faq](https://go.dev/doc/faq) | Official answers to common design, syntax, and concurrency questions. |
| **Go Doc Comments Guide** | [https://go.dev/doc/comment](https://go.dev/doc/comment) | Official guide for writing documentation comments formatted for `godoc` and `pkgsite`. |
| **Generics Tutorial** | [https://go.dev/doc/tutorial/generics](https://go.dev/doc/tutorial/generics) | Introduction to type parameters in Go. |

---

## 3. Official Go Blogs, Articles & Proposals

| Article / Proposal | Link | Topic / Key Takeaway |
| :--- | :--- | :--- |
| **Working with Errors in Go 1.13** | [https://go.dev/blog/go1.13-errors](https://go.dev/blog/go1.13-errors) | Guide to error wrapping (`%w`), `errors.Is`, and `errors.As`. |
| **Errors Are Values** | [https://go.dev/blog/errors-are-values](https://go.dev/blog/errors-are-values) | How to write elegant error handling patterns using structs and writers. |
| **Context and Structs** | [https://go.dev/blog/context-and-structs](https://go.dev/blog/context-and-structs) | Why context should be passed as the first parameter and never stored in a struct. |
| **Package Names** | [https://go.dev/blog/package-names](https://go.dev/blog/package-names) | Practical conventions for clean, expressive Go package names. |
| **Organizing Go Code** | [https://go.dev/blog/organizing-go-code](https://go.dev/blog/organizing-go-code) | How to organize packages, files, and commands in a Go project. |
| **Defer, Panic, and Recover** | [https://go.dev/blog/defer-panic-and-recover](https://go.dev/blog/defer-panic-and-recover) | Safe usage and semantics of control flow mechanisms. |
| **Testable Examples in Go** | [https://go.dev/blog/examples](https://go.dev/blog/examples) | Writing executable, verified documentation examples with `// Output:`. |
| **Type Parameters Proposal** | [https://go.dev/design/43651-type-parameters](https://go.dev/design/43651-type-parameters) | Complete specification for Go type parameters (Generics). |
| **Type Alias Proposal** | [https://go.googlesource.com/proposal/+/master/design/18130-type-alias.md](https://go.googlesource.com/proposal/+/master/design/18130-type-alias.md) | Design for codebase refactoring via type aliases (`type T = pkg.T`). |

---

## 4. Google Code Health & Testing on the Toilet (TotT)

| TotT Article | Link | Core Principle |
| :--- | :--- | :--- |
| **Identifier Naming** | [TotT: Identifier Naming](https://testing.googleblog.com/2017/10/code-health-identifiernamingpostforworl.html) | Name identifiers for clarity in their calling context; avoid redundant qualifiers. |
| **State vs. Interactions** | [TotT: State vs Interactions](https://testing.googleblog.com/2013/03/testing-on-toilet-testing-state-vs.html) | Prefer asserting observable state changes over verifying mock function call sequences. |
| **Effective Testing** | [TotT: Effective Testing](https://testing.googleblog.com/2014/05/testing-on-toilet-effective-testing.html) | Write readable, deterministic, hermetic tests that clearly document intent. |
| **Risk-Driven Testing** | [TotT: Risk-driven Testing](https://testing.googleblog.com/2014/05/testing-on-toilet-risk-driven-testing.html) | Focus test effort on business risk and complexity rather than raw code coverage. |
| **Change-Detector Tests** | [TotT: Change-Detector Tests](https://testing.googleblog.com/2015/01/testing-on-toilet-change-detector-tests.html) | Avoid brittle tests that break whenever implementation changes without behavior change. |
| **Reduce Nesting, Reduce Complexity** | [TotT: Reduce Nesting](https://testing.googleblog.com/2017/06/code-health-reduce-nesting-reduce.html) | Return early; avoid deeply nested `if/else` blocks to reduce cyclomatic complexity. |
| **Data-Driven Traps** | [TotT: Data-Driven Traps](https://testing.googleblog.com/2008/09/tott-data-driven-traps.html) | Avoid complex looping logic inside test tables that obscures why tests fail. |

---

## 5. Standard Library & Tooling Documentation

| Library / Tool | Link | Purpose / Context in Style Guide |
| :--- | :--- | :--- |
| **`testing`** | [https://pkg.go.dev/testing](https://pkg.go.dev/testing) | The core unit test package (`*testing.T`, `t.Run`, `t.Helper`, `t.Cleanup`). |
| **`context`** | [https://pkg.go.dev/context](https://pkg.go.dev/context) | Standard interface for request lifecycles, cancellation, and deadlines. |
| **`errors`** | [https://pkg.go.dev/errors](https://pkg.go.dev/errors) | Error handling primitives (`errors.Is`, `errors.As`, `errors.New`). |
| **`log/slog`** | [https://pkg.go.dev/log/slog](https://pkg.go.dev/log/slog) | Standard library structured logging (replaces custom log wrappers). |
| **`sync` & `Mutex`** | [https://pkg.go.dev/sync](https://pkg.go.dev/sync) | Concurrency synchronization; structs containing `sync.Mutex` must not be copied. |
| **`strings.Cut`** | [https://pkg.go.dev/strings#Cut](https://pkg.go.dev/strings#Cut) | Recommended idiom for string splitting (cleaner than `strings.Split` for key=val). |
| **`io.Writer`** | [https://pkg.go.dev/io#Writer](https://pkg.go.dev/io#Writer) | Canonical single-method interface design pattern in Go. |
| **`gofmt`** | [https://pkg.go.dev/cmd/gofmt/](https://pkg.go.dev/cmd/gofmt/) | The standard Go source code formatting tool. |
| **`goimports`** | [https://pkg.go.dev/golang.org/x/tools/cmd/goimports](https://pkg.go.dev/golang.org/x/tools/cmd/goimports) | Tool to format code and automatically manage import lines. |

---

## 6. External Packages & Libraries

| Package | Link | Usage / Recommendation in Guide |
| :--- | :--- | :--- |
| **`go-cmp` (`cmp`)** | [https://pkg.go.dev/github.com/google/go-cmp/cmp](https://pkg.go.dev/github.com/google/go-cmp/cmp) | **Recommended** standard for equality testing and diff reporting in tests. |
| **`cmpopts`** | [https://pkg.go.dev/github.com/google/go-cmp/cmp/cmpopts](https://pkg.go.dev/github.com/google/go-cmp/cmp/cmpopts) | Options for `go-cmp` (e.g. `EquateErrors`, `IgnoreFields`, `IgnoreInterfaces`). |
| **`protocmp`** | [https://pkg.go.dev/google.golang.org/protobuf/testing/protocmp](https://pkg.go.dev/google.golang.org/protobuf/testing/protocmp) | Protocol buffer comparison transformation for `cmp.Diff`. |
| **`golang.org/x/sync/errgroup`** | [https://pkg.go.dev/golang.org/x/sync/errgroup](https://pkg.go.dev/golang.org/x/sync/errgroup) | Structured concurrency with goroutine error propagation and context cancellation. |
| **`godebug/pretty`** | [https://pkg.go.dev/github.com/kylelemons/godebug/pretty](https://pkg.go.dev/github.com/kylelemons/godebug/pretty) | Alternative lightweight struct comparison and pretty printer. |
| **`cobra`** | [https://pkg.go.dev/github.com/spf13/cobra](https://pkg.go.dev/github.com/spf13/cobra) | Standard library for complex CLI applications with subcommands. |

---

## 7. Talks, Proverbs, and Thought Leadership

| Resource | Link | Summary & Insight |
| :--- | :--- | :--- |
| **Go Proverbs** | [https://go-proverbs.github.io/](https://go-proverbs.github.io/) | Rob Pike's 19 foundational proverbs (e.g., *"Don't communicate by sharing memory, share memory by communicating"*). |
| **Go and Dogma** | [https://research.swtch.com/dogma](https://research.swtch.com/dogma) | Russ Cox on pragmatic engineering and avoiding dogma in Go development. |
| **Go Data Structures** | [https://research.swtch.com/godata](https://research.swtch.com/godata) | Deep architectural look into memory layout of Go structs, slices, and interfaces. |
| **Go Interfaces** | [https://research.swtch.com/interfaces](https://research.swtch.com/interfaces) | Under-the-hood explanation of dynamic dispatch and interface method tables (`itable`). |
| **Less is Exponentially More** | [Blog Post](https://commandcenter.blogspot.com/2012/06/less-is-exponentially-more.html) | Rob Pike explaining the design decisions and deliberate omissions in Go. |
| **Gofmt Talk (YouTube)** | [YouTube Video](https://www.youtube.com/watch?v=PAAkCSZUG1c&t=8m43s) | Rob Pike on why mechanical formatting eliminates bikeshedding and debates. |
| **Simplicity is Complicated** | [YouTube Video](https://www.youtube.com/watch?v=rFejpH_tAHM) | Rob Pike exploring what true simplicity means in software engineering. |
