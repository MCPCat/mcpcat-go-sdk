# Port plan: privacy-first SDK diagnostics (Go)

Port the TypeScript SDK's `feat/sdk-diagnostics-otlp` PR (MCPCat/mcpcat-typescript-sdk#40) to this
Go SDK. The feature mirrors internal diagnostic logs to MCPCat's own monitoring as OTLP/HTTP log
records so we can detect when a developer's SDK **fails to set up** — sending **only operational
metadata, never event payloads or user data**.

> The Go SDK's architecture is the furthest from TS/Python. **Read §1 before anything** — several
> TS changes are no-ops here, and a few things move to different layers.

---

## 0. Feature shape (what to reproduce)

1. The logger gains a **sink hook**: a registered callback receives every log entry + its level.
2. A new `internal/diagnostics` package registers that sink. Each entry becomes an OTLP `LogRecord`
   (metadata-only body + severity from the log level), buffered and flushed via batched,
   fire-and-forget HTTP POST to `https://otel.agentcat.com/v1/logs` with a shared bearer token.
3. Resource attributes carry identity + environment metadata: attempted `projectID` (or an
   anonymous `install_id` hash when none), SDK language/version, MCP-SDK version, Go runtime/OS/arch.
4. On by default; opt out via option or `DISABLE_DIAGNOSTICS` env var. Never panics into the host;
   never blocks.
5. Two `track()` "setup beacons" (start + complete) so a clean install still emits a heartbeat tied
   to its project id, and setup failures are localizable.

---

## 1. Go is different — read first

