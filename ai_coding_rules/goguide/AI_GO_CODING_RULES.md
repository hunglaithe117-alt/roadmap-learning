# AI Go Coding Rules & Guidelines

> **Purpose:** This document is an actionable, high-density ruleset optimized for AI coding assistants (and engineers) writing and reviewing Go code. It is derived directly from the canonical **Google Go Style Guide**, **Effective Go**, and **Go Code Review Comments**.
>
> When generating or refactoring Go code, strictly adhere to these rules.

---

## 1. Core Principles

1. **Clarity over Cleverness:** Code must make its purpose, edge cases, and rationale obvious to the reader. Avoid obscure constructs.
2. **Simplicity (Least Mechanism):** Solve the problem with the minimum necessary mechanism. Do not introduce premature abstractions, interfaces, or generic frameworks.
3. **Concision:** Maintain a high signal-to-noise ratio. Avoid repetitive boilerplate, redundant comments, or excessive naming prefixes.
4. **Consistency:** Conform to the surrounding codebase style and standard library conventions.

---

## 2. Naming Conventions

### 2.1 Package Names
- **DO:** Use short, lowercase, single-word names that describe what the package provides (e.g., `user`, `http`, `tar`, `postgres`).
- **DON'T:** Use snake_case (`user_service`), camelCase (`userService`), plurals (`users`), or generic utility names (`util`, `common`, `helper`, `shared`, `base`).
- **DON'T:** Stutter package name in exported types:
  ```go
  // Bad
  package user
  type User struct{}
  func NewUser() *User {}

  // Good
  package user
  type Profile struct{} // or simply user.Entity
  func New() *Profile {}
  ```

### 2.2 Variables & Scope
- **Rule of Scope:** Variable name length should be proportional to its scope.
  - Short-lived variables (loops, small functions): single letters or abbreviations (`i`, `k`, `v`, `r`, `w`, `buf`, `ctx`, `err`).
  - Package-level or long-lived variables: descriptive, fully qualified names (`maxRetryAttempts`).
- **Acronyms & Initialisms:** Casing must be consistent (all uppercase or all lowercase in camelCase):
  - **DO:** `userID`, `httpServer`, `apiURL`, `serveHTTP`, `jsonBytes`, `xmlParser`, `parseUUID`.
  - **DON'T:** `userId`, `HttpServer`, `apiUrl`, `serveHttp`, `JsonBytes`.

### 2.3 Getters and Setters
- **DO NOT** prefix getters with `Get`:
  ```go
  // Bad
  func (c *Customer) GetBalance() int

  // Good
  func (c *Customer) Balance() int
  func (c *Customer) SetBalance(b int)
  ```

### 2.4 Receivers
- **DO:** Use 1-2 letter abbreviations matching the type name:
  ```go
  // Good
  func (c *Client) Connect() error
  func (s *Server) Start() error
  ```
- **DON'T:** Use generic object-oriented terms like `this`, `self`, or `me`.
- **Consistency:** Keep receiver names identical across all methods on the same type.

### 2.5 Constants
- **DO:** Use `MixedCaps` or `camelCase` for constants:
  ```go
  const (
      DefaultTimeout = 5 * time.Second
      maxRetries     = 3
  )
  ```
- **DON'T:** Use uppercase snake_case (`MAX_RETRIES`) or Hungarian notation (`kMaxRetries`).

---

## 3. Code Organization & Formatting

### 3.1 Imports
- Group imports into exactly two or three blocks separated by an empty line:
  1. Standard library imports
  2. Third-party package imports
  3. Local/internal module imports (optional sub-group)
  ```go
  import (
      "context"
      "fmt"
      "time"

      "github.com/google/uuid"
      "golang.org/x/sync/errgroup"

      "langapp/internal/store"
  )
  ```
- **BANNED:** Never use dot imports (`import . "foo"`).
- **Blank Imports (`_`):** Only allowed in `main` packages or test files for driver/plugin registration with a documenting comment.

