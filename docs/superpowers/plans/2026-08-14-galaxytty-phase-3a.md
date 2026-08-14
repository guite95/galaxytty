# GalaxyTTY Phase 3-A Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Read real Samsung Galaxy SMS conversations and histories through ADB Content Providers in the existing GalaxyTTY CLI/TUI while making real sending impossible.

**Architecture:** Keep presentation above `app.Service` and domain ports. Add focused ADB, provider, read-only sender, bootstrap, and doctor packages; runtime discovery selects one physical Galaxy and injects its target into the provider store.

**Tech Stack:** Go 1.23 module syntax with local Go 1.26.6, `os/exec`, Android platform-tools 37.0.1, Bubble Tea/Bubbles/Lip Gloss, go-toml/v2, Go standard testing.

**Spec:** `docs/superpowers/specs/2026-08-14-galaxytty-phase-3a-design.md`

## Global Constraints

- Physical-device operations are read-only: no send intent, input event, clipboard command, provider mutation, pairing, unlock, wake, APK install, package mutation, or scrcpy launch.
- Never hardcode a real ADB serial or wireless endpoint; examples and fixtures use synthetic identifiers.
- Never save or print live bodies, phone numbers, or contact names during verification.
- `go test ./...` remains hardware-independent; real-device checks require the `integration` build tag and `GALAXYTTY_REAL_READ_TEST=1`.
- Use `exec.CommandContext`; do not introduce host `sh -c`.
- Add no third-party dependencies.
- Do not delete tests or weaken assertions.
- Do not commit or push unless requested separately.
- Complete behavior changes through RED, GREEN, and refactor cycles.

## File map

- `internal/domain`: sending error, status source, phone normalization.
- `internal/mock` and `internal/config`: Phase 2 regressions.
- `internal/adb`: subprocess execution, device parsing, target discovery.
- `internal/provider`: safe query construction and real MessageStore.
- `internal/readonly`: real sender rejection and no-op notifier.
- `internal/bootstrap`: mock/real assembly.
- `internal/doctor` and `internal/scrcpy`: privacy-safe diagnostics.
- `internal/cli` and `internal/tui`: presentation wiring and status.
- `internal/integration`: explicitly gated read-only physical-device test.
- `README.md` and `go.sum`: usage and dependency metadata.

---

### Task 1: Phase 2 leftovers and dependency metadata

**Files:**
- Create: `go.sum`
- Create: `internal/mock/mock_test.go`
- Modify: `internal/mock/mock.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/root_test.go`

**Interfaces:**
- Consumes: `domain.NormalizePhone`, `config.Path`, `config.Load`.
- Produces: `Config.Connection.Device string`; startup config loading; correct mock thread persistence.

- [ ] **Step 1: Generate dependency checksums**

```sh
go mod tidy
```

Expected: `go.sum` is created and `go.mod` keeps the same four direct dependencies.

- [ ] **Step 2: Write the failing mock regression**

```go
func TestSendStoresMessageInMatchingParticipantThread(t *testing.T) {
    backend := New()
    phone := backend.ConversationsData[1].Participants[0].Phone
    if err := backend.Send(context.Background(), phone, "synthetic hello"); err != nil {
        t.Fatal(err)
    }
    if got := backend.Sent[len(backend.Sent)-1].ThreadID; got != 2 {
        t.Fatalf("ThreadID = %d, want 2", got)
    }
}
```

Add config and CLI tests that load `device='synthetic-target'` and that place `unknown=1` under a temporary `XDG_CONFIG_HOME/galaxytty/config.toml` and require a `decode config` error from `doctor --mock`.

- [ ] **Step 3: Verify RED**

```sh
go test ./internal/mock ./internal/config ./internal/cli
```

Expected: ThreadID is 1, the Device field is absent, and invalid CLI startup config is ignored.

- [ ] **Step 4: Implement the fixes**

Add:

```go
Connection struct {
    PreferUSB bool   `toml:"prefer_usb"`
    Device    string `toml:"device"`
} `toml:"connection"`
```

At CLI startup, after help handling, call `config.Path()` then `config.Load(path)` and wrap both errors. Remove the later `config.Default()` call.

In `mock.Backend.Send`, normalize the requested phone and every conversation participant phone, select the matching conversation's ThreadID, and return `mock conversation for phone not found` if no match exists. Do not retain a thread-1 fallback.

- [ ] **Step 5: Verify GREEN**

