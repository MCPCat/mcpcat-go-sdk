# Privacy-First SDK Diagnostics (OTLP) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Mirror the SDK's internal diagnostic logs to MCPCat's monitoring as OTLP/HTTP log records — operational metadata only, never event payloads or user data — so we can detect when a developer's SDK fails to set up.

**Architecture:** A package-level sink hook in `internal/logging` tees every Info/Warn/Error line (level-tagged, independent of debug) to a new `internal/diagnostics` package, which buffers each entry as an OTLP `LogRecord` and flushes batched, fire-and-forget HTTP POSTs to `https://otel.agentcat.com/v1/logs` with a shared bearer token. The root `mcpcat` facade exposes `InitDiagnostics`/`LogSetupComplete`/`LogSetupFailed`; both integration `Track`s call them. On by default; opt out via `Options.DisableDiagnostics` or the `DISABLE_DIAGNOSTICS` env var.

**Tech Stack:** Go 1.24, stdlib only (`crypto/sha256`, `encoding/hex`, `encoding/json`, `net/http`, `runtime`, `runtime/debug`, `os`, `sync`, `time`), `httptest` for tests. Go workspace (`go.work`) with three modules: root `github.com/mcpcat/mcpcat-go-sdk`, `mcpgo/`, `officialsdk/`.

## Global Constraints

- Diagnostics **never panic into the host and never block** the caller. Every sink call, capture, and flush is wrapped in `defer recover()` / error-swallow.
- The tee fires **regardless of `Debug`** — diagnostics must work even when the developer hasn't enabled debug. The sink is separate from the file writer.
- Tee **Info/Warn/Error only**; skip **Debug** (high-volume per-event).
- Severity mapping: `Error → (17,"ERROR")`, `Warn → (13,"WARN")`, `Info → (9,"INFO")`.
- Bearer token literal (must match the collector verbatim, shared across SDKs): `dgk_sdk_diag_3f9a2c7e1b8d4065af2e9c1d7b6a4f80`
- Endpoint: `https://otel.agentcat.com`, POST path `/v1/logs`. Scope name: `mcpcat-diagnostics`.
- Buffer cap `maxBuffer = 1000` (drop-oldest). Batch flush interval `batchFlush = 2 * time.Second`. HTTP client `Timeout: 5 * time.Second`.
- `DISABLE_DIAGNOSTICS` is **value-interpreted**: disabled iff `strings.ToLower(strings.TrimSpace(v))` ∉ {`""`,`false`,`0`,`no`,`off`}. Never bare `os.Getenv(...) != ""`.
- `deployment.environment` attribute reads **`ENVIRONMENT` only** (omit attribute if unset).
- `mcpcat.sdk.language = "go"`. Module paths: SDK `github.com/mcpcat/mcpcat-go-sdk`; official MCP SDK `github.com/modelcontextprotocol/go-sdk`; mcpgo `github.com/mark3labs/mcp-go`.
- Per-record `attributes` is always `[]` (empty); all metadata is resource-level.
- Internal modules (`mcpgo/`, `officialsdk/`) **cannot import `internal/`** — they reach diagnostics only through the public `mcpcat` facade.
- Run `go test -race` in all three modules; `gofmt -l .` must be empty; `make fmt && make vet && make test` must pass.
- Do **not** document the shared token or endpoint/token env overrides in the README — doc comments only.

---

### Task 1: Logging sink hook (`internal/logging`)

Add a `Level` type, a package-level diagnostics sink, and an `emit()` helper that tees to the sink **outside `l.mu`** and independent of `debug`, then writes to the file logger as before.

**Files:**
- Modify: `internal/logging/log.go`
- Test: `internal/logging/log_test.go` (extend)

**Interfaces:**
- Produces: `type Level int`; consts `LevelInfo, LevelWarn, LevelError, LevelDebug Level`; `func SetDiagnosticsSink(fn func(Level, string))`. `Info/Warn/Error/Debug` keep their signatures; `*f` variants unchanged (they delegate to the base method).

- [ ] **Step 1: Write the failing tests**

Append to `internal/logging/log_test.go`:

```go
func TestDiagnosticsSink_ReceivesLevels(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()
	defer SetDiagnosticsSink(nil)

	type rec struct {
		level Level
		msg   string
	}
	var got []rec
	var mu sync.Mutex
	SetDiagnosticsSink(func(l Level, m string) {
		mu.Lock()
		defer mu.Unlock()
		got = append(got, rec{l, m})
	})

	lg := New()
	lg.Info("info-line")
	lg.Warn("warn-line")
	lg.Error("error-line")
	lg.Debug("debug-line")

	mu.Lock()
	defer mu.Unlock()
	want := []rec{
		{LevelInfo, "info-line"},
		{LevelWarn, "warn-line"},
		{LevelError, "error-line"},
		{LevelDebug, "debug-line"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("entry %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDiagnosticsSink_FiresWhenDebugFalse(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()
	defer SetDiagnosticsSink(nil)

	// Default state: globalDebug == false.
	var called bool
	SetDiagnosticsSink(func(l Level, m string) { called = true })

	New().Info("hello")

	if !called {
		t.Fatal("sink must fire even when debug is false")
	}
}

func TestDiagnosticsSink_PanicDoesNotBreakLogging(t *testing.T) {
	resetGlobalState()
	defer resetGlobalState()
	defer SetDiagnosticsSink(nil)

	SetDiagnosticsSink(func(l Level, m string) { panic("boom") })

	lg := New()
	var buf bytes.Buffer
	lg.logger = log.New(&buf, "[MCPCat] ", log.LstdFlags)

	lg.Info("survives") // must not panic

	if !strings.Contains(buf.String(), "survives") {
		t.Fatalf("logging must continue after sink panic, got %q", buf.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/logging/ -run TestDiagnosticsSink -v`
Expected: FAIL — `SetDiagnosticsSink`, `Level`, `LevelInfo`, etc. undefined.

- [ ] **Step 3: Add the Level type, sink, and emit() in `internal/logging/log.go`**

Add near the top of the file, after the existing `import` block:

```go
// Level identifies the severity of a log entry for diagnostics teeing.
type Level int

const (
	LevelInfo Level = iota
	LevelWarn
	LevelError
	LevelDebug
)

var (
	diagSink   func(Level, string)
	diagSinkMu sync.RWMutex
)

// SetDiagnosticsSink registers a callback that receives every Info/Warn/Error/Debug
// entry with its level. Pass nil to clear. The sink fires independently of Debug.
func SetDiagnosticsSink(fn func(Level, string)) {
	diagSinkMu.Lock()
	defer diagSinkMu.Unlock()
	diagSink = fn
}

// emit tees the entry to the diagnostics sink (outside l.mu, never breaks logging)
// then writes to the file logger.
func (l *Logger) emit(level Level, prefix, msg string) {
	diagSinkMu.RLock()
	sink := diagSink
	diagSinkMu.RUnlock()
	if sink != nil {
		func() {
			defer func() { _ = recover() }()
			sink(level, msg)
		}()
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logger.Printf("%s: %s", prefix, msg)
}
```

Then replace the four base methods (keep the `*f` variants exactly as they are):

```go
func (l *Logger) Info(msg string)  { l.emit(LevelInfo, "INFO", msg) }
func (l *Logger) Warn(msg string)  { l.emit(LevelWarn, "WARN", msg) }
func (l *Logger) Error(msg string) { l.emit(LevelError, "ERROR", msg) }
func (l *Logger) Debug(msg string) { l.emit(LevelDebug, "DEBUG", msg) }
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/logging/ -race -v`
Expected: PASS (new sink tests + all existing tests still pass).

