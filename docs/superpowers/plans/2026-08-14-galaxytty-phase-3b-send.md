# GalaxyTTY Phase 3-B Real Send Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Send one-to-one Unicode text through Samsung Messages on a reusable scrcpy virtual display and report success only after an exact outgoing SMS Provider row is observed.

**Architecture:** Preserve the existing CLI/TUI → application → domain-port flow. `bootstrap.Real` assembles one selected ADB target into `provider.Store`, `scrcpy.Manager`, `samsung.Controller`, `clipboard.Mac`, and `samsung.Sender`; the sender owns sequencing and verification while presentation layers receive only `domain.SendResult`.

**Tech Stack:** Go 1.22+, `os/exec`, public adb and scrcpy 4.1 CLIs, macOS `pbcopy`/`pbpaste`, Bubble Tea, Android SMS Content Provider.

**Spec:** `docs/superpowers/specs/2026-08-14-galaxytty-phase-3b-send-design.md`

## Global Constraints

- Samsung Messages `com.samsung.android.messaging` is the only message transport owner.
- Do not use Android SMS APIs, `SEND_SMS` workarounds, helper APKs, UIAutomator selectors, `adb input text`, or scrcpy private protocols.
- Never wake, unlock, or enter a PIN on the main display.
- Reuse the selected Phase 3-A ADB target; never hardcode a serial, executable path, display ID, recipient, or body.
- Pass subprocess arguments directly; never use `sh -c`.
- Do not log or commit actual recipient, message body, clipboard contents, provider rows, or contact data.
- Keep group and attachment sending unsupported.
- Keep default unit tests hardware-independent; actual send requires the integration build tag and both explicit environment variables.
- Write every production behavior test-first and observe the intended RED failure before implementation.
- End each task with fresh tests and a focused commit; do not push.
- Do not add a `Co-Authored-By` trailer or generated-by footer.

---

### Task 1: scrcpy virtual display lifecycle

**Files:**
- Modify: `internal/scrcpy/display.go`
- Modify: `internal/scrcpy/display_test.go`
- Create: `internal/scrcpy/manager.go`
- Create: `internal/scrcpy/manager_test.go`
- Modify: `internal/domain/errors.go`

**Interfaces:**
- Consumes: selected target string, configured width/height, `context.Context`.
- Produces: `scrcpy.NewManager(Config) (*Manager, error)` implementing `domain.VirtualDisplayManager`; `BuildArgs(Config, recordPath) []string`.

- [ ] **Step 1: Add parser and command-construction tests**

Add table tests proving both stdout/stderr-style log prefixes parse `id=18`, unrelated numeric IDs fail, and command args exactly contain:

```go
[]string{
    "-s", "synthetic-target",
    "--new-display=1080x1920",
    "--start-app=com.samsung.android.messaging",
    "--record=/synthetic/record.mp4",
    "--no-video-playback",
    "--no-audio",
}
```

Run: `go test ./internal/scrcpy -run 'Test(ParseDisplayID|BuildArgs)' -count=1`
Expected: FAIL because `BuildArgs` and the stricter parser contract do not exist.

- [ ] **Step 2: Implement config, args, and recognizable errors**

Use these production shapes:

```go
type Config struct {
    Path, Target, Package string
    Width, Height         int
    StartupTimeout        time.Duration
}

func BuildArgs(cfg Config, recordPath string) []string
func NewManager(cfg Config) (*Manager, error)
```

`NewManager` validates static values only. Resolve `cfg.Path` with
`exec.LookPath` inside the first `Start`, never during read-only runtime
construction.

Add errors wrapping `ErrScrcpyNotFound`, `ErrScrcpyStartup`,
`ErrVirtualDisplayIDNotFound`, and `ErrScrcpyExited` without target data.

Run: `go test ./internal/scrcpy -run 'Test(ParseDisplayID|BuildArgs)' -count=1`
Expected: PASS.

- [ ] **Step 3: Add process lifecycle RED tests**