### 3.2 Struct Literals
- **DO:** Always use explicit field names when constructing structs (unless the type has exactly 1 field or represents a basic coordinate/pair):
  ```go
  // Good
  req := Request{
      ID:      uuid.NewString(),
      Payload: data,
      Timeout: 10 * time.Second,
  }

  // Bad
  req := Request{uuid.NewString(), data, 10 * time.Second}
  ```

### 3.3 Nil Slices vs Empty Slices
- Prefer `var s []string` (nil slice) over `s := []string{}` when declaring empty slices, unless JSON serialization requires an explicit empty `[]` array rather than `null`.

---

## 4. Error Handling

### 4.1 Return Early & Un-indent the Happy Path
- Keep normal execution aligned on the left edge. Handle errors immediately with early return:
  ```go
  // Good
  f, err := os.Open(filename)
  if err != nil {
      return fmt.Errorf("open file %q: %w", filename, err)
  }
  defer f.Close()

  data, err := io.ReadAll(f)
  if err != nil {
      return fmt.Errorf("read file %q: %w", filename, err)
  }
  return process(data)
  ```

### 4.2 Error Strings
- Error messages must be **lowercase** and **not end with punctuation**:
  ```go
  // Good
  return fmt.Errorf("user %q not found", id)

  // Bad
  return fmt.Errorf("User %q not found!", id)
  ```

### 4.3 Wrapping Errors (`%w` vs `%v`)
- Use `%w` when callers are explicitly intended to match or unwrap the underlying error using `errors.Is` or `errors.As`.
- Use `%v` when concealing the underlying error implementation details or avoiding tight coupling.
- Wrap errors with context (what action was being performed):
  ```go
  if err := db.Ping(ctx); err != nil {
      return fmt.Errorf("connecting to database: %w", err)
  }
  ```

### 4.4 Do Not Panic
- **DO NOT** use `panic` for standard error flows. Return an `error`.
- `panic` is only acceptable:
  1. During program startup/initialization for fatal configuration bugs (e.g. `MustCompile` for static regex).
  2. For impossible internal invariants that represent immediate programmer bugs.

### 4.5 Sentinel Errors & Error Types
- Sentinel errors should be package-level variables named with prefix `Err`:
  ```go
  var ErrNotFound = errors.New("entity not found")
  ```
- Custom error structs should be named with suffix `Error`:
  ```go
  type ValidationError struct {
      Field  string
      Reason string
  }
  func (e *ValidationError) Error() string { ... }
  ```

---

## 5. Concurrency & Goroutines

### 5.1 Goroutine Lifetimes
- Never fire-and-forget a goroutine without knowing **how and when it will terminate**.
- Every goroutine must have an explicit stop mechanism (via `context.Context` cancellation or channel close).
- Do not create background goroutines inside library functions without caller control; functions should be **synchronous** by default.

### 5.2 Context Propagation
- `ctx context.Context` **must always be the first parameter** of functions executing I/O, network calls, or long computations:
  ```go
  func (s *Service) FetchUser(ctx context.Context, id string) (*User, error)
  ```
- **Never** store a `context.Context` in a struct field. Pass it through function arguments.
- Always check `ctx.Err()` in long-running loops.

### 5.3 Mutexes & Value Copying
- A struct containing `sync.Mutex` or `sync.RWMutex` **must only have pointer receivers** and must never be passed or copied by value:
  ```go
  type SafeCounter struct {
      mu    sync.Mutex
      count int
  }

  // Good: pointer receiver
  func (c *SafeCounter) Inc() {
      c.mu.Lock()
      defer c.mu.Unlock()
      c.count++
  }
  ```

---

## 6. Types, Structs & Interfaces

### 6.1 Interface Ownership
- **Define interfaces where they are consumed**, not where they are implemented.
- The consumer defines what it requires:
  ```go
  // Inside package consumer/service
  type UserFetcher interface {
      FetchUser(ctx context.Context, id string) (*User, error)
  }

  type Service struct {
      fetcher UserFetcher
  }
  ```
- **Accept interfaces, return concrete structs**:
  - Functions should return concrete types (e.g., `*Client`, `*Store`), allowing callers to decide how to mock or consume them.