```sh
gofmt -w internal/mock internal/config internal/cli
go test ./internal/mock ./internal/config ./internal/cli
go mod tidy
git diff --check
```

Expected: tests pass and a second tidy changes nothing.

### Task 2: Read-only sender and dynamic application status

**Files:**
- Create: `internal/domain/errors.go`
- Create: `internal/readonly/sender.go`
- Create: `internal/readonly/sender_test.go`
- Modify: `internal/domain/model.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/service_test.go`

**Interfaces:**
- Produces: `domain.ErrSendingNotImplemented`, `domain.StatusProvider`, and `(*app.Service).WithStatusProvider`.

- [ ] **Step 1: Write failing tests**

```go
func TestSenderAlwaysRejects(t *testing.T) {
    err := (Sender{}).Send(context.Background(), "synthetic", "synthetic")
    if !errors.Is(err, domain.ErrSendingNotImplemented) {
        t.Fatalf("err=%v", err)
    }
}

type fixedStatus struct{ value domain.ApplicationStatus }
func (f fixedStatus) Status(context.Context) domain.ApplicationStatus { return f.value }

func TestServiceUsesDynamicStatusProvider(t *testing.T) {
    service, _, _ := serviceFixture(NotificationPolicy{})
    service.WithStatusProvider(fixedStatus{domain.ApplicationStatus{
        State: "disconnected", Label: "Offline",
    }})
    got := service.Status(context.Background())
    if got.Label != "Offline" || got.State != "disconnected" {
        t.Fatalf("%+v", got)
    }
}
```

- [ ] **Step 2: Verify RED**

```sh
go test ./internal/readonly ./internal/app
```

Expected: missing package, sentinel, interface, and method failures.

- [ ] **Step 3: Implement minimal ports/adapters**

```go
var ErrSendingNotImplemented =
    errors.New("sending is not available yet (Phase 3-A read-only mode)")

type StatusProvider interface {
    Status(context.Context) ApplicationStatus
}
```

`readonly.Sender.Send` returns only that sentinel. Add a no-op notifier returning nil. Store an optional status provider in `app.Service`; `WithStatusProvider` sets it and returns the service. `Service.Status` uses the provider when present and preserves static mock behavior otherwise.

- [ ] **Step 4: Verify GREEN**

```sh
gofmt -w internal/domain internal/readonly internal/app
go test ./internal/readonly ./internal/app
```

### Task 3: ADB subprocess client

**Files:**
- Create: `internal/adb/errors.go`
- Create: `internal/adb/client.go`
- Create: `internal/adb/client_test.go`

**Interfaces:**
- Produces: `NewClient(path string, timeout time.Duration) (*Client, error)`, `Version`, `Shell`, and captured stdout/stderr.

- [ ] **Step 1: Write failing exact-argv and error tests**

Use a same-package recording runner and assert:

```go
want := []string{
    "-s", "synthetic-target", "shell",
    "getprop", "ro.product.manufacturer",
}
```

Cover missing executable, independent stderr, `context.Canceled`, `context.DeadlineExceeded`, unauthorized, and offline text.

- [ ] **Step 2: Verify RED**

```sh
go test ./internal/adb -run 'TestShell|TestClient|TestVersion'
```

- [ ] **Step 3: Implement the client**

```go
type runner interface {
    Run(context.Context, string, ...string) ([]byte, []byte, error)
}

type Client struct {
    path    string
    timeout time.Duration
    runner  runner
}
```

`NewClient` calls `exec.LookPath("adb")` when path is empty. `execRunner` uses `exec.CommandContext` and separate `bytes.Buffer` values. `Client.run` applies a timeout, checks `ctx.Err()`, and maps safe ADB state errors without provider row text. `Shell` runs `-s TARGET shell ARGS...` directly.

- [ ] **Step 4: Verify GREEN**

```sh
gofmt -w internal/adb
go test ./internal/adb
```

### Task 4: Device parser and Galaxy selection

**Files:**
- Create: `internal/adb/devices.go`
- Create: `internal/adb/devices_test.go`
- Create: `internal/adb/discovery.go`
- Create: `internal/adb/discovery_test.go`
- Modify: `internal/adb/client.go`

**Interfaces:**
- Produces: `ParseDevices`, `Client.Devices`, `Client.MDNSServices`, `Discover(ctx, Backend, SelectionOptions) (*Target, error)`; `Target` implements `domain.Device` and `domain.StatusProvider`.

- [ ] **Step 1: Write failing parser tests**