Inject a package-private process starter returning a fake with log reader,
`Wait`, `Signal`, and `Kill`. Test:

```go
display, err := manager.Start(ctx)
// display.AndroidDisplayID == 18; width/height match config
// a second Start returns the same display and startCount remains 1
// Healthy is true until fake completion closes
// Stop twice signals/reaps once and removes the injected temp path
```

Also test timeout, process exit before ID, context cancellation, unexpected
exit, and cleanup after every failed start.

Run: `go test ./internal/scrcpy -run TestManager -count=1`
Expected: FAIL because `Manager.Start`, `Stop`, and `Healthy` are not implemented.

- [ ] **Step 4: Implement synchronized Start, Healthy, and Stop**

Implement a mutex-protected session containing the command process, parsed
`domain.VirtualDisplay`, done channel, and record path. Route stdout and stderr
through one pipe, scan until `ParseDisplayID` succeeds, and run exactly one
background waiter. Detach state before termination in `Stop`; wait within the
caller context and kill only when graceful termination cannot finish.

Run: `go test ./internal/scrcpy -run TestManager -count=1`
Expected: PASS.

- [ ] **Step 5: Refactor and verify the package**

Run: `gofmt -w internal/scrcpy internal/domain/errors.go`

Run: `go test -race ./internal/scrcpy ./internal/domain -count=1`
Expected: PASS with no race report.

- [ ] **Step 6: Commit the lifecycle**

```bash
git add internal/scrcpy internal/domain/errors.go
git diff --cached --check
git commit -m "feat: manage scrcpy virtual displays"
```

---

### Task 2: Samsung controller and macOS clipboard adapters

**Files:**
- Modify: `internal/samsung/layout.go`
- Create: `internal/samsung/controller.go`
- Create: `internal/samsung/controller_test.go`
- Create: `internal/clipboard/macos.go`
- Create: `internal/clipboard/macos_test.go`
- Modify: `internal/domain/model.go`
- Modify: `internal/domain/errors.go`

**Interfaces:**
- Consumes: `domain.Device`, `domain.VirtualDisplay`, `samsung.Layout`, and direct host commands.
- Produces: `samsung.NewController(domain.Device, Layout) *Controller`; methods `OpenConversation`, `FocusComposer`, `Paste`, and `TapSend`; `clipboard.New(pathCopy, pathPaste string) *Mac` implementing `domain.Clipboard.Read/Set`.

- [ ] **Step 1: Add exact Samsung command RED tests**

With a recording fake device and display ID 18, assert these calls in order:

```go
[]string{"am", "start", "--display", "18", "-a", "android.intent.action.SENDTO", "-d", "smsto:01012345678"}
[]string{"input", "-d", "18", "tap", "500", "1800"}
[]string{"input", "-d", "18", "keyevent", "279"}
[]string{"input", "-d", "18", "tap", "1004", "1273"}
```

Assert a failing device returns operation names but neither the synthetic phone
nor body. Assert zero display IDs and invalid coordinates fail before ADB.

Run: `go test ./internal/samsung -run TestController -count=1`
Expected: FAIL because the controller does not exist.

- [ ] **Step 2: Implement Samsung controller and layout validation**

Use direct `device.Shell(ctx, args...)` calls and decimal conversion with
`strconv.FormatInt`/`strconv.Itoa`. Add:

```go
func (l Layout) Validate() error
func NewController(device domain.Device, layout Layout) (*Controller, error)
func (c *Controller) OpenConversation(context.Context, domain.VirtualDisplay, string) error
func (c *Controller) FocusComposer(context.Context, domain.VirtualDisplay) error
func (c *Controller) Paste(context.Context, domain.VirtualDisplay) error
func (c *Controller) TapSend(context.Context, domain.VirtualDisplay) error
```

Run: `go test ./internal/samsung -run TestController -count=1`
Expected: PASS.

- [ ] **Step 3: Add clipboard RED tests**

