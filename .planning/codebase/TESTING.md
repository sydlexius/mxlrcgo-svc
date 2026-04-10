# Testing Patterns

**Analysis Date:** 2026-04-10

## Test Framework

**Runner:**
- Go's built-in `testing` package (no third-party test framework)
- No separate config file -- Go's test infrastructure requires none

**Assertion Library:**
- None -- uses manual comparison with `if got != want` pattern
- No testify, gomega, or other assertion libraries

**Run Commands:**
```bash
make test              # Run all tests (verbose, race detector, no caching)
make test-cover        # Tests + HTML coverage report
go test ./...          # Quick run (all packages)
go test -run TestFoo   # Run a single test by name
go test -v -race -count=1 ./...  # Exact CI command
```

## Test File Organization

**Location:**
- Co-located with source files in the project root (same `package main`)
- Test file sits alongside the file it tests

**Naming:**
- Standard Go convention: `{source}_test.go`
- Currently only `utils_test.go` exists (tests for `utils.go`)

**Structure:**
```
/
├── utils.go           # Source
├── utils_test.go      # Tests
├── main.go            # No test file
├── lyrics.go          # No test file
├── musixmatch.go      # No test file
└── structs.go         # No test file
```

## Test Structure

**Suite Organization:**
```go
func TestSlugify(t *testing.T) {
    tests := []struct {
        input string
        want  string
    }{
        {"\\/:*?\"<>|", ""},
        {"-Hello_", "Hello"},
        {"Hello ---- World", "Hello - World"},
    }

    for i, tc := range tests {
        t.Run(fmt.Sprintf("slugify=%d", i), func(t *testing.T) {
            got := slugify(tc.input)
            if got != tc.want {
                t.Fatalf("got %v; want %v", got, tc.want)
            } else {
                t.Logf("success")
            }
        })
    }
}
```

**Patterns:**
- **Table-driven tests** with anonymous struct slices (`[]struct{ input, want }`)
- **Subtests** via `t.Run()` with descriptive names
- Subtest names use `fmt.Sprintf("funcname=%d", i)` with numeric index
- `t.Fatalf("got %v; want %v", got, tc.want)` for assertion failures
- `t.Logf("success")` for passing tests (verbose output)

**Follow these conventions when writing new tests:**
1. Use table-driven tests with `[]struct` for multiple cases
2. Use `t.Run()` subtests for each case
3. Name subtests as `fmt.Sprintf("functionName=%d", i)` or use a descriptive `name` field
4. Assert with `t.Fatalf("got %v; want %v", got, tc.want)` -- no third-party assertion library
5. Do not use `t.Logf("success")` in new tests (unnecessary noise)

## Mocking

**Framework:** None

**Patterns:**
- No mocking infrastructure exists
- No interfaces defined for dependency injection
- The `Musixmatch` struct makes real HTTP calls with no abstraction layer
- File I/O operations use `os` directly with no wrapping

**What would need mocking for new tests:**
- HTTP client in `musixmatch.go` -- extract `http.Client` as interface or use `httptest.Server`
- File system operations in `lyrics.go` and `utils.go` -- use `os.MkdirTemp()` or `testing/fstest`
- The `tag.ReadFrom()` call in `getSongDir()` -- needs audio file fixtures or interface extraction

**What NOT to mock:**
- Pure functions like `slugify()`, `assertInput()`, `isInArray()` -- test directly
- `InputsQueue` methods -- test the real implementation

## Fixtures and Factories

**Test Data:**
```go
// Inline struct literals in test tables
tests := []struct {
    input string
    want  string
}{
    {"\\/:*?\"<>|", ""},
    {"-Hello_", "Hello"},
}
```

**Location:**
- No fixture files exist
- Test data is defined inline within test functions
- No test helper functions or shared test utilities
- No `testdata/` directory

## Coverage

**Requirements:** None enforced (no minimum coverage threshold)

**Current State:** Only `slugify()` in `utils.go` has test coverage. All other functions and files are untested.

**View Coverage:**
```bash
make test-cover                              # Generates coverage.out and coverage.html
go tool cover -html=coverage.out -o coverage.html  # Manual HTML report
go tool cover -func=coverage.out             # Per-function coverage summary
```

**Coverage artifacts:**
- `coverage.out` -- raw coverage profile (gitignored)
- `coverage.html` -- HTML report (gitignored)

## Test Types

**Unit Tests:**
- Only pure function tests exist (`TestSlugify`)
- Test the function in isolation with known inputs and expected outputs
- No setup/teardown needed

**Integration Tests:**
- None exist
- CLAUDE.md prescribes: use real SQLite with `file::memory:?cache=shared` for database tests (when database features are added)
- No HTTP integration tests for the Musixmatch API

**E2E Tests:**
- Not used

## CI Test Execution

**Pipeline:** `.github/workflows/ci.yml`
- Tests run on `ubuntu-latest` with Go version from `go.mod` (1.22)
- Command: `go test -v -race -count=1 ./...`
- Flags: `-v` (verbose), `-race` (race detector), `-count=1` (disable test caching)
- Tests run in parallel with lint (both depend on change detection, not each other)
- Build matrix only runs after both lint and test pass

**Race Detector:**
- Enabled in both `make test` and CI via `-race` flag
- Relevant because of package-level mutable state (`inputs`, `failed` in `main.go`)

## Common Patterns

**Table-Driven Test Template:**
```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name  string
        input TypeA
        want  TypeB
    }{
        {"description of case", inputVal, expectedVal},
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            got := functionName(tc.input)
            if got != tc.want {
                t.Fatalf("got %v; want %v", got, tc.want)
            }
        })
    }
}
```

**Error Testing (recommended pattern for this codebase):**
```go
func TestFunctionReturnsError(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        wantErr bool
    }{
        {"valid input", "good", false},
        {"invalid input", "bad", true},
    }

    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            _, err := functionName(tc.input)
            if (err != nil) != tc.wantErr {
                t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
            }
        })
    }
}
```

**Temp Directory for File Tests (recommended):**
```go
func TestWriteFunction(t *testing.T) {
    dir := t.TempDir() // auto-cleaned up
    // ... write files to dir, assert contents
}
```

## Linter Exclusions for Tests

From `.golangci.yml`:
```yaml
exclusions:
  rules:
    - path: _test\.go
      linters:
        - gosec     # Security checks relaxed in tests
        - errcheck  # Unchecked errors OK in tests
        - noctx     # Context-less HTTP OK in tests
```

---

*Testing analysis: 2026-04-10*