- [ ] **Step 5: Commit**

```bash
git add internal/logging/log.go internal/logging/log_test.go
git commit -m "feat(logging): add diagnostics sink hook teed outside mutex and debug gate"
```

---

### Task 2: Diagnostics constants + OTLP types (`internal/diagnostics`)

Create the new package's constants and the OTLP JSON types.

**Files:**
- Create: `internal/diagnostics/constants.go`
- Create: `internal/diagnostics/otlp.go`

**Interfaces:**
- Produces: consts `DiagnosticsScopeName`, `DefaultDiagnosticsEndpoint`, `DefaultDiagnosticsToken`, `sdkModulePath`, `maxBuffer`, `batchFlush`. Types `otlpAttribute`, `otlpAttrValue`, `otlpBody`, `otlpLogRecord`, `otlpScope`, `otlpScopeLogs`, `otlpResource`, `otlpResourceLogs`, `otlpPayload`.

- [ ] **Step 1: Create `internal/diagnostics/constants.go`**

```go
// Package diagnostics mirrors the SDK's internal operational logs to MCPCat's
// monitoring as OTLP/HTTP log records. It sends only operational metadata —
// never event payloads or user data. On by default; opt out via the
// DisableDiagnostics option or the DISABLE_DIAGNOSTICS env var.
package diagnostics

import "time"

const (
	// DiagnosticsScopeName is the OTLP instrumentation scope name.
	DiagnosticsScopeName = "mcpcat-diagnostics"

	// DefaultDiagnosticsEndpoint is the OTLP collector base URL. The POST path
	// /v1/logs is appended by resolveEndpoint. Override with DIAGNOSTICS_ENDPOINT.
	DefaultDiagnosticsEndpoint = "https://otel.agentcat.com"

	// DefaultDiagnosticsToken is the public shared ingestion key — NOT a secret.
	// It ships in the binary to deter drive-by traffic, paired with a server-side
	// rate limit, and must match the collector's bearer token. Override with
	// DIAGNOSTICS_TOKEN.
	DefaultDiagnosticsToken = "dgk_sdk_diag_3f9a2c7e1b8d4065af2e9c1d7b6a4f80"

	// sdkModulePath is this SDK's module path, used to resolve its own version.
	sdkModulePath = "github.com/mcpcat/mcpcat-go-sdk"

	// maxBuffer caps buffered log records (drop-oldest on overflow).
	maxBuffer = 1000
)

// batchFlush is the delay before a buffered batch is flushed.
const batchFlush = 2 * time.Second
```

- [ ] **Step 2: Create `internal/diagnostics/otlp.go`**

```go
package diagnostics

type otlpAttrValue struct {
	StringValue string `json:"stringValue"`
}

type otlpAttribute struct {
	Key   string        `json:"key"`
	Value otlpAttrValue `json:"value"`
}

type otlpBody struct {
	StringValue string `json:"stringValue"`
}

type otlpLogRecord struct {
	TimeUnixNano   string          `json:"timeUnixNano"`
	SeverityNumber int             `json:"severityNumber"`
	SeverityText   string          `json:"severityText"`
	Body           otlpBody        `json:"body"`
	Attributes     []otlpAttribute `json:"attributes"`
}

type otlpScope struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type otlpScopeLogs struct {
	Scope      otlpScope       `json:"scope"`
	LogRecords []otlpLogRecord `json:"logRecords"`
}

type otlpResource struct {
	Attributes []otlpAttribute `json:"attributes"`
}

type otlpResourceLogs struct {
	Resource  otlpResource    `json:"resource"`
	ScopeLogs []otlpScopeLogs `json:"scopeLogs"`
}

type otlpPayload struct {
	ResourceLogs []otlpResourceLogs `json:"resourceLogs"`
}
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./internal/diagnostics/`
Expected: builds (unused types are fine at package scope; no error).

- [ ] **Step 4: Commit**

```bash
git add internal/diagnostics/constants.go internal/diagnostics/otlp.go
git commit -m "feat(diagnostics): add constants and OTLP payload types"
```

---

### Task 3: Env resolution + record building + severity

**Files:**
- Create: `internal/diagnostics/env.go`
- Create: `internal/diagnostics/record.go`
- Test: `internal/diagnostics/record_test.go`

**Interfaces:**
- Consumes: `logging.Level` and its consts (Task 1); constants (Task 2).
- Produces: `func envDisabled() bool`; `func resolveEndpoint() string`; `func resolveToken() string`; `func severityFor(level logging.Level) (int, string)`; `func buildRecord(level logging.Level, msg string) otlpLogRecord`; `func BuildRecordForTest(level logging.Level, msg string) otlpLogRecord`.

- [ ] **Step 1: Write the failing test `internal/diagnostics/record_test.go`**

```go
package diagnostics

import (
	"testing"

	"github.com/mcpcat/mcpcat-go-sdk/internal/logging"
)

func TestBuildRecord_Severity(t *testing.T) {
	cases := []struct {
		level    logging.Level
		wantNum  int
		wantText string
	}{
		{logging.LevelError, 17, "ERROR"},
		{logging.LevelWarn, 13, "WARN"},
		{logging.LevelInfo, 9, "INFO"},
	}
	for _, c := range cases {
		rec := BuildRecordForTest(c.level, "the message")
		if rec.SeverityNumber != c.wantNum || rec.SeverityText != c.wantText {
			t.Errorf("level %v: got (%d,%q), want (%d,%q)",
				c.level, rec.SeverityNumber, rec.SeverityText, c.wantNum, c.wantText)
		}
		if rec.Body.StringValue != "the message" {
			t.Errorf("body = %q, want %q", rec.Body.StringValue, "the message")
		}
		if len(rec.Attributes) != 0 {
			t.Errorf("per-record attributes must be empty, got %d", len(rec.Attributes))
		}
		if rec.TimeUnixNano == "" {
			t.Error("timeUnixNano must be set")
		}
		for _, r := range rec.TimeUnixNano {
			if r < '0' || r > '9' {
				t.Errorf("timeUnixNano must be decimal digits, got %q", rec.TimeUnixNano)
				break
			}
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/diagnostics/ -run TestBuildRecord -v`
Expected: FAIL — `BuildRecordForTest` undefined.

- [ ] **Step 3: Create `internal/diagnostics/env.go`**

```go
package diagnostics

import (
	"os"
	"strings"
)

// envDisabled interprets DISABLE_DIAGNOSTICS by value. Disabled iff the
// normalized value is not one of "", "false", "0", "no", "off".
func envDisabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("DISABLE_DIAGNOSTICS")))
	switch v {
	case "", "false", "0", "no", "off":
		return false
	default:
		return true
	}
}

// resolveEndpoint returns the OTLP logs URL: DIAGNOSTICS_ENDPOINT or the default,
// with a single /v1/logs suffix.
func resolveEndpoint() string {
	base := DefaultDiagnosticsEndpoint
	if v := os.Getenv("DIAGNOSTICS_ENDPOINT"); v != "" {
		base = v
	}
	base = strings.TrimRight(base, "/")
	if strings.HasSuffix(base, "/v1/logs") {
		return base
	}
	return base + "/v1/logs"
}

// resolveToken returns DIAGNOSTICS_TOKEN or the default shared token.
func resolveToken() string {
	if v := os.Getenv("DIAGNOSTICS_TOKEN"); v != "" {
		return v
	}
	return DefaultDiagnosticsToken
}
```

- [ ] **Step 4: Create `internal/diagnostics/record.go`**