Inject a runner whose `Run` accepts argv and optional stdin. Verify:

```go
old, err := clip.Read(ctx)       // runs pbpaste, returns exact Unicode bytes
err = clip.Set(ctx, "안녕 😀") // runs pbcopy with exact stdin, no argv text
```

Test missing executables at first `Read`/`Set`, read failure, set failure, empty
clipboard, and errors that do not contain clipboard content.

Run: `go test ./internal/clipboard -count=1`
Expected: FAIL because the package does not exist.

- [ ] **Step 4: Implement the clipboard port and adapter**

Change the unused Phase 3-A port to:

```go
type Clipboard interface {
    Read(context.Context) (string, error)
    Set(context.Context, string) error
}
```

Store default command names in the constructor without resolving them. Use
`exec.CommandContext`, `bytes.NewBufferString` for stdin, and safe errors
`ErrClipboardRead`/`ErrClipboardSet`; map `exec.ErrNotFound` when the first send
actually invokes a missing tool. This keeps all real read commands independent
of send-only host prerequisites.

Run: `go test ./internal/clipboard -count=1`
Expected: PASS.

- [ ] **Step 5: Verify and commit adapters**

Run: `gofmt -w internal/samsung internal/clipboard internal/domain`

Run: `go test -race ./internal/samsung ./internal/clipboard ./internal/domain -count=1`
Expected: PASS.

```bash
git add internal/samsung internal/clipboard internal/domain
git diff --cached --check
git commit -m "feat: control Samsung Messages input"
```

---

### Task 3: Verified Samsung sender and application result

**Files:**
- Create: `internal/samsung/sender.go`
- Create: `internal/samsung/sender_test.go`
- Create: `internal/samsung/verify.go`
- Create: `internal/samsung/verify_test.go`
- Modify: `internal/domain/model.go`
- Modify: `internal/domain/errors.go`
- Modify: `internal/app/service.go`
- Modify: `internal/app/service_test.go`
- Modify: `internal/mock/mock.go`
- Modify: `internal/mock/mock_test.go`
- Modify: `internal/readonly/sender.go`
- Modify: `internal/readonly/sender_test.go`

**Interfaces:**
- Consumes: `domain.VirtualDisplayManager`, a local `MessageController` interface, `domain.Clipboard`, `domain.MessageStore`, and `SenderConfig` timings.
- Produces: `samsung.NewSender(...) *Sender` implementing `domain.MessageSender`; `domain.SendResult`; result-returning application send methods.

- [ ] **Step 1: Add exact verification RED tests**

Create a fake store with scripted `MessagesAfter` batches and test that
`verifySent` accepts only:

```go
domain.Message{
    ID: 101, ThreadID: 49, Address: "+82 10-1234-5678",
    Body: "안녕하세요 😀", Direction: domain.DirectionOutgoing,
}
```

for normalized target `01012345678` and identical body. Separate tests must
ignore an incoming self-echo, wrong recipient, wrong body, pre-baseline ID, and
unrelated duplicates; assert provider error propagation, context cancellation,
and `errors.Is(err, domain.ErrSendVerificationTimeout)`.

Run: `go test ./internal/samsung -run TestVerify -count=1`
Expected: FAIL because verification does not exist.

- [ ] **Step 2: Implement exact polling verification**

Use:

```go
type SendResult struct {
    MessageID int64 `json:"message_id"`
    ThreadID  int64 `json:"thread_id"`
}

func verifySent(ctx context.Context, store domain.MessageStore, baseline int64,
    recipient, body string, timeout, interval time.Duration) (domain.SendResult, error)
```

Poll immediately once, then with a timer. Never include recipient or body in
the timeout error.

Run: `go test ./internal/samsung -run TestVerify -count=1`
Expected: PASS.

- [ ] **Step 3: Add sender sequence and cleanup RED tests**

Record calls from fake display/controller/clipboard/store and assert success
order:

```text
display.start -> conversation.open -> wait.ready -> composer.tap ->
clipboard.read -> clipboard.set -> wait.sync -> paste -> wait.settle ->
store.latest -> send.tap -> store.after -> clipboard.restore
```

Assert the verified IDs are returned. Add a table where each operation fails
and confirm later operations are absent. Test display stop after controller
failure, display reuse across two sends, restore failure precedence, primary
error plus restore cleanup error, and no body/recipient in formatted errors.

Run: `go test ./internal/samsung -run TestSender -count=1`
Expected: FAIL because `Sender` does not exist.

- [ ] **Step 4: Implement minimal sender orchestration**

Use these shapes:

```go
type MessageController interface {
    OpenConversation(context.Context, domain.VirtualDisplay, string) error
    FocusComposer(context.Context, domain.VirtualDisplay) error
    Paste(context.Context, domain.VirtualDisplay) error
    TapSend(context.Context, domain.VirtualDisplay) error
}

type SenderConfig struct {
    ConversationReadyDelay time.Duration
    ClipboardSyncDelay     time.Duration
    SendSettleDelay        time.Duration
    VerificationTimeout    time.Duration
    VerificationInterval   time.Duration
}
```

Keep `Send` serialized with a mutex so CLI/TUI cannot overlap clipboard and
Samsung UI transactions. Restore the clipboard in one deferred cleanup and use
`errors.Join` only when a primary error already exists.

Run: `go test ./internal/samsung -run TestSender -count=1`
Expected: PASS.

- [ ] **Step 5: Change application and mock send signatures test-first**

Change:

```go
type MessageSender interface {
    Send(context.Context, string, string) (SendResult, error)
}

SendToAddress(context.Context, string, string) (SendResult, error)
SendToConversation(context.Context, int64, string) (SendResult, error)
```

Update application tests to assert mock IDs/thread IDs and group rejection
before sender invocation. Update mock/readonly tests to compile against and
exercise the result contract.

Run: `go test ./internal/app ./internal/mock ./internal/readonly -count=1`
Expected first run: FAIL from the old signatures. After minimal changes: PASS.

- [ ] **Step 6: Verify and commit verified sending**

Run: `gofmt -w internal/samsung internal/domain internal/app internal/mock internal/readonly`

Run: `go test -race ./internal/samsung ./internal/app ./internal/mock ./internal/readonly -count=1`
Expected: PASS.

```bash
git add internal/samsung internal/domain internal/app internal/mock internal/readonly
git diff --cached --check
git commit -m "feat: verify Samsung Messages sends"
```

---