Use:

```text
List of devices attached
USB123 device usb:123 product:a37 model:SM_A376N transport_id:1
adb-USB123-token._adb-tls-connect._tcp device model:SM_A376N transport_id:2
192.0.2.10:37123 offline transport_id:3
OTHER unauthorized transport_id:4
```

Assert USB comes only from `usb:` metadata; the TLS suffix and valid host-port are Wireless; offline/unauthorized are preserved; an arbitrary colon token is not automatically Wireless.

- [ ] **Step 2: Write failing discovery tests**

Cover USB/Wireless dedupe using one synthetic hardware serial, `PreferUSB=false`, exact selector, two different physical Galaxy serials, no targets, unauthorized, offline, non-Samsung, and missing Samsung Messages.

- [ ] **Step 3: Verify RED**

```sh
go test ./internal/adb -run 'TestParse|TestClassify|TestDiscover'
```

- [ ] **Step 4: Implement parsing and discovery**

```go
type Device struct {
    Target     string
    State      string
    Metadata   map[string]string
    Connection domain.ConnectionKind
}

type Backend interface {
    Devices(context.Context) ([]Device, error)
    Shell(context.Context, string, ...string) ([]byte, error)
}

type SelectionOptions struct {
    PreferUSB bool
    Target    string
    Package   string
}
```

Parse metadata with `strings.SplitN(token, ":", 2)`. Probe authorized candidates for manufacturer, model, hardware serial, and `pm path com.samsung.android.messaging`. Group by hardware serial, reject multiple physical Galaxies, and apply connection preference inside one group.

`Target.Shell` delegates to the selected endpoint and updates a mutex-protected cached device state. `Target.Status` performs no subprocess call and returns USB, Wireless, or Offline.

- [ ] **Step 5: Verify GREEN**

```sh
gofmt -w internal/adb
go test ./internal/adb
```

### Task 5: Strict Content Provider query boundary

**Files:**
- Create: `internal/provider/errors.go`
- Create: `internal/provider/store.go`
- Create: `internal/provider/query.go`
- Create: `internal/provider/query_test.go`
- Modify: `internal/provider/parser_test.go`

**Interfaces:**
- Produces: `NewStore(Sheller) *Store`, private `Store.query`, and `Store.Probe`.

- [ ] **Step 1: Write failing construction/error tests**

Assert exact latest-ID argv:

```go
[]string{
    "content", "query", "--uri", "content://sms",
    "--projection", "_id",
    "--sort", "\"_id DESC LIMIT 1\"",
}
```

Also assert numeric selection is transmitted as `"_id > 42"`, `No result found.` is empty, blank output is malformed, `Permission Denial` maps to `ErrProviderPermissionDenied`, and `[ERROR] Unsupported argument` maps to `ErrProviderOutput` even with a nil shell error.

- [ ] **Step 2: Verify RED**

```sh
go test ./internal/provider -run 'TestQuery|TestProvider|TestNoResult'
```

- [ ] **Step 3: Implement the query helper**

```go
type Sheller interface {
    Shell(context.Context, ...string) ([]byte, error)
}
type Query struct {
    URI          string
    Projection   []string
    Where        string
    Sort         string
    FreeFormLast string
}
```

Build only `content query`. Reject empty URI/projection and newline, carriage return, backtick, dollar-sign, backslash, or double-quote in generated where/sort text before surrounding the complete remote value with literal double quotes. Check ADB error, permission text, `[ERROR]` text, no-result text, and then projected rows in that order. `Probe` accepts explicit non-sensitive columns and never includes values in returned errors.

- [ ] **Step 4: Verify GREEN**

```sh
gofmt -w internal/provider
go test ./internal/provider
```

### Task 6: SMS MessageStore

**Files:**
- Create: `internal/provider/sms.go`
- Create: `internal/provider/sms_test.go`

**Interfaces:**
- Produces: `Store.Messages`, `Store.MessagesAfter`, `Store.LatestMessageID`.

- [ ] **Step 1: Write failing row and method tests**

Use synthetic rows whose bodies contain commas, equals signs, Unicode, and a newline. Assert Unix milliseconds, types 1/2, read 0/1, `MessageSMS`, and strict numeric/type failures.

Assert these clauses:

```text
thread_id = 2 AND _id < 9
_id DESC LIMIT 200
_id DESC LIMIT 1000
_id DESC LIMIT 1
_id > 42
_id ASC LIMIT 500
```

