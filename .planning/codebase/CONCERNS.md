# Codebase Concerns

**Analysis Date:** 2025-04-10

## Tech Debt

**Global mutable state:**
- Issue: `inputs` and `failed` are package-level `InputsQueue` variables in `main.go` (lines 15-16). The `closeHandler` goroutine accesses both without synchronization, and the main loop mutates them concurrently. This makes the code untestable (cannot run `main()` in tests without side effects) and creates a data race between the signal handler goroutine and the main loop.
- Files: `main.go:15-16`, `main.go:43-60`, `main.go:117-126`
- Impact: Data race on SIGTERM during active processing. Cannot unit-test the main orchestration loop. Prevents any future parallelization.
- Fix approach: Move `inputs` and `failed` into a struct passed through the call chain, or at minimum protect access with a `sync.Mutex`. Extract the orchestration loop into a testable function that accepts dependencies.

**Reflection-based `isInArray` helper:**
- Issue: `isInArray` in `utils.go:150-161` uses `reflect.ValueOf` for a simple array membership check. This is unnecessary given Go 1.22 (which supports generics via 1.21+), slower than a direct loop or `slices.Contains`, and calls `log.Fatal` on invalid input rather than returning an error.
- Files: `utils.go:150-161`
- Impact: Runtime panic via `log.Fatal` on bad input type. Performance overhead from reflection. Harder to read than idiomatic Go.
- Fix approach: Replace with `slices.Contains` from the standard library (available since Go 1.21). The `supportedFType()` function at `utils.go:18` should return a slice (`[]string`) instead of a fixed-size array (`[8]string`) to work with `slices.Contains`.

**Hardcoded fallback API token:**
- Issue: A Musixmatch token is hardcoded at `main.go:37` as a fallback when `--token` is not provided. This token is committed to source and publicly visible.
- Files: `main.go:36-38`
- Impact: Token can be revoked at any time, breaking the default experience. If the token has rate limits or billing, anyone can use it. Potential API abuse vector.
- Fix approach: Remove the hardcoded token. Require `--token` as a required flag, or read from an environment variable (`MXLRC_TOKEN`). Document how to obtain a token.

**snake_case variable names:**
- Issue: Several functions in `utils.go` use Python-style `snake_case` naming (`song_list`, `save_path`, `text_fn`, `lrc_file`), violating Go conventions (`camelCase`). This is a carry-over from the Python port.
- Files: `utils.go:34`, `utils.go:45`, `utils.go:52`, `utils.go:88`
- Impact: Linter warnings (revive may flag these). Inconsistent with the rest of the codebase and Go ecosystem conventions.
- Fix approach: Rename to `songList`, `savePath`, `textFn`, `lrcFile` etc. Straightforward find-and-replace within `utils.go`.

**Regex compiled on every call:**
- Issue: `slugify` in `utils.go:141-148` compiles two regexes (`re1`, `re2`) on every invocation. These are constant patterns.
- Files: `utils.go:141-148`
- Impact: Minor performance overhead on bulk operations (directory mode with thousands of files). Unnecessary allocations.
- Fix approach: Hoist `re1` and `re2` to package-level `var` using `regexp.MustCompile`.

**Fixed-size array for supported types:**
- Issue: `supportedFType()` returns `[8]string` -- a fixed-size array. Adding a new format requires changing the array size, the function, and anything that consumes it.
- Files: `utils.go:18-20`
- Impact: Brittle to extend. Incompatible with standard library slice functions like `slices.Contains`.
- Fix approach: Change to `[]string` (slice). Consider making it a package-level `var` or `const`-like pattern.

## Known Bugs

**`next()` and `pop()` panic on empty queue:**
- Symptoms: Runtime panic (index out of range) if `next()` or `pop()` is called on an empty `InputsQueue`.
- Files: `structs.go:59-67`
- Trigger: If `inputs` is somehow empty when the main loop starts, or if signal handler races with the main loop draining the queue.
- Workaround: The main loop checks `!inputs.empty()` before calling `next()`/`pop()`, but the signal handler in `closeHandler` calls `failedHandler` which accesses `inputs.Queue` directly (`main.go:80`) without checking emptiness first.

**`failedHandler` called from goroutine without synchronization:**
- Symptoms: Potential data race. The signal handler goroutine (`closeHandler`) calls `failedHandler` which reads and mutates both `inputs` and `failed` queues while the main goroutine may be actively modifying them.
- Files: `main.go:117-126`, `main.go:77-114`
- Trigger: Send SIGTERM/SIGINT while the main loop is between `inputs.next()` and `inputs.pop()`.
- Workaround: None. The race condition exists but is unlikely to cause visible corruption in practice due to the sequential nature of the main loop with sleep intervals.