### Task 4: Configuration, runtime, lifecycle, and CLI integration

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/bootstrap/runtime.go`
- Modify: `internal/bootstrap/runtime_test.go`
- Modify: `internal/app/lifecycle.go`
- Modify: `internal/core_test.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/root_test.go`

**Interfaces:**
- Consumes: all Task 1–3 adapters and existing `adb.Discover` target.
- Produces: lazy real runtime send wiring; validated Samsung config; plain/JSON send outputs; lifecycle-owned display cleanup.

- [ ] **Step 1: Add config RED tests**

Assert defaults:

```go
ClipboardSyncDelay = 300 * time.Millisecond
SendSettleDelay = 200 * time.Millisecond
VerificationTimeout = 10 * time.Second
```

Parse custom TOML durations. Reject zero/negative durations, non-positive
display dimensions, negative or out-of-bounds coordinates through
`Config.Validate()`.

Run: `go test ./internal/config -count=1`
Expected: FAIL because timing fields and validation do not exist.

- [ ] **Step 2: Implement focused timing config and validation**

Add the three `Duration` fields under `[samsung]`. Call `Validate` after defaults
are merged and TOML is decoded. Keep startup timeout, conversation-ready delay,
and provider poll interval internal constants.

Run: `go test ./internal/config -count=1`
Expected: PASS.

- [ ] **Step 3: Add bootstrap/lifecycle RED tests**

Extend the synthetic ADB fake for Samsung controller commands and inject
factories for scrcpy/clipboard so tests do not start host processes. Assert:

- `realWithBackend` creates a sender that can return a verified synthetic row;
- constructing the runtime does not call display `Start`;
- the selected target is used by provider and controller;
- `Shutdown` stops the display exactly once, including repeated calls.

Run: `go test ./internal/bootstrap ./internal/app -count=1`
Expected: FAIL while real mode still wires `readonly.Sender` and no display.

- [ ] **Step 4: Wire the real adapter graph lazily**

Build layout from config, create the scrcpy manager with `target.Info().Serial`,
create the clipboard adapter, controller, provider store, and Samsung sender,
then pass the same display manager to `app.NewLifecycle`. Do not call
`display.Start` in bootstrap or doctor, and do not resolve scrcpy/pbcopy/pbpaste
during runtime construction.

Use injectable package-level constructors only inside tests; production
defaults must use real adapters.

Run: `go test ./internal/bootstrap ./internal/app -count=1`
Expected: PASS.

- [ ] **Step 5: Add CLI RED tests**

Replace the Phase 3-A rejection test with:

```go
// real runtime factory is called once
// service.SendToAddress returns {MessageID: 12345, ThreadID: 49}
// --json output decodes to success=true and both IDs
// plain output contains "Message sent" but no recipient/body
```

Keep JSON stdout empty on runtime or send failure. Add actionable error tests
for scrcpy missing, clipboard failure, and verification timeout.

Run: `go test ./internal/cli -run 'Test(RealSend|SendJSON|SendError)' -count=1`
Expected: FAIL because real send is guarded and no output result exists.

- [ ] **Step 6: Enable CLI send through app.Service**

Remove only the early real-send guard. Add a private response:

```go
type sendResponse struct {
    Success   bool  `json:"success"`
    MessageID int64 `json:"message_id"`
    ThreadID  int64 `json:"thread_id"`
}
```

Require non-empty `--to` and `--text`, call the service, and print either the
JSON object or `Message sent.`. Extend `actionableRealError` without including
command args, recipient, or body.

Run: `go test ./internal/cli -count=1`
Expected: PASS.

- [ ] **Step 7: Verify and commit runtime/CLI integration**

Run: `gofmt -w internal/config internal/bootstrap internal/app internal/cli internal/core_test.go`

Run: `go test -race ./internal/config ./internal/bootstrap ./internal/app ./internal/cli ./internal -count=1`
Expected: PASS.

```bash
git add internal/config internal/bootstrap internal/app internal/cli internal/core_test.go
git diff --cached --check
git commit -m "feat: enable real CLI message sending"
```

---

### Task 5: TUI duplicate suppression and refresh

**Files:**
- Modify: `internal/tui/model.go`
- Modify: `internal/tui/model_test.go`

**Interfaces:**
- Consumes: result-returning `app.API.SendToConversation`.
- Produces: one in-flight TUI send, status/error transitions, success reloads, and failure text preservation.

- [ ] **Step 1: Add TUI send-state RED tests**

Use a controllable fake `app.API` whose send command completes only when the
test invokes it. Assert:

- first Enter sets `sending=true`, status `Sending…`, and returns one command;
- second Enter while sending returns no command;
- `sentMsg{result, nil}` clears composer/error and returns a batch containing
  conversation and selected-message reloads;
- `sentMsg{err}` leaves the exact composer value and renders the error;
- group error leaves text and clears `sending`;
- successful history and conversation messages restore normal status.

Run: `go test ./internal/tui -run 'Test(Send|Duplicate|Composer|Group)' -count=1`
Expected: FAIL because there is no `sending` state and `sentMsg` has no result.

- [ ] **Step 2: Implement the Bubble Tea state transitions**

Add `sending bool`, include result in `sentMsg`, ignore Enter while true, and
clear stale errors when a send starts. On success return:

```go
tea.Batch(m.loadConversations(), m.loadMessages(m.selectedID()))
```

Do not clear composer on failure. Show `Sending…` without replacing the live
connection label after completion.

Run: `go test ./internal/tui -count=1`
Expected: PASS.

- [ ] **Step 3: Verify and commit TUI behavior**

Run: `gofmt -w internal/tui`

Run: `go test -race ./internal/tui ./internal/app -count=1`
Expected: PASS.

```bash
git add internal/tui
git diff --cached --check
git commit -m "feat: enable safe TUI message sending"
```

---

### Task 6: Doctor send readiness

**Files:**
- Modify: `internal/doctor/report.go`
- Modify: `internal/doctor/service.go`
- Modify: `internal/doctor/service_test.go`
- Modify: `internal/cli/root.go`
- Modify: `internal/cli/root_test.go`

**Interfaces:**
- Consumes: existing ADB/provider/scrcpy inspections plus `exec.LookPath` for `pbcopy` and `pbpaste`.
- Produces: independent `ReadReady` and `SendReady` report fields and non-mutating CLI readiness output.

- [ ] **Step 1: Add readiness RED tests**

Assert a fully healthy fake report has both readiness fields true and summary
containing `Read: ready` and `Send: ready`. Assert missing scrcpy or either
clipboard command keeps `ReadReady=true` but sets `SendReady=false`. Assert all
fake ADB calls remain discovery, package inspection, or `content` probes and no
call starts with `am`, `input`, or scrcpy startup.

Run: `go test ./internal/doctor -count=1`
Expected: FAIL because readiness is a single `Ready` boolean and clipboard
prerequisites are absent.

- [ ] **Step 2: Implement split readiness without mutations**

Add:

```go
type Report struct {
    Checks                 []Check
    ReadReady, SendReady   bool
    Ready                  bool // compatibility alias for ReadReady
    Summary                string
}
```

Inject `LookPath func(string) (string, error)` into `doctor.Service`. Treat
scrcpy and clipboard as send requirements, not read requirements. Validate the
layout from config without constructing a controller or display.

Run: `go test ./internal/doctor -count=1`
Expected: PASS.

- [ ] **Step 3: Update doctor CLI formatting and tests**

Print each check and the two-line readiness result. Continue returning a CLI
error only when read readiness fails so Phase 3-A read diagnostics stay usable
when optional send tooling is absent.

Run: `go test ./internal/cli -run TestDoctor -count=1`
Expected first run: FAIL on the old summary; after formatting changes: PASS.

- [ ] **Step 4: Verify and commit doctor behavior**

Run: `gofmt -w internal/doctor internal/cli`

Run: `go test -race ./internal/doctor ./internal/cli -count=1`
Expected: PASS.

```bash
git add internal/doctor internal/cli
git diff --cached --check
git commit -m "feat: report real send readiness"
```

---

### Task 7: Integration gates, documentation, and full verification

**Files:**
- Modify: `internal/integration/real_read_test.go`
- Create: `internal/integration/real_send_test.go`
- Modify: `README.md`

**Interfaces:**
- Consumes: public runtime factories, selected target discovery, the two actual-send opt-in variables, and read-only `dumpsys` commands.
- Produces: explicit virtual-display smoke and actual-send commands, privacy-safe test output, Phase 3-B user documentation.

- [ ] **Step 1: Update the Phase 3-A integration expectation**

Remove the assertion that real sending is unimplemented. Retain every read-only
structural assertion and no-sensitive-output guarantee.

Run: `go test -tags=integration ./internal/integration -run TestRealGalaxyReadPath -count=1`
Expected: SKIP without `GALAXYTTY_REAL_READ_TEST=1`, demonstrating the old send
assertion no longer blocks integration compilation.

- [ ] **Step 2: Add explicit real-display and send integration tests**

Create `real_send_test.go` with `//go:build integration` and two tests:

```go
func TestRealVirtualDisplaySmoke(t *testing.T)
func TestRealSamsungSend(t *testing.T)
```

The display smoke test requires `GALAXYTTY_REAL_DISPLAY_TEST=1`, discovers the
selected target, starts the production manager, asserts positive logical ID and
healthy state, optionally opens only the explicit recipient conversation, then
stops and asserts unhealthy state.

The send test begins with:

```go
if os.Getenv("GALAXYTTY_ENABLE_SEND_TEST") != "1" ||
    strings.TrimSpace(os.Getenv("GALAXYTTY_TEST_RECIPIENT")) == "" {
    t.Skip("explicit real send test opt-in and recipient required")
}
```

Generate the body from a fixed label plus UTC timestamp, call
`runtime.Service.SendToAddress`, assert positive IDs, and never log the body or
recipient. Read lock/display state before and after through read-only dumpsys
helpers and fail if an already locked/off main display becomes unlocked/on.

Run: `go test -tags=integration ./internal/integration -run 'TestReal(VirtualDisplaySmoke|SamsungSend)' -count=1`
Expected: both tests SKIP without their gates.

- [ ] **Step 3: Update README for the real send architecture and safety**

Document prerequisites, lazy virtual display behavior, Unicode clipboard
handling, CLI JSON, TUI duplicate suppression, config defaults, doctor read/send
readiness, integration commands, exact two-variable gate, and known limitations.
Remove Phase 3-A read-only warnings and deferred Phase 3-B list without exposing
the live target or recipient.