```go
package diagnostics

import (
	"strconv"
	"time"

	"github.com/mcpcat/mcpcat-go-sdk/internal/logging"
)

// severityFor maps a log level to an OTLP severity number and text.
// LevelDebug is filtered before this is called.
func severityFor(level logging.Level) (int, string) {
	switch level {
	case logging.LevelError:
		return 17, "ERROR"
	case logging.LevelWarn:
		return 13, "WARN"
	default:
		return 9, "INFO"
	}
}

func buildRecord(level logging.Level, msg string) otlpLogRecord {
	num, text := severityFor(level)
	return otlpLogRecord{
		TimeUnixNano:   strconv.FormatInt(time.Now().UnixNano(), 10),
		SeverityNumber: num,
		SeverityText:   text,
		Body:           otlpBody{StringValue: msg},
		Attributes:     []otlpAttribute{},
	}
}

// BuildRecordForTest exposes buildRecord for tests.
func BuildRecordForTest(level logging.Level, msg string) otlpLogRecord {
	return buildRecord(level, msg)
}
```

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/diagnostics/ -race -run TestBuildRecord -v`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/diagnostics/env.go internal/diagnostics/record.go internal/diagnostics/record_test.go
git commit -m "feat(diagnostics): add env resolution, severity mapping, and record builder"
```

---

### Task 4: Static resource attributes + install_id

**Files:**
- Create: `internal/diagnostics/attributes.go`
- Test: `internal/diagnostics/attributes_test.go`

**Interfaces:**
- Consumes: constants (Task 2); `session.GetDependencyVersion` from `internal/session`.
- Produces: `func buildStaticAttributes(projectID, integration, mcpSDKPath string) []otlpAttribute`; `func computeInstallID() string`.

- [ ] **Step 1: Write the failing test `internal/diagnostics/attributes_test.go`**

```go
package diagnostics

import "testing"

func attrMap(attrs []otlpAttribute) map[string]string {
	m := make(map[string]string, len(attrs))
	for _, a := range attrs {
		m[a.Key] = a.Value.StringValue
	}
	return m
}

func TestBuildStaticAttributes_WithProjectID(t *testing.T) {
	m := attrMap(buildStaticAttributes("proj_123", "officialsdk", "github.com/modelcontextprotocol/go-sdk"))

	if m["mcpcat.project_id"] != "proj_123" {
		t.Errorf("project_id = %q, want proj_123", m["mcpcat.project_id"])
	}
	if _, ok := m["mcpcat.install_id"]; ok {
		t.Error("install_id must be absent when project_id is set")
	}
	if m["mcpcat.sdk.language"] != "go" {
		t.Errorf("sdk.language = %q, want go", m["mcpcat.sdk.language"])
	}
	if m["mcpcat.integration"] != "officialsdk" {
		t.Errorf("integration = %q, want officialsdk", m["mcpcat.integration"])
	}
	if m["os.type"] == "" {
		t.Error("os.type must be present")
	}
	if m["host.arch"] == "" {
		t.Error("host.arch must be present")
	}
	if m["process.runtime.name"] != "go" {
		t.Errorf("process.runtime.name = %q, want go", m["process.runtime.name"])
	}
}

func TestBuildStaticAttributes_WithoutProjectID(t *testing.T) {
	m := attrMap(buildStaticAttributes("", "mcpgo", "github.com/mark3labs/mcp-go"))

	if _, ok := m["mcpcat.project_id"]; ok {
		t.Error("project_id must be absent when empty")
	}
	if m["mcpcat.install_id"] == "" {
		t.Error("install_id must be present (anonymous) when project_id is empty")
	}
	if len(m["mcpcat.install_id"]) != 16 {
		t.Errorf("install_id must be 16 hex chars, got %q", m["mcpcat.install_id"])
	}
}

func TestComputeInstallID_StableAndShort(t *testing.T) {
	a := computeInstallID()
	b := computeInstallID()
	if a != b {
		t.Errorf("install_id must be stable: %q != %q", a, b)
	}
	if len(a) != 16 {
		t.Errorf("install_id length = %d, want 16", len(a))
	}
}

func TestBuildStaticAttributes_DeploymentEnvironment(t *testing.T) {
	t.Setenv("ENVIRONMENT", "")
	if _, ok := attrMap(buildStaticAttributes("p", "x", "y"))["deployment.environment"]; ok {
		t.Error("deployment.environment must be omitted when ENVIRONMENT is unset")
	}
	t.Setenv("ENVIRONMENT", "production")
	if attrMap(buildStaticAttributes("p", "x", "y"))["deployment.environment"] != "production" {
		t.Error("deployment.environment must equal ENVIRONMENT")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/diagnostics/ -run 'TestBuildStaticAttributes|TestComputeInstallID' -v`
Expected: FAIL — `buildStaticAttributes`, `computeInstallID` undefined.

- [ ] **Step 3: Create `internal/diagnostics/attributes.go`**

```go
package diagnostics

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"runtime"
	"strconv"

	"github.com/mcpcat/mcpcat-go-sdk/internal/session"
)

// computeInstallID returns a stable, anonymous 16-hex-char id derived from the
// hostname and executable path. Best-effort; empty inputs are tolerated.
func computeInstallID() string {
	hostname, _ := os.Hostname()
	exePath, _ := os.Executable()
	h := sha256.Sum256([]byte(hostname + "|" + exePath))
	return hex.EncodeToString(h[:])[:16]
}

// buildStaticAttributes builds the OTLP resource attributes (identity +
// environment metadata). Empty values are omitted entirely.
func buildStaticAttributes(projectID, integration, mcpSDKPath string) []otlpAttribute {
	var attrs []otlpAttribute
	add := func(key, value string) {
		if value != "" {
			attrs = append(attrs, otlpAttribute{Key: key, Value: otlpAttrValue{StringValue: value}})
		}
	}

	if projectID != "" {
		add("mcpcat.project_id", projectID)
	} else {
		add("mcpcat.install_id", computeInstallID())
	}

	add("mcpcat.sdk.language", "go")
	add("mcpcat.sdk.version", session.GetDependencyVersion(sdkModulePath))
	add("mcpcat.mcp_sdk.version", session.GetDependencyVersion(mcpSDKPath))
	add("mcpcat.integration", integration)

	add("process.runtime.name", "go")
	add("process.runtime.version", runtime.Version())
	add("process.pid", strconv.Itoa(os.Getpid()))

	add("os.type", runtime.GOOS)
	add("host.arch", runtime.GOARCH)
	add("host.cpu.count", strconv.Itoa(runtime.NumCPU()))

	add("deployment.environment", os.Getenv("ENVIRONMENT"))

	return attrs
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/diagnostics/ -race -run 'TestBuildStaticAttributes|TestComputeInstallID' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/diagnostics/attributes.go internal/diagnostics/attributes_test.go
git commit -m "feat(diagnostics): add static resource attributes and anonymous install id"
```

---

### Task 5: Init, capture, Flush, HTTP export + opt-out

The package's stateful core: idempotent `Init`, bounded `capture`, batched `Flush` with fire-and-forget POST, plus test helpers.

**Files:**
- Create: `internal/diagnostics/diagnostics.go`
- Test: `internal/diagnostics/optout_test.go`
- Test: `internal/diagnostics/export_test.go`

**Interfaces:**
- Consumes: everything from Tasks 2–4; `logging.SetDiagnosticsSink`, `logging.Level`.
- Produces: `func Init(projectID string, disabled bool, integration, mcpSDKPath string)`; `func Flush()`; `func Enabled() bool`; `func ResetForTest()`; `func StaticAttributesForTest() []otlpAttribute`; internal `capture(level logging.Level, msg string)`.