**HTTP 401 mapped to "too many requests":**
- Symptoms: Misleading error message. HTTP 401 (Unauthorized) is reported as "too many requests" at `musixmatch.go:68`. The Musixmatch API may return 401 for rate limiting, but it conflates authentication failures with rate limit errors.
- Files: `musixmatch.go:66-69`
- Trigger: Providing an invalid token, or when the API rejects the token for non-rate-limit reasons.
- Workaround: None. User must guess whether to wait or fix their token.

## Security Considerations

**Hardcoded API token in source:**
- Risk: The fallback token at `main.go:37` is committed to the public repository. Anyone can extract and use it.
- Files: `main.go:36-38`
- Current mitigation: Token is for a free-tier desktop API endpoint; limited blast radius.
- Recommendations: Remove hardcoded token. Support `MXLRC_TOKEN` environment variable. Add `.env` to `.gitignore` and document token setup.

**No TLS certificate verification override (good):**
- The HTTP client in `musixmatch.go:48` uses the default `http.Client` transport, which verifies TLS certificates. No `InsecureSkipVerify` detected. This is correct.

**Path traversal in directory mode:**
- Risk: `getSongDir` recursively walks user-specified directories and constructs file paths from directory entries. If a filesystem contains symlinks pointing outside the intended directory, the tool follows them.
- Files: `utils.go:63-119`
- Current mitigation: `os.ReadDir` does not follow symlinks by default for listing, but `os.Open` on symlinked files does follow them.
- Recommendations: Low risk for a CLI tool run by the user on their own filesystem. Consider adding `--no-follow-symlinks` if security is a concern.

**Unbounded recursion depth default:**
- Risk: Default `--depth` is 100 (`structs.go:49`). A deeply nested or circular symlink structure could cause stack overflow or excessive resource consumption.
- Files: `structs.go:49`, `utils.go:63-119`
- Current mitigation: Go stack grows dynamically, so stack overflow is unlikely. The depth limit prevents true infinite recursion.
- Recommendations: The default of 100 is reasonable. Consider detecting and skipping symlink cycles if robustness matters.

## Performance Bottlenecks

**Sequential API requests with fixed cooldown:**
- Problem: The main loop processes songs one at a time with a fixed `--cooldown` (default 15 seconds) sleep between each request. For a directory with 1000 songs, this means at minimum ~4 hours of cooldown time alone.
- Files: `main.go:43-60`, `main.go:66-75`
- Cause: Single-threaded design plus mandatory sleep after every request.
- Improvement path: Allow configurable concurrency (e.g., `--workers N`) with per-worker rate limiting. Use a semaphore or worker pool pattern. This would require resolving the global mutable state issue first.

**Reflection in hot path:**
- Problem: `isInArray` uses `reflect` for every file extension check during directory scanning.
- Files: `utils.go:150-161`, `utils.go:94`
- Cause: Generic-before-generics pattern carried over from the Python port.
- Improvement path: Replace with `slices.Contains`. Negligible real-world impact but unnecessary overhead.

**New HTTP client per API call:**
- Problem: `findLyrics` creates a new `http.Client` on every invocation (`musixmatch.go:48`). While Go's `http.Client` reuses connections via the default transport, creating a new client per call prevents sharing timeout configuration and adds minor overhead.
- Files: `musixmatch.go:48`
- Cause: Stateless function design.
- Improvement path: Store the `http.Client` as a field on the `Musixmatch` struct, initialized once.

## Fragile Areas

**InputsQueue data structure:**
- Files: `structs.go:55-79`
- Why fragile: `pop()` uses slice reslicing (`q.Queue = q.Queue[1:]`), which does not release memory for popped elements. For large queues (thousands of songs), this causes memory to remain allocated for the entire run. Also, `next()` and `pop()` panic on empty queues with no guard.
- Safe modification: Always check `empty()` before calling `next()` or `pop()`. If extending, consider using a proper queue implementation or at minimum add bounds checking.
- Test coverage: No tests for `InputsQueue` methods.

**Musixmatch API response parsing:**
- Files: `musixmatch.go:86-139`
- Why fragile: The response is parsed with `fastjson` using deeply nested path lookups (e.g., `"message", "body", "macro_calls", "matcher.track.get", "message"`). Any change to the Musixmatch API response structure silently breaks parsing -- `fastjson.Get` returns `nil` for missing paths rather than erroring. Some paths are checked for `nil` (lines 103, 128) but others are not (lines 97-98).
- Safe modification: Add nil checks on `tlg` and `tsg` before using them. Consider adding integration tests against recorded API responses (golden files).
- Test coverage: No tests for `findLyrics` or any API response parsing.