### 6.2 Small Interfaces
- Keep interfaces focused. The best Go interfaces have only 1 or 2 methods (`io.Reader`, `io.Closer`, `fmt.Stringer`).

### 6.3 Pointer vs. Value Receivers
- **Use a pointer receiver if:**
  - The method modifies the receiver.
  - The receiver contains a `sync.Mutex` or cannot be copied.
  - The receiver is a large struct.
- **Use a value receiver if:**
  - The receiver is a small, immutable struct, a basic type, or a map/channel/func.
- When in doubt, prefer a pointer receiver for structs to maintain consistency across all methods.

---

## 7. Documentation & Comments

### 7.1 Doc Comments on Exported Declarations
- Every exported type, function, constant, and variable must have a doc comment.
- The doc comment must begin with the name of the symbol and form a complete grammatical sentence:
  ```go
  // Client manages connections and transactions with the remote service.
  type Client struct{}

  // Execute runs the provided query against the database cluster.
  func (c *Client) Execute(ctx context.Context, query string) (Result, error)
  ```

### 7.2 Package Comments
- Every package should have a package comment at the top of `doc.go` or the primary source file:
  ```go
  // Package auth provides token parsing, validation, and session verification.
  package auth
  ```

---

## 8. Testing Standards

### 8.1 Table-Driven Tests
- Prefer table-driven tests with `t.Run` for testing multiple scenarios:
  ```go
  func TestParse(t *testing.T) {
      tests := []struct {
          name    string
          input   string
          want    Result
          wantErr bool
      }{
          {
              name:    "valid input",
              input:   "user_123",
              want:    Result{ID: 123},
              wantErr: false,
          },
          {
              name:    "invalid empty input",
              input:   "",
              wantErr: true,
          },
      }

      for _, tc := range tests {
          t.Run(tc.name, func(t *testing.T) {
              got, err := Parse(tc.input)
              if (err != nil) != tc.wantErr {
                  t.Fatalf("Parse(%q) err = %v, wantErr = %v", tc.input, err, tc.wantErr)
              }
              if diff := cmp.Diff(tc.want, got); diff != "" {
                  t.Errorf("Parse(%q) mismatch (-want +got):\n%s", tc.input, diff)
              }
          })
      }
  }
  ```

### 8.2 Failure Reporting
- Format failure outputs clearly as **`got X, want Y`**:
  ```go
  t.Errorf("Calculate(%d) = %d, want %d", tc.input, got, tc.want)
  ```
- For complex structs, slices, or maps, use **`github.com/google/go-cmp/cmp`** to print clean diffs.

### 8.3 Test Helpers & Teardown
- Always call `t.Helper()` as the first line of any helper function:
  ```go
  func setupTestDB(t *testing.T) *sql.DB {
      t.Helper()
      db := connect()
      t.Cleanup(func() { db.Close() })
      return db
  }
  ```
- Use `t.Cleanup(fn)` instead of deferred cleanups to guarantee teardown runs even if subtests fail or exit early.
- **Never call `t.Fatal` inside a separate goroutine**; `t.Fatal` terminates only the calling goroutine, causing hangs or races.

---

## 9. AI Quick Reference Checklist

Before completing Go code generation, verify:

- [ ] All package names are single-word lowercase without underscores.
- [ ] No `Get` prefix on getter methods.
- [ ] Initialisms and acronyms (`ID`, `URL`, `HTTP`, `UUID`) are cased consistently.
- [ ] Errors are checked immediately; happy path is not indented inside `else` blocks.
- [ ] Error strings are lowercase and free of trailing punctuation.
- [ ] `context.Context` is the first argument on I/O or cancellable functions.
- [ ] No goroutines are started without clear shutdown conditions.
- [ ] Structs containing `sync.Mutex` are never passed or returned by value.
- [ ] Exported symbols have full-sentence doc comments starting with the symbol name.
- [ ] Tests use table-driven design, standard `testing.T`, `t.Helper()`, and `cmp.Diff`.