- [ ] **Step 1: Write the failing tests**

`internal/diagnostics/optout_test.go`:

```go
package diagnostics

import (
	"testing"

	"github.com/mcpcat/mcpcat-go-sdk/internal/logging"
)

func TestInit_EnabledByDefault(t *testing.T) {
	t.Setenv("DISABLE_DIAGNOSTICS", "")
	ResetForTest()
	defer ResetForTest()

	Init("proj_x", false, "officialsdk", "github.com/modelcontextprotocol/go-sdk")
	if !Enabled() {
		t.Fatal("diagnostics must be enabled by default")
	}
}

func TestInit_DisabledViaOption(t *testing.T) {
	t.Setenv("DISABLE_DIAGNOSTICS", "")
	ResetForTest()
	defer ResetForTest()

	Init("proj_x", true, "officialsdk", "p")
	if Enabled() {
		t.Fatal("disabled=true must disable diagnostics")
	}
}

func TestInit_EnvDisableValues(t *testing.T) {
	disable := []string{"true", "TRUE", "1", "yes", "on"}
	for _, v := range disable {
		t.Run("disable_"+v, func(t *testing.T) {
			t.Setenv("DISABLE_DIAGNOSTICS", v)
			ResetForTest()
			defer ResetForTest()
			Init("p", false, "x", "y")
			if Enabled() {
				t.Errorf("%q must disable diagnostics", v)
			}
		})
	}

	stay := []string{"false", "0", "no", "off", "  "}
	for _, v := range stay {
		t.Run("enabled_"+v, func(t *testing.T) {
			t.Setenv("DISABLE_DIAGNOSTICS", v)
			ResetForTest()
			defer ResetForTest()
			Init("p", false, "x", "y")
			if !Enabled() {
				t.Errorf("%q must NOT disable diagnostics", v)
			}
		})
	}
}

func TestInit_RegistersSink(t *testing.T) {
	t.Setenv("DISABLE_DIAGNOSTICS", "")
	ResetForTest()
	defer ResetForTest()

	Init("p", false, "x", "y")
	// The sink should be registered; capturing an Info entry must enqueue a record.
	logging.New().Info("a setup line")
	if n := bufferLenForTest(); n == 0 {
		t.Fatal("Init must register the sink so Info entries are captured")
	}
}
```

`internal/diagnostics/export_test.go`:

```go
package diagnostics

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/mcpcat/mcpcat-go-sdk/internal/logging"
)

func TestExport_PostsOTLPWithAuth(t *testing.T) {
	type captured struct {
		path string
		auth string
		body []byte
	}
	ch := make(chan captured, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		ch <- captured{path: r.URL.Path, auth: r.Header.Get("Authorization"), body: b}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("DIAGNOSTICS_ENDPOINT", srv.URL)
	t.Setenv("DISABLE_DIAGNOSTICS", "")
	ResetForTest()
	defer ResetForTest()

	Init("proj_1", false, "officialsdk", "github.com/modelcontextprotocol/go-sdk")
	capture(logging.LevelInfo, "MCPCat setup started | project proj_1")
	Flush()

	got := <-ch

	if !strings.HasSuffix(got.path, "/v1/logs") {
		t.Errorf("path = %q, want suffix /v1/logs", got.path)
	}
	if !strings.HasPrefix(got.auth, "Bearer dgk_sdk_diag_") {
		t.Errorf("auth = %q, want Bearer dgk_sdk_diag_...", got.auth)
	}

	var payload otlpPayload
	if err := json.Unmarshal(got.body, &payload); err != nil {
		t.Fatalf("body is not valid OTLP JSON: %v\n%s", err, got.body)
	}
	if len(payload.ResourceLogs) != 1 {
		t.Fatalf("resourceLogs = %d, want 1", len(payload.ResourceLogs))
	}
	rl := payload.ResourceLogs[0]
	if len(rl.ScopeLogs) != 1 || rl.ScopeLogs[0].Scope.Name != DiagnosticsScopeName {
		t.Fatalf("scope = %+v, want name %q", rl.ScopeLogs, DiagnosticsScopeName)
	}
	recs := rl.ScopeLogs[0].LogRecords
	if len(recs) == 0 || recs[0].Body.StringValue != "MCPCat setup started | project proj_1" {
		t.Fatalf("logRecords = %+v, want body match", recs)
	}
	var hasProject bool
	for _, a := range rl.Resource.Attributes {
		if a.Key == "mcpcat.project_id" && a.Value.StringValue == "proj_1" {
			hasProject = true
		}
	}
	if !hasProject {
		t.Error("resource attributes must include mcpcat.project_id=proj_1")
	}
}

func TestExport_TokenOverride(t *testing.T) {
	ch := make(chan string, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.ReadAll(r.Body)
		ch <- r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	t.Setenv("DIAGNOSTICS_ENDPOINT", srv.URL)
	t.Setenv("DIAGNOSTICS_TOKEN", "custom-token-123")
	t.Setenv("DISABLE_DIAGNOSTICS", "")
	ResetForTest()
	defer ResetForTest()

	Init("proj_1", false, "officialsdk", "p")
	capture(logging.LevelInfo, "line")
	Flush()

	if auth := <-ch; auth != "Bearer custom-token-123" {
		t.Errorf("auth = %q, want Bearer custom-token-123", auth)
	}
}

func TestFlush_DisabledIsNoop(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("disabled diagnostics must not POST")
	}))
	defer srv.Close()

	t.Setenv("DIAGNOSTICS_ENDPOINT", srv.URL)
	ResetForTest()
	defer ResetForTest()

	Init("p", true, "x", "y") // disabled
	capture(logging.LevelInfo, "line")
	Flush()
}
```

> Note: `bufferLenForTest` is added in Step 2 alongside the other test helpers.

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/diagnostics/ -run 'TestInit|TestExport|TestFlush' -v`
Expected: FAIL — `Init`, `Flush`, `Enabled`, `ResetForTest`, `capture`, `bufferLenForTest` undefined.

- [ ] **Step 3: Create `internal/diagnostics/diagnostics.go`**

```go
package diagnostics

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/mcpcat/mcpcat-go-sdk/internal/logging"
	"github.com/mcpcat/mcpcat-go-sdk/internal/session"
)

var (
	mu           sync.Mutex
	initialized  bool
	enabled      bool
	buffer       []otlpLogRecord
	staticAttrs  []otlpAttribute
	flushPending bool
	sdkVersion   string

	httpClient = &http.Client{Timeout: 5 * time.Second}
)

// Init registers the diagnostics sink and builds static attributes. Idempotent:
// only the first call per process takes effect. enabled = !disabled && !envDisabled().
// Never panics.
func Init(projectID string, disabled bool, integration, mcpSDKPath string) {
	defer func() { _ = recover() }()
	mu.Lock()
	if initialized {
		mu.Unlock()
		return
	}
	initialized = true
	enabled = !disabled && !envDisabled()
	if !enabled {
		mu.Unlock()
		return
	}
	staticAttrs = buildStaticAttributes(projectID, integration, mcpSDKPath)
	sdkVersion = session.GetDependencyVersion(sdkModulePath)
	mu.Unlock()

	logging.SetDiagnosticsSink(capture)
}

// capture appends a record (drop-oldest at maxBuffer) and schedules a flush.
// Debug entries are ignored. Never panics, never blocks.
func capture(level logging.Level, msg string) {
	defer func() { _ = recover() }()
	if level == logging.LevelDebug {
		return
	}
	mu.Lock()
	if !enabled {
		mu.Unlock()
		return
	}
	if len(buffer) >= maxBuffer {
		buffer = buffer[1:]
	}
	buffer = append(buffer, buildRecord(level, msg))
	schedule := !flushPending
	if schedule {
		flushPending = true
	}
	mu.Unlock()

	if schedule {
		time.AfterFunc(batchFlush, Flush)
	}
}