Require history oldest-to-newest, latest zero for no rows, and incremental ascending order.

- [ ] **Step 2: Verify RED**

```sh
go test ./internal/provider -run 'TestMessages|TestLatest|TestSMS'
```

- [ ] **Step 3: Implement mapping and methods**

```go
var smsProjection = []string{
    "_id", "thread_id", "address", "date", "type", "read", "body",
}
```

Parse numeric fields with field-specific errors, use `time.UnixMilli`, map only observed types 1/2, and reject invalid read values. Default history limit is 200, cap is 1000, and after-ID batch is 500. Reverse descending history before returning it.

- [ ] **Step 4: Verify GREEN**

```sh
gofmt -w internal/provider
go test ./internal/provider
```

### Task 7: Conversations, canonical addresses, and contacts

**Files:**
- Create: `internal/provider/conversations.go`
- Create: `internal/provider/conversations_test.go`
- Modify: `internal/provider/store.go`
- Modify: `internal/domain/phone.go`
- Create: `internal/domain/phone_test.go`

**Interfaces:**
- Produces: `Store.Conversations` with all participants, contact/fallback titles, unread count, date, and snippet.

- [ ] **Step 1: Write failing normalization/recipient tests**

```go
tests := map[string]string{
    "010-1234-5678":       "01012345678",
    "+82 10-1234-5678":    "01012345678",
    "+82 (0)10-1234-5678": "01012345678",
    " 1588-0000 ":         "15880000",
}
```

Assert `parseRecipientIDs(" 12  34 ")` returns two IDs and non-numeric tokens fail.

- [ ] **Step 2: Write failing mapping/cache tests**

Feed synthetic conversation, two canonical-address, and contact rows. Require both participants, one display name, one address fallback, joined group title, unread/date/snippet mapping, `Unknown participant` for an absent canonical ID, and retry after a contact query error.

- [ ] **Step 3: Verify RED**

```sh
go test ./internal/domain ./internal/provider -run 'TestNormalize|TestRecipient|TestConversation|TestContact'
```

- [ ] **Step 4: Implement the mappings**

Query:

```text
content://mms-sms/conversations?simple=true
  _id:recipient_ids:unread_count:date:snippet
content://mms-sms/canonical-addresses
  _id:address
content://com.android.contacts/data/phones
  data1:data4:display_name
```

Use `strings.Fields`, canonical IDs as contact IDs, normalized `data1`/`data4` indexes, display-name fallback to address, and group titles joined by comma-space. Cache contacts only after a successful query using a mutex and boolean. Refresh conversations/canonicals only when `Conversations` is called. Correct the optional trunk zero in `+82 (0)10...`.

- [ ] **Step 5: Verify GREEN and race safety**

```sh
gofmt -w internal/domain internal/provider
go test ./internal/domain ./internal/provider
go test -race ./internal/provider
```

### Task 8: Runtime bootstrap

**Files:**
- Create: `internal/bootstrap/runtime.go`
- Create: `internal/bootstrap/runtime_test.go`

**Interfaces:**
- Produces: `Runtime{Service app.API}`, `Mock(config.Config)`, and `Real(context.Context, config.Config, string)`.
- Test seam: `realWithBackend(context.Context, config.Config, string, adb.Backend) (*Runtime, error)`.

- [ ] **Step 1: Write failing assembly tests**

Require mock conversations and sending to thread 2. Build real mode through a package-private synthetic backend and require status `USB`, an empty latest-ID polling baseline, and `ErrSendingNotImplemented` from sending.

- [ ] **Step 2: Verify RED**

```sh
go test ./internal/bootstrap
```

- [ ] **Step 3: Implement assembly**

`Mock` recreates current mock notifier/display/lifecycle wiring. `Real` creates the ADB client, applies CLI selector over config, discovers a target, builds `provider.Store`, `readonly.Sender`, no-op notifier, lifecycle, and `app.Service`, attaches the target status provider, and initializes polling.

Keep concrete adapters inside bootstrap. The public runtime exposes only `app.API`.

- [ ] **Step 4: Verify GREEN**

```sh
gofmt -w internal/bootstrap
go test ./internal/bootstrap ./internal/app ./internal/mock
```

### Task 9: Privacy-safe doctor and scrcpy inspection

**Files:**
- Create: `internal/doctor/report.go`
- Create: `internal/doctor/service.go`
- Create: `internal/doctor/service_test.go`
- Modify: `internal/scrcpy/display.go`
- Create: `internal/scrcpy/display_test.go`