| Difference | Consequence |
|---|---|
| **No event-payload logging exists today.** The publisher's success log is `Debugf("...published event %s", event.GetId())` — id only. There is **no `Event details` / `model_dump_json` dump** like TS/Python. | The big "scrub the event dump" change is **N/A**. The *only* payload-bearing function is `internal/event/helpers.go: LogEvent`, which dumps Parameters/Response/UserIntent/Error/Actor name/`IdentifyData` field-by-field via `Infof` — and it currently has **no production callers** (test/dead code). Still convert it to metadata-only (§7), because once Info is teed it would leak. |
| **No telemetry exporters.** `Options.Exporters` is documented "TODO: implement in future"; there is no `exporters/datadog`. | The Datadog scrub is **N/A**. Skip it. |
| **Leveled logger, severity by method.** `internal/logging/log.go` exposes `Info/Warn/Error/Debug` (+`f`). Severity is explicit, not inferred from text. | The sink receives a **level**, mapped directly to OTLP severity — no `fail|error` regex. Tee **Info/Warn/Error** (skip `Debug` — it's high-volume per-event). The TS "classify `Warning: Failed…` as ERROR" hack is unnecessary: just log setup failures with `Errorf`. |
| **Logger is debug-gated** (writes to `io.Discard` when `Debug` is false; only opens `~/mcpcat.log` in debug). | The sink is separate from the file writer, so tee fires **regardless of debug** — diagnostics must be sent even when the developer hasn't enabled debug. Call the sink in `Info/Warn/Error` independent of the file output. |
| **Multi-module (go.work).** Root module `github.com/mcpcat/mcpcat-go-sdk` holds `internal/` + the `mcpcat` facade (`mcpcat.go`). `officialsdk/` and `mcpgo/` are **separate modules** that import only the public `mcpcat` package — they **cannot import `internal/`**. | Expose diagnostics through the **`mcpcat` facade** (like `InitPublisher`, `SetDebug`, `Shutdown`). Both integration `Track`s call facade funcs. |
| **Two `Track` functions:** `officialsdk/officialsdk.go:65` and `mcpgo/mcpgo.go:67`, each with its **own `Options` struct** mapped into `mcpcat.Options`. | Add `DisableDiagnostics` to **three** structs (`core.Options`, `officialsdk.Options`, `mcpgo.Options`) and both `coreOpts` mappings; init diagnostics + beacons in **both** `Track`s. |
| **No debug/diagnostics env var pattern exists.** Go only has `Options.Debug` / `SetGlobalDebug`. | Write `DISABLE_DIAGNOSTICS` parsing fresh, **value-interpreting** (true/1/yes/on disable; false/0/no/off/empty stay enabled) — the same correctness point Kashish raised on the TS PR. Do not use bare `os.Getenv(...) != ""`. |
| **Concurrency is real** (goroutines, the logger holds `l.mu`). | The sink call must not block under `l.mu`. Capture the formatted string, then call the sink (which only appends to a buffer under its own mutex). The diagnostics package owns its buffer/timer/HTTP and flushes off-thread. |

Net: the Go port is **mostly additive** (diagnostics plumbing), with a single small scrub (`LogEvent`).

---

## 2. New package: `internal/diagnostics/diagnostics.go`

Port of `diagnostics.ts`, idiomatic Go. Package-level state guarded by a `sync.Mutex`; never panics.

**Exported (used by the `mcpcat` facade):**
- `Init(projectID string, disabled bool, integration, mcpSDKPath string)` — idempotent (sync.Once or
  an `initialized` bool). `enabled = !disabled && !envDisabled()`. If enabled, build static
  attributes and register the sink via `logging.SetDiagnosticsSink(capture)`.
- `Flush()` — swap the buffer under lock, marshal OTLP JSON, POST best-effort; never returns an error
  that escapes.
- `Enabled() bool` — for tests.
- `ResetForTest()`, `BuildRecordForTest(level, msg)`, `StaticAttributesForTest()` — mirror the TS
  test helpers.

**Internal:**
- `capture(level logging.Level, msg string)` — bounded append (`maxBuffer = 1000`, drop-oldest) +
  schedule flush.
- `envDisabled() bool` — interpret `DISABLE_DIAGNOSTICS`: `strings.ToLower(strings.TrimSpace(v))`;
  disabled iff value ∈ {true,1,yes,on}; {"",false,0,no,off} ⇒ not disabled.
- `resolveEndpoint()` — `DIAGNOSTICS_ENDPOINT` env or `DefaultDiagnosticsEndpoint`; ensure suffix
  `/v1/logs`.
- `resolveToken()` — `DIAGNOSTICS_TOKEN` env or `DefaultDiagnosticsToken`.
- `severityFor(level)` — Error→(17,"ERROR"), Warn→(13,"WARN"), Info→(9,"INFO") (Debug not teed).
- `buildRecord(level, msg)` — `{timeUnixNano: strconv.FormatInt(time.Now().UnixNano(),10),
  severityNumber, severityText, body:{stringValue: msg}, attributes: []}`.
- `computeInstallID()` — `h := sha256.Sum256([]byte(hostname + "|" + exePath));
  hex.EncodeToString(h[:])[:16]` (best-effort; `os.Hostname()`, `os.Executable()`).
- `buildStaticAttributes(projectID, integration, mcpSDKPath)` — list of `{key, value:{stringValue}}`:
  - `mcpcat.project_id` **or** `mcpcat.install_id` (the hash) when empty
  - `mcpcat.sdk.language="go"`, `mcpcat.sdk.version=session.GetDependencyVersion("github.com/mcpcat/mcpcat-go-sdk")`
  - `mcpcat.mcp_sdk.version=session.GetDependencyVersion(mcpSDKPath)` (path passed by the integration:
    official = `github.com/modelcontextprotocol/go-sdk`, mcpgo = `github.com/mark3labs/mcp-go` — confirm
    the exact module paths)
  - `mcpcat.integration=integration` ("officialsdk" | "mcpgo")
  - `process.runtime.name="go"`, `process.runtime.version=runtime.Version()`, `process.pid=strconv.Itoa(os.Getpid())`
  - `os.type=runtime.GOOS`, `host.arch=runtime.GOARCH`, `host.cpu.count=strconv.Itoa(runtime.NumCPU())`
  - `deployment.environment=os.Getenv("APP_ENV")` (or `ENVIRONMENT`) — omit if empty
- Flush scheduling: `batchFlush = 2 * time.Second`. On first buffered record, start a single
  `time.AfterFunc(batchFlush, Flush)` (guarded so only one is pending). Goroutines don't block exit,
  so no `.unref()` analog is needed — but **`Flush()` must be called on shutdown** (§8) so the last
  batch isn't lost.
- HTTP: a package `*http.Client{Timeout: 5 * time.Second}`. `Flush` does
  `client.Post(url, "application/json", body)` with header `Authorization: Bearer <token>` (build the
  request with `http.NewRequest` to set the header), all inside a `defer recover()` / error-swallow.

**OTLP payload** (identical shape to TS): `{"resourceLogs":[{"resource":{"attributes":...},
"scopeLogs":[{"scope":{"name":DiagnosticsScopeName,"version":sdkVersion},"logRecords":records}]}]}`.

---

## 3. `internal/logging/log.go` — sink hook (independent of debug)

Add a package-level sink and a `Level` type, and call the sink from each leveled method.

```go
type Level int
const ( LevelInfo Level = iota; LevelWarn; LevelError; LevelDebug )

var (
    diagSink   func(Level, string)
    diagSinkMu sync.RWMutex
)

func SetDiagnosticsSink(fn func(Level, string)) {
    diagSinkMu.Lock(); defer diagSinkMu.Unlock(); diagSink = fn
}

func (l *Logger) emit(level Level, prefix, msg string) {
    // Tee to diagnostics first — independent of l.debug. Must never break logging.
    diagSinkMu.RLock(); sink := diagSink; diagSinkMu.RUnlock()
    if sink != nil {
        func() { defer func() { _ = recover() }(); sink(level, msg) }()
    }
    l.mu.Lock(); defer l.mu.Unlock()
    l.logger.Printf("%s: %s", prefix, msg)
}
```
Refactor `Info/Warn/Error/Debug` to call `emit(LevelX, "INFO"/"WARN"/"ERROR"/"DEBUG", msg)`. Call the
sink **outside `l.mu`** (as above) so a slow sink can't stall logging. Debug may call `emit` too but
the diagnostics `capture` ignores `LevelDebug`.

---

## 4. Constants

Add (a new `internal/diagnostics/constants.go`, or extend an existing constants file):
```go
const (
    DiagnosticsScopeName      = "mcpcat-diagnostics"
    DefaultDiagnosticsEndpoint = "https://otel.agentcat.com"
    // Public shared ingestion key — NOT a secret; ships in the binary to deter drive-by
    // traffic, paired with a server-side rate limit. Override with DIAGNOSTICS_TOKEN.
    // Must match the collector's bearer token.
    DefaultDiagnosticsToken = "dgk_sdk_diag_3f9a2c7e1b8d4065af2e9c1d7b6a4f80"
)
```
> The token literal must match the collector exactly (same value as the other SDKs). The collector
> runs at `otel.agentcat.com` and validates it via WAF.

---

## 5. Options — add `DisableDiagnostics` in THREE places

- `internal/core/types.go` `Options`: `DisableDiagnostics bool` (doc: disables internal SDK
  diagnostics; on by default; also via `DISABLE_DIAGNOSTICS` env; `~/mcpcat.log` unaffected).
- `officialsdk/officialsdk.go` `Options`: same field.
- `mcpgo/mcpgo.go` `Options`: same field.
- Map it through in **both** `Track` `coreOpts` builders (`DisableDiagnostics: opts.DisableDiagnostics`).

---

## 6. `Track` wiring (both integrations) + facade

**Facade (`mcpcat.go`)** — expose internal diagnostics to the integration modules:
```go
func InitDiagnostics(projectID string, disabled bool, integration, mcpSDKPath string) {
    diagnostics.Init(projectID, disabled, integration, mcpSDKPath)
    logging.New().Infof("MCPCat setup started | project %s | integration %s",
        orTelemetryOnly(projectID), integration)               // start beacon
}
func LogSetupComplete(projectID string, opts *Options) {       // complete beacon
    logging.New().Infof(
        "MCPCat setup complete | project %s | tracing=%t context=%t report_missing=%t",
        orTelemetryOnly(projectID), !opts.DisableTracingX, !opts.DisableToolCallContext,
        !opts.DisableReportMissing)
}
```
(Adjust flag names to the actual `core.Options`; Go uses negative flags — there is no
`enableTracing`, so phrase as `report_missing=%t` from `!DisableReportMissing`, etc. Also flush
diagnostics from `Shutdown`: see §8.)

**In each `Track`** (`officialsdk` and `mcpgo`), after `coreOpts` is built and the server registered:
```go
mcpcat.InitDiagnostics(projectID, coreOpts.DisableDiagnostics, "officialsdk",
    "github.com/modelcontextprotocol/go-sdk")
// ... existing setup (middleware, get_more_tools) ...
mcpcat.LogSetupComplete(projectID, coreOpts)   // just before returning shutdownFn
```
Both beacons are INFO (won't trip the ERROR→Slack alarm). **Recommended add:** log the early
validation failures (`ErrNilServer`, `ErrEmptyProjectID`) with `Errorf` before returning, so genuine
setup failures show up in diagnostics as ERROR (today they're only returned to the caller). Note
`InitDiagnostics` runs after those guards, so a nil-server failure won't emit — that's acceptable
(nothing to attribute yet); the project-id-empty case can be logged right before returning.

---

## 7. Scrub `internal/event/helpers.go: LogEvent` → metadata only

It currently dumps values. Convert the payload-bearing lines to counts/presence (keep IDs, types,
durations, session/client/server names, severity flag):
- **Parameters:** replace the per-key value loop with `logger.Infof("  Parameters: %d field(s)", len(evt.Parameters))`.
- **Response:** `logger.Infof("  Response: %d field(s)", len(evt.Response))` (drop the value printing).
- **User Intent:** `logger.Infof("  User Intent: %d chars", len(*evt.UserIntent))` (don't print the text).
- **Error Details:** drop `%v` of `evt.Error`; log only `Is Error: true` (or the error *type*).
- **Identity:** keep `Actor ID` (an identifier), drop `Actor Name` value and the `IdentifyData`
  key/value loop → `logger.Infof("    Additional Data: %d field(s)", len(evt.IdentifyData))`.

> Leave `*-level error interpolations elsewhere (`Errorf("...: %v", err)`) alone — error text is
> high-value for diagnostics and isn't a full payload dump (same scoping as the TS PR). The publisher
> success log (`Debugf` with id only) is already clean and isn't teed anyway.

---

## 8. Shutdown flush

`mcpcat.Shutdown(ctx)` is called by **both** integration `shutdownFn`s. Add a `diagnostics.Flush()`
call there (after `publisher.ShutdownGlobal`). That guarantees the last buffered batch is sent on
graceful shutdown.

---

## 9. README

Add an **Internal diagnostics** section, privacy-forward (mirror the TS README's final wording): lead
with what we *never* collect (tool calls, responses, anything about users); records carry only
operational metadata (project id or anonymous install id); `~/mcpcat.log` unchanged; then opt-out
(`Options{DisableDiagnostics: true}` or the `DISABLE_DIAGNOSTICS` env var). Do **not** document the
shared key or endpoint/token overrides in the README — keep those in doc comments.

---

## 10. Tests (`testing`, `httptest`)

Mirror the TS suites. Root-module tests live under `internal/...`; integration beacon tests live in
each module.

| TS test | Go test | Asserts |
|---|---|---|
| `diagnostics-sink` | `internal/logging/log_test.go` (extend) | `SetDiagnosticsSink` receives every Info/Warn/Error with the right `Level`; a panicking sink never breaks logging; **sink fires with `Debug=false`** (the key invariant). |
| `diagnostics-optout` | `internal/diagnostics/optout_test.go` | enabled by default; disabled via `Init(..., disabled=true, ...)`; `DISABLE_DIAGNOSTICS` ∈ {true,1,yes,on} disables; ∈ {false,0,no,off,"  "} stays enabled (the Kashish case). Use `t.Setenv`. |
| `diagnostics-record` | `internal/diagnostics/record_test.go` | `BuildRecordForTest`: Error⇒17/ERROR, Warn⇒13/WARN, Info⇒9/INFO; body == msg. |
| `diagnostics-attributes` | `internal/diagnostics/attributes_test.go` | with projectID → `mcpcat.project_id`, no install_id; without → anonymous `install_id`; `mcpcat.sdk.language=="go"`, GOOS/GOARCH present. |
| `diagnostics-export` + `-auth` | `internal/diagnostics/export_test.go` | point `DIAGNOSTICS_ENDPOINT` at an `httptest.NewServer`; capture a log → `Flush()` → assert OTLP-shaped JSON body **and** `Authorization: Bearer dgk_sdk_diag_…`; `DIAGNOSTICS_TOKEN` overrides. |
| `diagnostics-no-payload` | `internal/event/helpers_test.go` (extend) | drive `LogEvent` with a sensitive value in Parameters/Response/UserIntent/IdentifyData through a capturing sink; assert no captured entry contains the value, only counts/IDs. |
| setup beacons | `officialsdk/..._test.go` & `mcpgo/..._test.go` | a capturing sink + `Track(...)` emits `MCPCat setup started` and `MCPCat setup complete` with the project id, metadata-only. |

Reset between tests with `diagnostics.ResetForTest()` + `logging.ResetForTesting()` +
`logging.SetDiagnosticsSink(nil)`.

---

## 11. Verify

```bash
make fmt && make vet && make test          # go test -race ./... in all modules
cd officialsdk && go test -race ./... && cd ..
cd mcpgo && go test -race ./... && cd ..
gofmt -l .                                  # must be empty
```
Optional smoke: `DIAGNOSTICS_ENDPOINT` → local `httptest`, run a `Track`'d server with `Debug=false`,
assert a record is POSTed (proves the not-gated-by-debug invariant end to end).

---

## 12. File-by-file checklist

- [ ] `internal/diagnostics/diagnostics.go` (+`constants.go`) — **new** (§2, §4)
- [ ] `internal/logging/log.go` — `Level`, `SetDiagnosticsSink`, `emit()` teeing outside `l.mu`/debug (§3)
- [ ] `internal/core/types.go` — `Options.DisableDiagnostics` (§5)
- [ ] `mcpcat.go` — facade `InitDiagnostics` / `LogSetupComplete`; `Shutdown` flushes diagnostics (§6, §8)
- [ ] `officialsdk/officialsdk.go` — `Options.DisableDiagnostics`, map it, init + beacons in `Track` (§5, §6)
- [ ] `mcpgo/mcpgo.go` — same as officialsdk (§5, §6)
- [ ] `internal/event/helpers.go` — `LogEvent` metadata-only (§7)
- [ ] `README.md` — privacy-forward section (§9)
- [ ] tests per §10

### N/A vs the TS PR (don't go looking for these)
- No `Event details`/`model_dump_json` dump to remove (publisher logs id only).
- No Datadog/exporter scrub (exporters unimplemented).
- No text-based severity inference (levels are explicit).

### Companion (already done, do not rebuild)
Server-side collector + WAF token gate live in `MCPCat/mcpcat-server` (PR #343, merged/approved) at
`otel.agentcat.com`. The bearer-token literal in §4 must match it. Nothing to deploy from here.