// Flush sends the buffered batch best-effort. Never returns an error; never panics.
func Flush() {
	defer func() { _ = recover() }()

	mu.Lock()
	flushPending = false
	if !enabled || len(buffer) == 0 {
		mu.Unlock()
		return
	}
	records := buffer
	buffer = nil
	attrs := staticAttrs
	ver := sdkVersion
	mu.Unlock()

	payload := otlpPayload{
		ResourceLogs: []otlpResourceLogs{{
			Resource: otlpResource{Attributes: attrs},
			ScopeLogs: []otlpScopeLogs{{
				Scope:      otlpScope{Name: DiagnosticsScopeName, Version: ver},
				LogRecords: records,
			}},
		}},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return
	}
	req, err := http.NewRequest(http.MethodPost, resolveEndpoint(), bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	if token := resolveToken(); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return
	}
	_ = resp.Body.Close()
}

// Enabled reports whether diagnostics is active. For tests.
func Enabled() bool {
	mu.Lock()
	defer mu.Unlock()
	return enabled
}

// ResetForTest clears all package state and unregisters the sink. For tests.
func ResetForTest() {
	mu.Lock()
	initialized = false
	enabled = false
	buffer = nil
	staticAttrs = nil
	flushPending = false
	sdkVersion = ""
	mu.Unlock()
	logging.SetDiagnosticsSink(nil)
}

// StaticAttributesForTest returns the built resource attributes. For tests.
func StaticAttributesForTest() []otlpAttribute {
	mu.Lock()
	defer mu.Unlock()
	return staticAttrs
}