**Interfaces:**
- Produces: `doctor.Run(ctx, cfg, selector) Report` and `scrcpy.Inspect(ctx, path)`.
- Test seam: `doctor.Service.Run(ctx, cfg, selector)` with injected `ADB`, discovery, provider-probe, and scrcpy-inspection functions.

- [ ] **Step 1: Write failing tests**

For scrcpy, require only `--version` and reject any test invocation containing display/device options.

For doctor, cover ready USB/Wireless, missing ADB, unauthorized/offline, missing Samsung Messages, SMS permission denial, optional MMS failure, and RCS informational state. Require doctor projections to exclude body, address, display_name, data1, data4, and snippet.

- [ ] **Step 2: Verify RED**

```sh
go test ./internal/doctor ./internal/scrcpy
```

- [ ] **Step 3: Implement reports/probes**

```go
type State string
const (
    Pass State = "pass"
    Info State = "info"
    Fail State = "fail"
)
type Check struct { Name, Detail string; State State }
type Report struct { Checks []Check; Ready bool; Summary string }

type ADB interface {
    adb.Backend
    Version(context.Context) (string, error)
    MDNSServices(context.Context) ([]adb.MDNSService, error)
}

type Service struct {
    ADB     ADB
    InspectScrcpy func(context.Context, string) (scrcpy.VersionInfo, error)
}
```

Required checks are ADB, selected Galaxy, Samsung Messages, SMS, conversations, and contacts. MMS, MMS parts, standard SMS-provider RCS columns, mDNS availability, and scrcpy are informational. Provider probes project `_id` only except the RCS structural column probe and discard all values. Summary is exactly `Read mode ready. Sending not implemented yet.` when required checks pass.

`scrcpy.Inspect` resolves the executable, runs `--version`, and parses only the version header.
The package-level `doctor.Run` constructs production dependencies and delegates to `Service.Run`; unit tests instantiate `Service` with synthetic dependencies.

- [ ] **Step 4: Verify GREEN**

```sh
gofmt -w internal/doctor internal/scrcpy
go test ./internal/doctor ./internal/scrcpy
```

### Task 10: Real CLI and TUI wiring

**Files:**
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/root_test.go`
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`
- Modify: `cmd/msg/main.go`

**Interfaces:**
- Produces: real default mode, `--device`, real doctor/read commands, JSON-only stdout, and USB/Wireless/Offline TUI status.

- [ ] **Step 1: Write failing CLI tests**

Through an unexported dependency seam, cover configured selector/CLI override, real conversations/unread/messages, valid JSON without diagnostic text, doctor formatting, and real send rejection before real runtime construction.

Use:

```go
type options struct {
    mock    bool
    json    bool
    device  string
    command []string
}
```

The send safety test fails if its real factory or doctor dependency is invoked and requires `errors.Is(err, domain.ErrSendingNotImplemented)`.

- [ ] **Step 2: Write failing TUI tests**

Cover provider error changing the label to Offline, USB/Wireless header text, polling error scheduling one normal next tick, and read-only send retaining composer text while displaying `Sending is not available yet.`.

- [ ] **Step 3: Verify RED**

```sh
go test ./internal/cli ./internal/tui
```

- [ ] **Step 4: Implement presentation changes**

Parse `--mock`, `--json`, and `--device TARGET` anywhere and reject missing values/unknown flags. Load config once after help. In real mode, reject `send` before constructing a runtime; run doctor separately; build real runtime for TUI/read commands. Preserve mock commands and send.

TUI refreshes `service.Status` after conversation/message/poll/send results. It clears the composer only after successful mock send, keeps it for the read-only error, and schedules `m.tick()` after poll failure without recursive polling.

- [ ] **Step 5: Verify GREEN and mock regressions**

```sh
gofmt -w internal/cli internal/tui cmd/msg
go test ./internal/cli ./internal/tui
go run ./cmd/msg conversations --mock >/dev/null
go run ./cmd/msg conversations --mock --json | jq -e 'type == "array"' >/dev/null
go run ./cmd/msg unread --mock >/dev/null
go run ./cmd/msg messages 1 --mock >/dev/null
go run ./cmd/msg doctor --mock >/dev/null
```

### Task 11: Gated real-device read test and optional provider truth

**Files:**
- Create: `internal/integration/real_read_test.go`
- Modify only if live facts demand it: `internal/doctor/service.go` and `internal/doctor/service_test.go`