- [ ] **Step 4: Run formatting and dependency cleanup**

Run: `gofmt -w cmd internal`

Run: `go mod tidy`

Run: `git diff --check`
Expected: all commands exit 0.

- [ ] **Step 5: Run the complete hardware-independent gate**

Run each command separately:

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/msg
```

Expected: all exit 0 with no test failures or race/vet/build errors.

- [ ] **Step 6: Run read and virtual-display smoke tests**

Run the Phase 3-A read integration with its explicit read gate. Then run the
new virtual-display smoke gate, confirm a runtime logical ID is parsed, stop the
manager, and confirm no temporary recording or scrcpy child remains. Record
only pass/fail, model, connection kind, and lock/off booleans.

- [ ] **Step 7: Run the explicitly approved actual-send test once**

Pass both approved environment variables only to this command:

```bash
GALAXYTTY_ENABLE_SEND_TEST=1 \
GALAXYTTY_TEST_RECIPIENT='<explicit-value>' \
go test -tags=integration ./internal/integration -run TestRealSamsungSend -count=1 -v
```

Expected: PASS with a verified positive outgoing provider row. If it fails,
inspect current non-sensitive scrcpy/ADB/provider behavior, write a failing
regression test, apply the minimum fix, and rerun the full hardware-independent
gate before another actual send. Do not blindly tap Send more than once.

- [ ] **Step 8: Perform one CLI send smoke only if the integration send passed**

Use a new clearly labeled timestamp body and the same explicit recipient, then
run real `msg send --json`. Decode only `success`, `message_id`, and `thread_id`;
do not print recipient or body. Do not perform a second TUI send automatically;
TUI behavior is covered by deterministic tests because interactive automation
would create another real message.

- [ ] **Step 9: Confirm shutdown and privacy state**

Run read-only process/temp-file checks, `git status --short`, and a repository
search for the explicit recipient and generated live bodies. Remove only
GalaxyTTY-created test artifacts. Confirm no actual values appear in tracked or
untracked files.

- [ ] **Step 10: Commit integration safety and docs**

```bash
git add internal/integration README.md
git diff --cached --check
git commit -m "test: verify opt-in real Samsung sending"
```

- [ ] **Step 11: Run the final completion gate after the last commit**

Run:

```bash
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/msg
git status --short --branch
git log --oneline --decorate -10
```

Expected: all Go commands exit 0, the worktree is clean on
`feat/phase-3b-send`, and the functional commits are present locally without a
push.