func bufferLenForTest() int {
	mu.Lock()
	defer mu.Unlock()
	return len(buffer)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/diagnostics/ -race -v`
Expected: PASS (all diagnostics tests).

- [ ] **Step 5: Commit**

```bash
git add internal/diagnostics/diagnostics.go internal/diagnostics/optout_test.go internal/diagnostics/export_test.go
git commit -m "feat(diagnostics): add init/capture/flush with fire-and-forget OTLP export"
```

---

### Task 6: `core.Options.DisableDiagnostics`

**Files:**
- Modify: `internal/core/types.go`
- Test: `internal/core/types_test.go` (create or extend — minimal field-presence assertion)

**Interfaces:**
- Produces: `Options.DisableDiagnostics bool`.

- [ ] **Step 1: Write the failing test**

Add to `internal/core/types_test.go` (create the file with this content if it does not exist):

```go
package core

import "testing"

func TestOptions_DisableDiagnosticsField(t *testing.T) {
	o := Options{DisableDiagnostics: true}
	if !o.DisableDiagnostics {
		t.Fatal("DisableDiagnostics field must exist and be settable")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/core/ -run TestOptions_DisableDiagnosticsField -v`
Expected: FAIL — `DisableDiagnostics` unknown field.

- [ ] **Step 3: Add the field in `internal/core/types.go`**

In the `Options` struct, add:

```go
	// DisableDiagnostics disables MCPCat's internal SDK diagnostics (anonymous
	// operational error/setup reporting used to detect SDK setup failures).
	// On by default; also disable via the DISABLE_DIAGNOSTICS env var.
	// Local ~/mcpcat.log logging is unaffected.
	DisableDiagnostics bool
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/core/ -run TestOptions_DisableDiagnosticsField -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/core/types.go internal/core/types_test.go
git commit -m "feat(core): add Options.DisableDiagnostics"
```

---

### Task 7: Facade — `InitDiagnostics`, `LogSetupComplete`, `LogSetupFailed`, `Shutdown` flush

**Files:**
- Create: `diagnostics.go` (root package `mcpcat`) — facade wrappers + beacons.
- Modify: `mcpcat.go` — make `Shutdown` flush diagnostics.
- Test: `diagnostics_facade_test.go` (root package `mcpcat`).

**Interfaces:**
- Consumes: `internal/diagnostics`, `internal/logging`, root `Options` (alias of `core.Options`).
- Produces: `func InitDiagnostics(projectID string, disabled bool, integration, mcpSDKPath string)`; `func LogSetupComplete(projectID string, opts *Options)`; `func LogSetupFailed(reason string)`; `func ResetDiagnosticsForTest()`. `Shutdown` now calls `diagnostics.Flush()`.

- [ ] **Step 1: Write the failing test `diagnostics_facade_test.go`**

```go
package mcpcat

import (
	"bytes"
	"log"
	"strings"
	"testing"

	"github.com/mcpcat/mcpcat-go-sdk/internal/logging"
)

// captureLogger swaps the singleton logger's writer to buf and returns a restore func.
func captureLogger(t *testing.T, buf *bytes.Buffer) {
	t.Helper()
	lg := logging.New()
	lg.SwapWriterForTest(log.New(buf, "[MCPCat] ", log.LstdFlags))
}

func TestInitDiagnostics_EmitsStartBeacon(t *testing.T) {
	t.Setenv("DISABLE_DIAGNOSTICS", "1") // keep network off; only assert the log line
	logging.ResetForTesting()
	ResetDiagnosticsForTest()
	defer ResetDiagnosticsForTest()

	var buf bytes.Buffer
	captureLogger(t, &buf)

	InitDiagnostics("proj_abc", true, "officialsdk", "github.com/modelcontextprotocol/go-sdk")

	out := buf.String()
	if !strings.Contains(out, "MCPCat setup started") ||
		!strings.Contains(out, "proj_abc") ||
		!strings.Contains(out, "integration officialsdk") {
		t.Fatalf("start beacon missing/incomplete: %q", out)
	}
}

func TestInitDiagnostics_TelemetryOnlyLabel(t *testing.T) {
	t.Setenv("DISABLE_DIAGNOSTICS", "1")
	logging.ResetForTesting()
	ResetDiagnosticsForTest()
	defer ResetDiagnosticsForTest()

	var buf bytes.Buffer
	captureLogger(t, &buf)

	InitDiagnostics("", true, "mcpgo", "github.com/mark3labs/mcp-go")

	if !strings.Contains(buf.String(), "(telemetry-only)") {
		t.Fatalf("empty projectID must render as (telemetry-only): %q", buf.String())
	}
}

func TestLogSetupComplete_MetadataOnly(t *testing.T) {
	t.Setenv("DISABLE_DIAGNOSTICS", "1")
	logging.ResetForTesting()
	ResetDiagnosticsForTest()
	defer ResetDiagnosticsForTest()

	var buf bytes.Buffer
	captureLogger(t, &buf)

	LogSetupComplete("proj_abc", &Options{DisableToolCallContext: false, DisableReportMissing: true})

	out := buf.String()
	if !strings.Contains(out, "MCPCat setup complete") ||
		!strings.Contains(out, "proj_abc") ||
		!strings.Contains(out, "context=true") ||
		!strings.Contains(out, "report_missing=false") {
		t.Fatalf("complete beacon wrong: %q", out)
	}
}

func TestLogSetupFailed_IsError(t *testing.T) {
	t.Setenv("DISABLE_DIAGNOSTICS", "1")
	logging.ResetForTesting()
	ResetDiagnosticsForTest()
	defer ResetDiagnosticsForTest()

	var buf bytes.Buffer
	captureLogger(t, &buf)

	LogSetupFailed("projectID must not be empty")

	out := buf.String()
	if !strings.Contains(out, "ERROR:") || !strings.Contains(out, "MCPCat setup failed") {
		t.Fatalf("failure must log ERROR: %q", out)
	}
}
```

- [ ] **Step 2: Add `SwapWriterForTest` to `internal/logging/log.go`**

The facade test needs to capture the singleton logger's output. Add this test seam to `internal/logging/log.go`:

```go
// SwapWriterForTest replaces the logger's underlying *log.Logger. For tests only.
func (l *Logger) SwapWriterForTest(lg *log.Logger) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.logger = lg
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test . -run 'TestInitDiagnostics|TestLogSetup' -v`
Expected: FAIL — `InitDiagnostics`, `LogSetupComplete`, `LogSetupFailed`, `ResetDiagnosticsForTest` undefined.

- [ ] **Step 4: Create `diagnostics.go` in the root package**

```go
package mcpcat

import (
	"github.com/mcpcat/mcpcat-go-sdk/internal/diagnostics"
	"github.com/mcpcat/mcpcat-go-sdk/internal/logging"
)

// InitDiagnostics initializes internal SDK diagnostics and emits the setup-start
// beacon. Call it early in Track — before validation — so setup failures are
// captured. Idempotent across the process.
func InitDiagnostics(projectID string, disabled bool, integration, mcpSDKPath string) {
	diagnostics.Init(projectID, disabled, integration, mcpSDKPath)
	logging.New().Infof("MCPCat setup started | project %s | integration %s",
		orTelemetryOnly(projectID), integration)
}

// LogSetupComplete emits the setup-complete beacon (metadata only).
func LogSetupComplete(projectID string, opts *Options) {
	logging.New().Infof("MCPCat setup complete | project %s | context=%t report_missing=%t",
		orTelemetryOnly(projectID), !opts.DisableToolCallContext, !opts.DisableReportMissing)
}

// LogSetupFailed logs a setup failure as ERROR so it surfaces in diagnostics.
func LogSetupFailed(reason string) {
	logging.New().Errorf("MCPCat setup failed | %s", reason)
}

// ResetDiagnosticsForTest resets internal diagnostics + logging sink state. For tests.
func ResetDiagnosticsForTest() {
	diagnostics.ResetForTest()
}

func orTelemetryOnly(projectID string) string {
	if projectID == "" {
		return "(telemetry-only)"
	}
	return projectID
}
```

- [ ] **Step 5: Make `Shutdown` flush diagnostics in `mcpcat.go`**

Locate the `Shutdown` function. Add the diagnostics import to the file's import block:

```go
	"github.com/mcpcat/mcpcat-go-sdk/internal/diagnostics"
```

Change the body from `return publisher.ShutdownGlobal(ctx)` to:

```go
func Shutdown(ctx context.Context) error {
	err := publisher.ShutdownGlobal(ctx)
	diagnostics.Flush()
	return err
}
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `go test . -race -run 'TestInitDiagnostics|TestLogSetup' -v`
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add diagnostics.go mcpcat.go internal/logging/log.go diagnostics_facade_test.go
git commit -m "feat(mcpcat): add diagnostics facade beacons and flush on shutdown"
```

---

### Task 8: Scrub `internal/event/helpers.go: LogEvent` → metadata only

Convert the payload-bearing lines to counts/presence so a teed Info entry can never leak values.

**Files:**
- Modify: `internal/event/helpers.go`
- Test: `internal/event/helpers_test.go` (extend with a no-payload guard via the diagnostics sink)

**Interfaces:** unchanged (`LogEvent` keeps its signature).

- [ ] **Step 1: Write the failing test in `internal/event/helpers_test.go`**

```go
func TestLogEvent_NoPayloadLeak(t *testing.T) {
	logging.ResetForTesting()
	defer logging.ResetForTesting()
	defer logging.SetDiagnosticsSink(nil)

	const (
		secretParam  = "SUPER_SECRET_PARAM_VALUE"
		secretResp   = "SUPER_SECRET_RESPONSE_VALUE"
		secretIntent = "SUPER_SECRET_INTENT_TEXT"
		secretActor  = "SUPER_SECRET_ACTOR_NAME"
		secretData   = "SUPER_SECRET_IDENTIFY_DATA"
	)

	var captured []string
	logging.SetDiagnosticsSink(func(_ logging.Level, msg string) {
		captured = append(captured, msg)
	})

	intent := secretIntent
	isErr := true
	actorName := secretActor
	evt := &Event{}
	evt.SessionId = "ses_1"
	evt.UserIntent = &intent
	evt.IsError = &isErr
	evt.IdentifyActorName = &actorName
	evt.Parameters = map[string]any{"k": secretParam}
	evt.Response = map[string]any{"r": secretResp}
	evt.IdentifyData = map[string]any{"d": secretData}

	LogEvent(logging.New(), evt, "Test Event")

	joined := strings.Join(captured, "\n")
	for _, s := range []string{secretParam, secretResp, secretIntent, secretActor, secretData} {
		if strings.Contains(joined, s) {
			t.Errorf("payload value %q leaked into diagnostics:\n%s", s, joined)
		}
	}
	// Sanity: counts/presence still emitted.
	if !strings.Contains(joined, "Parameters: 1 field") {
		t.Errorf("expected parameter count, got:\n%s", joined)
	}
}
```

Ensure the test file imports `strings` and `github.com/mcpcat/mcpcat-go-sdk/internal/logging` (add if missing).

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/event/ -run TestLogEvent_NoPayloadLeak -v`
Expected: FAIL — secret values leak (current `LogEvent` dumps them).

- [ ] **Step 3: Scrub the payload-bearing lines in `internal/event/helpers.go`**

Replace the **Error Details** block:

```go
	// Error status
	if evt.IsError != nil && *evt.IsError {
		logger.Infof("  Is Error: true")
	}
```

Replace the **User intent** block:

```go
	// User intent (length only — never the text)
	if evt.UserIntent != nil {
		logger.Infof("  User Intent: %d chars", len(*evt.UserIntent))
	}
```

Replace the **Parameters** block:

```go
	// Parameters (count only)
	if len(evt.Parameters) > 0 {
		logger.Infof("  Parameters: %d field(s)", len(evt.Parameters))
	}
```

Replace the **Response** block:

```go
	// Response (count only)
	if len(evt.Response) > 0 {
		logger.Infof("  Response: %d field(s)", len(evt.Response))
	}
```

Replace the **Identity** block (keep Actor ID, drop Actor Name value, count IdentifyData):

```go
	// Identity info (Actor ID kept; name value dropped; data counted)
	if evt.IdentifyActorGivenId != nil || evt.IdentifyActorName != nil {
		logger.Infof("  Identity:")
		if evt.IdentifyActorGivenId != nil {
			logger.Infof("    Actor ID: %s", *evt.IdentifyActorGivenId)
		}
		if len(evt.IdentifyData) > 0 {
			logger.Infof("    Additional Data: %d field(s)", len(evt.IdentifyData))
		}
	}
```

Remove the now-unused `fmt` import only if it is no longer referenced elsewhere in the file (the Response truncation was the sole `fmt` user — verify with `go vet`; if `fmt` is still used, leave the import). Run `goimports`/`gofmt` to clean up.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/event/ -race -run TestLogEvent -v`
Expected: PASS (new no-payload test + existing `TestLogEvent` — update existing assertions in `helpers_test.go` that expected the old value-dumping format strings, e.g. `"    %s: %v"` for parameters, to the new metadata strings).

- [ ] **Step 5: Commit**

```bash
git add internal/event/helpers.go internal/event/helpers_test.go
git commit -m "refactor(event): make LogEvent metadata-only to prevent payload leakage"
```

---

### Task 9: `officialsdk` — option, mapping, Track wiring + module tests

**Files:**
- Modify: `officialsdk/officialsdk.go`
- Create: `officialsdk/diagnostics_test.go`

**Interfaces:**
- Consumes: facade `mcpcat.InitDiagnostics`, `mcpcat.LogSetupComplete`, `mcpcat.LogSetupFailed`, `mcpcat.ResetDiagnosticsForTest`, `mcpcat.Shutdown`.
- Produces: `officialsdk.Options.DisableDiagnostics bool`.

- [ ] **Step 1: Write the failing module test `officialsdk/diagnostics_test.go`**

```go
package officialsdk

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mcpcat/mcpcat-go-sdk"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestMain(m *testing.M) {
	// Keep diagnostics off for the suite by default so unrelated Track tests
	// never emit real network traffic. Beacon tests opt back in per-test.
	_ = os.Setenv("DISABLE_DIAGNOSTICS", "1")
	os.Exit(m.Run())
}

func newDiagServer(t *testing.T) (chan string, *httptest.Server) {
	t.Helper()
	ch := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		ch <- string(b)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return ch, srv
}

func TestTrack_EmitsSetupBeacons(t *testing.T) {
	ch, srv := newDiagServer(t)
	t.Setenv("DIAGNOSTICS_ENDPOINT", srv.URL)
	t.Setenv("DISABLE_DIAGNOSTICS", "false")
	mcpcat.ResetDiagnosticsForTest()
	t.Cleanup(mcpcat.ResetDiagnosticsForTest)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	shutdown, err := Track(server, "proj_test", &Options{})
	if err != nil {
		t.Fatalf("Track returned error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	body := drain(ch)
	if !strings.Contains(body, "MCPCat setup started") ||
		!strings.Contains(body, "MCPCat setup complete") ||
		!strings.Contains(body, "proj_test") ||
		!strings.Contains(body, "integration officialsdk") {
		t.Fatalf("beacons missing in diagnostics body:\n%s", body)
	}
}

func TestTrack_EmptyProjectIDLogsError(t *testing.T) {
	ch, srv := newDiagServer(t)
	t.Setenv("DIAGNOSTICS_ENDPOINT", srv.URL)
	t.Setenv("DISABLE_DIAGNOSTICS", "false")
	mcpcat.ResetDiagnosticsForTest()
	t.Cleanup(mcpcat.ResetDiagnosticsForTest)

	server := mcp.NewServer(&mcp.Implementation{Name: "test", Version: "1.0.0"}, nil)
	_, err := Track(server, "", &Options{})
	if err != mcpcat.ErrEmptyProjectID {
		t.Fatalf("err = %v, want ErrEmptyProjectID", err)
	}
	_ = mcpcat.Shutdown(context.Background()) // flush

	body := drain(ch)
	if !strings.Contains(body, "MCPCat setup failed") {
		t.Fatalf("expected setup-failed record:\n%s", body)
	}
}

func drain(ch chan string) string {
	var b strings.Builder
	for {
		select {
		case s := <-ch:
			b.WriteString(s)
			b.WriteByte('\n')
		default:
			return b.String()
		}
	}
}
```

> The `mcp.NewServer` signature is from `github.com/modelcontextprotocol/go-sdk/mcp`. If the existing `officialsdk` tests construct a server differently, copy their exact construction pattern instead of the line above.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd officialsdk && go test -run 'TestTrack_EmitsSetupBeacons|TestTrack_EmptyProjectIDLogsError' -v; cd ..`
Expected: FAIL — `Options.DisableDiagnostics` unknown and/or no beacons emitted.

- [ ] **Step 3: Add the option and wire `Track` in `officialsdk/officialsdk.go`**

Add to the `Options` struct:

```go
	// DisableDiagnostics disables MCPCat's internal SDK diagnostics. On by default;
	// also disable via the DISABLE_DIAGNOSTICS env var. ~/mcpcat.log is unaffected.
	DisableDiagnostics bool
```

Reorder the top of `Track` so options resolve first, then diagnostics init runs **before** validation, then guards log failures. Replace the guard/opts prologue with:

```go
	if opts == nil {
		opts = DefaultOptions()
	}

	mcpcat.InitDiagnostics(projectID, opts.DisableDiagnostics, "officialsdk",
		"github.com/modelcontextprotocol/go-sdk")

	if mcpServer == nil {
		mcpcat.LogSetupFailed("server must not be nil")
		return nil, mcpcat.ErrNilServer
	}
	if projectID == "" {
		mcpcat.LogSetupFailed("projectID must not be empty")
		return nil, mcpcat.ErrEmptyProjectID
	}
```

In the `coreOpts` builder, add the mapping:

```go
		DisableDiagnostics:         opts.DisableDiagnostics,
```

Immediately before `return shutdownFn, nil`, add the complete beacon:

```go
	mcpcat.LogSetupComplete(projectID, coreOpts)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd officialsdk && go test -race ./... -v; cd ..`
Expected: PASS (new diagnostics tests + existing officialsdk tests).

- [ ] **Step 5: Commit**

```bash
git add officialsdk/officialsdk.go officialsdk/diagnostics_test.go
git commit -m "feat(officialsdk): wire diagnostics init, beacons, and setup-failure logging into Track"
```

---

### Task 10: `mcpgo` — option, mapping, Track wiring + module tests

Mirror Task 9 for the mcp-go adapter.

**Files:**
- Modify: `mcpgo/mcpgo.go`
- Create: `mcpgo/diagnostics_test.go`

**Interfaces:**
- Produces: `mcpgo.Options.DisableDiagnostics bool`.

- [ ] **Step 1: Write the failing module test `mcpgo/diagnostics_test.go`**

```go
package mcpgo

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/mcpcat/mcpcat-go-sdk"
	"github.com/mark3labs/mcp-go/server"
)

func TestMain(m *testing.M) {
	_ = os.Setenv("DISABLE_DIAGNOSTICS", "1")
	os.Exit(m.Run())
}

func newDiagServer(t *testing.T) (chan string, *httptest.Server) {
	t.Helper()
	ch := make(chan string, 8)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		ch <- string(b)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return ch, srv
}

func drain(ch chan string) string {
	var b strings.Builder
	for {
		select {
		case s := <-ch:
			b.WriteString(s)
			b.WriteByte('\n')
		default:
			return b.String()
		}
	}
}

func TestTrack_EmitsSetupBeacons(t *testing.T) {
	ch, srv := newDiagServer(t)
	t.Setenv("DIAGNOSTICS_ENDPOINT", srv.URL)
	t.Setenv("DISABLE_DIAGNOSTICS", "false")
	mcpcat.ResetDiagnosticsForTest()
	t.Cleanup(mcpcat.ResetDiagnosticsForTest)

	s := server.NewMCPServer("test", "1.0.0")
	shutdown, err := Track(s, "proj_test", &Options{})
	if err != nil {
		t.Fatalf("Track returned error: %v", err)
	}
	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}

	body := drain(ch)
	if !strings.Contains(body, "MCPCat setup started") ||
		!strings.Contains(body, "MCPCat setup complete") ||
		!strings.Contains(body, "proj_test") ||
		!strings.Contains(body, "integration mcpgo") {
		t.Fatalf("beacons missing in diagnostics body:\n%s", body)
	}
}

func TestTrack_EmptyProjectIDLogsError(t *testing.T) {
	ch, srv := newDiagServer(t)
	t.Setenv("DIAGNOSTICS_ENDPOINT", srv.URL)
	t.Setenv("DISABLE_DIAGNOSTICS", "false")
	mcpcat.ResetDiagnosticsForTest()
	t.Cleanup(mcpcat.ResetDiagnosticsForTest)

	s := server.NewMCPServer("test", "1.0.0")
	_, err := Track(s, "", &Options{})
	if err != mcpcat.ErrEmptyProjectID {
		t.Fatalf("err = %v, want ErrEmptyProjectID", err)
	}
	_ = mcpcat.Shutdown(context.Background())

	body := drain(ch)
	if !strings.Contains(body, "MCPCat setup failed") {
		t.Fatalf("expected setup-failed record:\n%s", body)
	}
}
```

> `server.NewMCPServer` is from `github.com/mark3labs/mcp-go/server`. If existing mcpgo tests construct the server differently, copy their pattern.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd mcpgo && go test -run 'TestTrack_EmitsSetupBeacons|TestTrack_EmptyProjectIDLogsError' -v; cd ..`
Expected: FAIL — `Options.DisableDiagnostics` unknown / no beacons.

- [ ] **Step 3: Add option and wire `Track` in `mcpgo/mcpgo.go`**

Add to the `Options` struct:

```go
	// DisableDiagnostics disables MCPCat's internal SDK diagnostics. On by default;
	// also disable via the DISABLE_DIAGNOSTICS env var. ~/mcpcat.log is unaffected.
	DisableDiagnostics bool
```

At the top of `Track`, resolve opts first, then init diagnostics before validation:

```go
	if opts == nil {
		opts = DefaultOptions()
	}

	mcpcat.InitDiagnostics(projectID, opts.DisableDiagnostics, "mcpgo",
		"github.com/mark3labs/mcp-go")

	if mcpServer == nil {
		mcpcat.LogSetupFailed("server must not be nil")
		return nil, mcpcat.ErrNilServer
	}
	if projectID == "" {
		mcpcat.LogSetupFailed("projectID must not be empty")
		return nil, mcpcat.ErrEmptyProjectID
	}
```

> Note: the existing mcpgo `Track` sets up `hooks` before the validation guards. Keep the hooks setup where it is, but ensure `InitDiagnostics` and the guard logging run before any early `return nil, err`. If the current order is `nil`-check → `projectID`-check → `opts==nil` default → hooks, move the `opts==nil` default and `InitDiagnostics` above the two checks as shown, and leave the hooks block after the checks.

In the `coreOpts` builder, add:

```go
		DisableDiagnostics:         opts.DisableDiagnostics,
```

Immediately before `return shutdownFn, nil`, add:

```go
	mcpcat.LogSetupComplete(projectID, coreOpts)
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd mcpgo && go test -race ./... -v; cd ..`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add mcpgo/mcpgo.go mcpgo/diagnostics_test.go
git commit -m "feat(mcpgo): wire diagnostics init, beacons, and setup-failure logging into Track"
```

---

### Task 11: README — Internal diagnostics section

**Files:**
- Modify: `README.md`

- [ ] **Step 1: Add the section**

Insert an `### Internal diagnostics` section (privacy-forward; lead with what we never collect; mention project id or anonymous install id; note `~/mcpcat.log` is unchanged; then opt-out). Do **not** mention the shared token, endpoint, or override env vars. Suggested wording:

```markdown
### Internal diagnostics

To help us catch and fix broken installs, the SDK sends MCPCat a small, anonymized
signal when setup or runtime errors occur — never your tool calls, your responses,
or anything about your users. Records carry only operational metadata, such as your
project ID (or an anonymous install ID when none is set), SDK version, and Go
runtime/OS/arch. Your local `~/mcpcat.log` is unchanged.

Diagnostics are on by default and can be turned off completely with either:

- `mcpcat.Options{DisableDiagnostics: true}` passed to `Track`, or
- the `DISABLE_DIAGNOSTICS` environment variable.
```

> Adjust the `mcpcat.Options` reference to match how the README's existing examples spell the options type for each adapter (`officialsdk.Options` / `mcpgo.Options`).

- [ ] **Step 2: Commit**

```bash
git add README.md
git commit -m "docs: add privacy-forward Internal diagnostics section"
```

---

### Task 12: Full verification

**Files:** none (verification only).

- [ ] **Step 1: Format check**

Run: `make fmt && gofmt -l .`
Expected: `gofmt -l .` prints nothing.

- [ ] **Step 2: Vet**

Run: `make vet`
Expected: no output / exit 0.

- [ ] **Step 3: Race tests, all three modules**

Run:
```bash
make test
cd officialsdk && go test -race ./... && cd ..
cd mcpgo && go test -race ./... && cd ..
```
Expected: all PASS.

- [ ] **Step 4: Smoke — not gated by debug**

Confirm `TestExport_PostsOTLPWithAuth` (Task 5) passes with the default `Debug=false`: the sink fires and a record is POSTed to the httptest endpoint, proving diagnostics are not gated by debug.

Run: `go test ./internal/diagnostics/ -race -run TestExport_PostsOTLPWithAuth -v`
Expected: PASS.

- [ ] **Step 5: Final commit (if any formatting changes)**

```bash
git add -A && git commit -m "chore: gofmt + verification for diagnostics feature" || echo "nothing to commit"
```

---

## Self-Review

**Spec coverage (against DIAGNOSTICS_PORT_PLAN.md):**
- §2 new `internal/diagnostics` package → Tasks 2–5. ✓
- §3 logging sink hook independent of debug → Task 1. ✓
- §4 constants → Task 2. ✓
- §5 `DisableDiagnostics` in three places + both mappings → Tasks 6 (core), 9 (officialsdk), 10 (mcpgo). ✓
- §6 facade `InitDiagnostics`/`LogSetupComplete` + Track wiring + setup-failure ERROR logging → Tasks 7, 9, 10. ✓
- §7 scrub `LogEvent` → Task 8. ✓
- §8 Shutdown flush → Task 7 (Step 5). ✓
- §9 README → Task 11. ✓
- §10 tests (sink, optout, record, attributes, export+auth, no-payload, beacons) → Tasks 1, 3, 4, 5, 8, 9, 10. ✓
- §11 verify → Task 12. ✓

**Decisions baked in (from brainstorming):** `deployment.environment` ← `ENVIRONMENT` only (Task 4); setup failures logged as ERROR (Tasks 9, 10 via `LogSetupFailed`); setup-complete beacon = `context` + `report_missing` only (Task 7); `InitDiagnostics` runs **before** validation guards so the ERROR is captured (Tasks 9, 10) — a deliberate refinement of plan §6's ordering. HTTP client has a 5s timeout (Task 5); install_id seed = hostname + executable path (Task 4); `timeUnixNano` = `time.Now().UnixNano()` decimal string (Task 3).

**Placeholder scan:** none — every code/test step shows full content.

**Type consistency:** `Level`/`LevelInfo|Warn|Error|Debug`, `SetDiagnosticsSink`, `SwapWriterForTest`, `Init`, `Flush`, `Enabled`, `ResetForTest`, `capture`, `buildRecord`, `severityFor`, `buildStaticAttributes`, `computeInstallID`, `InitDiagnostics`, `LogSetupComplete`, `LogSetupFailed`, `ResetDiagnosticsForTest`, `orTelemetryOnly`, `otlpPayload` and friends — all defined once and referenced consistently across tasks. ✓

**N/A vs the TS PR (do not implement):** no `Event details`/`model_dump_json` dump to remove (publisher logs id only); no Datadog/exporter scrub (exporters unimplemented); no text-based severity inference (levels are explicit). Server-side collector + WAF token gate already live in `MCPCat/mcpcat-server` — nothing to deploy here.