**Interfaces:**
- Produces: opt-in physical read verification excluded from `go test ./...`.

- [ ] **Step 1: Add the gated integration test**

Start the file with `//go:build integration`. Skip unless `GALAXYTTY_REAL_READ_TEST=1`. Under a 30-second context, require doctor readiness, build real runtime, fetch conversations, fetch at most two SMS rows for the first thread, structurally validate thread/type, call `Poll` once, and require real send rejection. Never log model values.

- [ ] **Step 2: Prove default tests exclude hardware**

```sh
go test ./...
```

Expected: no ADB/device requirement.

- [ ] **Step 3: Run the opted-in read test**

```sh
GALAXYTTY_REAL_READ_TEST=1 go test -tags=integration ./internal/integration -run TestRealGalaxyReadPath -v
```

Expected: PASS without personal values or endpoint output.

- [ ] **Step 4: Observe lock state without changing it**

Resolve `target` from authorized `adb devices -l` output and run:

```sh
adb -s "$target" shell dumpsys power | rg 'mWakefulness|Display Power'
adb -s "$target" shell dumpsys window | rg 'mDreamingLockscreen|mShowingLockscreen|isStatusBarKeyguard'
```

Do not wake, unlock, send key events, or enter input regardless of state.

- [ ] **Step 5: Inspect optional provider structure with redaction**

Use one-row projections for MMS `_id:thread_id:date:msg_box:read`, MMS parts `_id:mid:ct`, and standard SMS-provider RCS extensions `_id:teleservice_id:app_id:chat_type:correlation_tag`. Reduce output locally to success, column presence, null/non-null booleans, and row count. Do not query restricted Samsung RCS URIs.

If live output differs, first add a synthetic failing doctor test containing only the structural error text, observe RED, then update the informational doctor mapping and rerun `go test ./internal/doctor`.

### Task 12: Documentation and final verification

**Files:**
- Modify: `README.md`
- Modify as formatting requires: changed Go files

**Interfaces:**
- Produces: safe Phase 3-A usage docs and final evidence.

- [ ] **Step 1: Update README**

Document ADB/already-authorized-device prerequisites, real-default and mock commands, `--device`, optional config, disabled real send, privacy behavior, MMS/RCS limits, and Phase 3-B deferrals. Use synthetic values only.

- [ ] **Step 2: Format and normalize dependencies**

```sh
gofmt -w cmd internal
go mod tidy
git diff --check
```

Expected: a second tidy is a no-op and no whitespace errors exist.

- [ ] **Step 3: Run all quality gates**

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/msg
```

Expected: every command exits 0.

- [ ] **Step 4: Run privacy-reduced real CLI smoke**

```sh
go run ./cmd/msg doctor
go run ./cmd/msg conversations --json | jq -e 'length >= 1' >/dev/null
thread_id=$(go run ./cmd/msg conversations --json | jq -r '.[0].ThreadID')
go run ./cmd/msg messages "$thread_id" --json | jq -e 'type == "array"' >/dev/null
go run ./cmd/msg unread --json | jq -e 'type == "array"' >/dev/null
```

Only doctor status and structural reducers may reach the transcript.

- [ ] **Step 5: Prove real sending is disabled**

```sh
if go run ./cmd/msg send --to synthetic --text synthetic 2>send-error.txt; then exit 1; fi
rg -q 'sending is not available yet' send-error.txt
rm send-error.txt
```

Expected: non-zero exit and no real runtime/ADB invocation.

- [ ] **Step 6: Re-run mock CLI coverage**

```sh
go run ./cmd/msg conversations --mock >/dev/null
go run ./cmd/msg conversations --mock --json | jq -e 'type == "array"' >/dev/null
go run ./cmd/msg unread --mock >/dev/null
go run ./cmd/msg messages 1 --mock >/dev/null
go run ./cmd/msg doctor --mock >/dev/null
```

Keyboard navigation, mock send, Esc, `/exit`, `/quit`, and Ctrl+C remain covered by `internal/tui/model_test.go`.

- [ ] **Step 7: Inspect final workspace scope/privacy**

```sh
git status --short
git diff --stat
git diff --check
rg -n 'RFKL|content query.*body=' . --glob '!**/*_test.go' || true
```

Expected: only planned source/tests/docs and `go.sum` changed; no real serial, dump, screenshot, ARIA snapshot, or temporary smoke file remains.