**Signal handler interaction with main loop:**
- Files: `main.go:117-126`, `main.go:43-60`
- Why fragile: The `closeHandler` goroutine calls `failedHandler` which directly mutates `inputs` and `failed` queues without locking. The main goroutine is concurrently reading/writing these same queues. Go's race detector would flag this.
- Safe modification: Do not add more shared state. If modifying signal handling, use a context with cancellation instead of direct goroutine access to shared state.
- Test coverage: No tests for signal handling.

## Scaling Limits

**In-memory queue for all inputs:**
- Current capacity: All song inputs are loaded into memory at startup via `InputsQueue`.
- Limit: For extremely large music libraries (100k+ files), all `Inputs` structs are held in memory simultaneously. Each `Inputs` struct is small (~200 bytes), so 100k entries is ~20MB -- unlikely to be a practical issue.
- Scaling path: Stream directory entries lazily instead of pre-scanning. Only relevant if combined with parallel workers.

**Single-threaded processing:**
- Current capacity: One song per ~16 seconds (1s API call + 15s cooldown).
- Limit: ~225 songs/hour, ~5400 songs/day.
- Scaling path: Worker pool with shared rate limiter. Requires architectural changes to eliminate global state.

## Dependencies at Risk

**Outdated dependencies:**
- Risk: All dependencies are pinned to older versions. `golang.org/x/text v0.3.8` is several major versions behind (current is v0.14+). `go-arg v1.4.3` is behind v1.5+. `dhowden/tag` is pinned to a 2022 commit hash.
- Impact: Missing security patches, bug fixes, and new features. `govulncheck` in CI should catch known vulnerabilities.
- Files: `go.mod`
- Migration plan: Run `go get -u ./...` and `go mod tidy` to update. Test with `make test` afterward. Dependabot is configured to propose weekly updates.

**`dhowden/tag` maintenance status:**
- Risk: Pinned to a 2022 commit (`v0.0.0-20220618230019-adf36e896086`). The library has no semver releases, only commit-based pseudo-versions.
- Impact: If the library becomes unmaintained, bugs in audio metadata reading (ID3, MP4, FLAC) will not be fixed.
- Migration plan: Monitor for alternatives. `go-taglib` or `go-mp3` could replace parts of the functionality, but `dhowden/tag` is the most complete pure-Go option.

**Go version 1.22:**
- Risk: `go.mod` specifies Go 1.22, which is current but will eventually leave the supported window.
- Impact: Low risk currently. Go 1.22 is supported.
- Migration plan: Update `go.mod` when new Go versions are released. CI tests on the specified version.

## Missing Critical Features

**No retry logic:**
- Problem: Failed API requests are recorded but never retried within the same run. The user must manually re-run with the `_failed.txt` file.
- Blocks: Robust batch processing. Network hiccups cause permanent failures in that run.

**No progress indicator:**
- Problem: For large directories, there is no indication of progress beyond individual log lines. No "X of Y complete" counter.
- Blocks: User awareness of remaining work during long runs.

**No `--dry-run` mode:**
- Problem: No way to preview what the tool will do without actually fetching lyrics and writing files.
- Blocks: Safe testing on large music libraries.

## Test Coverage Gaps

**Only `slugify` is tested:**
- What's not tested: Everything except `slugify`. No tests for `parseInput`, `assertInput`, `getSongMulti`, `getSongText`, `getSongDir`, `isInArray`, `InputsQueue` methods, `findLyrics`, `writeLRC`, `writeSyncedLRC`, `writeUnsyncedLRC`, `writeInstrumentalLRC`, `timer`, `failedHandler`, or `closeHandler`.
- Files: `utils_test.go` (28 lines, 1 test function)
- Risk: Any refactoring could silently break core functionality. The API response parsing in `musixmatch.go` is particularly risky -- a Musixmatch API change would go undetected until runtime.
- Priority: High. The most impactful tests to add:
  1. `InputsQueue` methods (unit tests, trivial to write)
  2. `assertInput` / `parseInput` (unit tests, no I/O needed for CLI mode)
  3. `findLyrics` with recorded HTTP responses (integration test with `httptest.Server`)
  4. `writeLRC` / `writeSyncedLRC` / `writeUnsyncedLRC` (unit tests writing to temp files)

---

*Concerns audit: 2025-04-10*
