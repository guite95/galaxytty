# GalaxyTTY Phase 3-C Stabilization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stabilize GalaxyTTY read history, TUI scrolling, and the real Samsung Messages send path without weakening exact provider verification or privacy.

**Architecture:** Preserve the existing TUI/CLI → application → domain ports → adapters layering. Add only small adapter-level values and queries: SMS row classification, a TUI offset viewport, Samsung role checking, explicit input mode, mode-specific scrcpy arguments, and stdin-delivered public Android intents.

**Tech Stack:** Go 1.25, Bubble Tea/Bubbles, Android ADB Content Provider and role commands, scrcpy 4.1, table-driven unit tests, build-tagged integration tests.

**Spec:** `docs/superpowers/specs/2026-08-14-galaxytty-phase-3c-stabilization-design.md`

## Global Constraints

- Samsung Messages package is exactly `com.samsung.android.messaging`.
- Text input modes are exactly `intent_body` and `clipboard`; default is `intent_body`.
- Android SMS history types are exactly inbox `1` and sent `2`; types `3` through `6` and unknown values are skipped.
- Default tests never start scrcpy or send a real message.
- Actual SMS, RCS, and MMS tests require their own explicit enable flag and recipient variable.
- No recipient or body appears in normal errors, logs, docs, commits, or host process argv.
- Do not add helper APKs, private protocols, role mutation, PIN entry, main-display unlock, or Unicode `adb input text`.
- Make local functional commits without Co-Authored-By trailers and do not push.

---

### Task 1: Robust SMS history classification

**Files:**
- Modify: `internal/provider/parser.go`
- Modify: `internal/provider/sms.go`
- Test: `internal/provider/sms_test.go`
- Test: `internal/app/poller_test.go`

**Interfaces:**
- Consumes: Android provider rows with a numeric `type` column.
- Produces: supported `domain.Message` values only; `Messages`, `MessagesAfter`, and `LatestMessageID` select history types `1,2`.

- [ ] **Step 1: Write failing mixed-row and query tests**

```go
func TestMapSMSRowsSkipsNonHistoryAndUnknownTypes(t *testing.T) {
    rows := []map[string]string{
        smsRow("10", "1", "0"),
        smsRow("11", "3", "1"),
        smsRow("12", "99", "1"),
        smsRow("13", "2", "1"),
    }
    messages, err := mapSMSRows(rows)
    if err != nil || len(messages) != 2 || messages[0].ID != 10 || messages[1].ID != 13 {
        t.Fatalf("messages=%+v err=%v", messages, err)
    }
}
```

Assert history/incremental/latest where clauses contain `type IN (1,2)` and
supported malformed rows still fail.

- [ ] **Step 2: Run the provider and poller tests and confirm RED**

Run: `go test ./internal/provider ./internal/app -run 'SMS|Poll' -count=1`

Expected: FAIL because type `3`/unknown currently abort mapping and queries do not filter types.

- [ ] **Step 3: Implement the smallest row classifier**

```go
func DirectionFromAndroid(v int) (uint8, bool) {
    switch v {
    case 1, 2:
        return uint8(v), true
    default:
        return 0, false
    }
}
```

Parse `type` before the remaining row fields, skip unsupported rows in
`mapSMSRows`, and append `type IN (1,2)` to all history cursor queries.

- [ ] **Step 4: Run focused and package tests and confirm GREEN**

Run: `go test ./internal/provider ./internal/app -count=1`

- [ ] **Step 5: Commit the provider unit**

```sh
git add internal/provider internal/app
git commit -m "fix: tolerate non-history SMS rows"
```

### Task 2: TUI text keys and scrolling viewport

**Files:**
- Modify: `internal/tui/model.go`
- Test: `internal/tui/model_test.go`

**Interfaces:**
- Consumes: `MessageQuery{BeforeID, Limit}`, poll batches, Bubble Tea PageUp/PageDown/End/window messages.
- Produces: de-duplicated loaded history and an offset-from-bottom viewport.

- [ ] **Step 1: Write failing input and viewport tests**

Add tests proving:

```go
model = typeText(model, "j/k q")
// composer retains the exact text and cursor did not move.

updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyPgUp})
// offset increases, and at the loaded top cmd requests BeforeID=oldestID.

updated, _ = updated.(Model).Update(pollMsg{items: newer})
// offset increases while scrolled, remains zero at bottom, and IDs are unique.
```

Cover arrows, initial bottom, PageDown, End, lazy prepend anchor, resize,
successful-send bottom reset, polling while scrolled, and selected-thread
identity.

- [ ] **Step 2: Run the TUI tests and confirm RED**

Run: `go test ./internal/tui -count=1`

Expected: FAIL because `j/k` move the list and the model has no viewport state.

- [ ] **Step 3: Implement offset and merge helpers**

```go
type olderMessagesMsg struct {
    items []domain.Message
    err error
}

// chatOffset is the count of message lines hidden below the viewport.
chatOffset int
loadingOlder bool
hasOlder bool
```

Use `mergeMessages` keyed by `Message.ID`, preserve ascending order, increase
the offset only for newly appended rows while scrolled, and leave it unchanged
when older rows prepend. Clamp offset after resize or replacement.

- [ ] **Step 4: Restrict list shortcuts and implement Page keys**

Keep only `up`, `down`, and `enter` in `updateList`. Handle PageUp, PageDown,
and End only on the chat screen before forwarding ordinary characters to the
composer.

- [ ] **Step 5: Run focused and full TUI tests and confirm GREEN**

Run: `go test ./internal/tui -count=1`

- [ ] **Step 6: Commit the TUI unit**

```sh
git add internal/tui
git commit -m "feat: add stable TUI history scrolling"
```

### Task 3: Explicit input mode and mode-specific scrcpy arguments

**Files:**
- Modify: `internal/domain/model.go`
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/scrcpy/manager.go`
- Modify: `internal/scrcpy/manager_test.go`
- Modify: `internal/bootstrap/runtime.go`
- Modify: `internal/bootstrap/runtime_test.go`

**Interfaces:**
- Produces: `domain.TextInputMode`, config default/validation, `scrcpy.Config.InputMode`, and `samsung.SenderConfig.InputMode`.

- [ ] **Step 1: Write failing config and scrcpy tests**

```go
if got := config.Default().Samsung.TextInputMode; got != domain.TextInputIntentBody { ... }
// invalid mode fails Load/Validate.

wantIntent := []string{"--no-video-playback", "--no-audio"}
// intent args omit --keep-active, --window-width, and --window-borderless.
// clipboard args retain the tiny window and omit --no-video-playback.
```

- [ ] **Step 2: Run config/scrcpy/bootstrap tests and confirm RED**

Run: `go test ./internal/config ./internal/scrcpy ./internal/bootstrap -count=1`

- [ ] **Step 3: Add the explicit value and validation**

```go
type TextInputMode string
const (
    TextInputIntentBody TextInputMode = "intent_body"
    TextInputClipboard  TextInputMode = "clipboard"
)
```

Default to intent body and reject every other value.

- [ ] **Step 4: Build scrcpy arguments per mode**

Use the common new-display/local-IME/start-app/record/no-audio arguments. Add
`--no-video-playback` only for intent mode; add title/1x1/borderless arguments
only for clipboard mode. Remove `--keep-active` from both until live evidence
requires it.

- [ ] **Step 5: Wire mode through bootstrap and confirm GREEN**

Run: `go test ./internal/config ./internal/scrcpy ./internal/bootstrap -count=1`

- [ ] **Step 6: Commit the input/scrcpy unit**

```sh
git add internal/domain internal/config internal/scrcpy internal/bootstrap
git commit -m "feat: separate Samsung text input modes"
```

### Task 4: Samsung package targeting and default SMS role

**Files:**
- Create: `internal/samsung/role.go`
- Create: `internal/samsung/role_test.go`
- Modify: `internal/samsung/controller.go`
- Modify: `internal/samsung/controller_test.go`
- Modify: `internal/samsung/sender.go`
- Modify: `internal/samsung/sender_test.go`
- Modify: `internal/domain/errors.go`

**Interfaces:**
- Produces: non-mutating `EnsureDefaultSMSHandler(context.Context) error` and package-constrained SENDTO commands.

- [ ] **Step 1: Write failing target and role tests**

Assert both open paths include `-p com.samsung.android.messaging`; role output
with that exact line succeeds, another package or empty output returns
`ErrSamsungMessagesNotDefault`, and sender checks role before `display.Start`.

- [ ] **Step 2: Run Samsung tests and confirm RED**

Run: `go test ./internal/samsung -run 'Target|Role|Default' -count=1`

- [ ] **Step 3: Implement role parsing and send preflight**

```go
output, err := device.Shell(ctx, "cmd", "role", "get-role-holders", "android.app.role.SMS")
// Split lines and require an exact Samsung package match.
```

Return safe errors without embedding command output. Invoke the check before
any virtual-display mutation.

- [ ] **Step 4: Add public package restriction and confirm GREEN**

Run: `go test ./internal/samsung -count=1`

- [ ] **Step 5: Commit the targeting/role unit**

```sh
git add internal/domain internal/samsung
git commit -m "fix: require Samsung Messages send ownership"
```

### Task 5: Remove send power mutation and hide intent data from argv

**Files:**
- Modify: `internal/adb/client.go`
- Modify: `internal/adb/client_test.go`
- Modify: `internal/adb/discovery.go`
- Modify: `internal/adb/discovery_test.go`
- Modify: `internal/samsung/controller.go`
- Modify: `internal/samsung/controller_test.go`
- Modify: `internal/samsung/sender.go`
- Modify: `internal/samsung/sender_test.go`
- Modify: `internal/bootstrap/runtime_test.go`

**Interfaces:**
- Produces: `ShellStdin(ctx, target, command)` on the ADB adapter and target; sender sequence without wake/sleep calls.

- [ ] **Step 1: Write failing stdin and minimal-power tests**

```go
client.ShellStdin(ctx, "synthetic-target", "private remote command")
// runner argv is only -s, target, shell; stdin has the command.

sender.Send(ctx, recipient, body)
// sequence contains role, display.start, intent, waits, baseline, tap, verify;
// it contains no power.read, power.wake, or power.restore.
```

Assert an ADB failure does not place stdin content into `CommandError`.

- [ ] **Step 2: Run adb/Samsung/bootstrap tests and confirm RED**

Run: `go test ./internal/adb ./internal/samsung ./internal/bootstrap -count=1`

- [ ] **Step 3: Add stdin-capable process execution**

Extend the command runner with an input byte slice, set `cmd.Stdin` from a
reader, and keep input outside `CommandError.Args` and stderr-derived normal
errors. Add `Target.ShellStdin` with the same connection-state transitions as
`Target.Shell`.

- [ ] **Step 4: Move SENDTO intents to remote-shell stdin**

Construct one command from constants, numeric display ID, normalized phone,
and single-quoted recipient/body. Reject NUL in every variable field and add a
trailing newline before invoking `adb -s target shell`.

- [ ] **Step 5: Delete power preparation from the sender sequence**

Remove `MainDisplayOff`, `WakeVirtualDisplay`, and `SleepMainDisplay` from the
sender controller contract and sequence. Retain display cleanup for controller
or disconnect failures.

- [ ] **Step 6: Run focused tests and confirm GREEN**

Run: `go test ./internal/adb ./internal/samsung ./internal/bootstrap -count=1`

- [ ] **Step 7: Commit the power/privacy unit**

```sh
git add internal/adb internal/samsung internal/bootstrap
git commit -m "fix: minimize send power and argv exposure"
```

### Task 6: Doctor mode and role readiness

**Files:**
- Modify: `internal/doctor/service.go`
- Modify: `internal/doctor/service_test.go`
- Modify: `internal/doctor/report.go`

**Interfaces:**
- Consumes: config input mode, Samsung role holder, optional provider probes.
- Produces: non-mutating role and mode-specific readiness checks.

- [ ] **Step 1: Write failing doctor tests**

Cover Samsung default/non-default output, intent mode without clipboard tools,
clipboard mode with a missing tool, text input mode/layout details, and absence
of `am start`, `input`, or scrcpy process calls.

- [ ] **Step 2: Run doctor tests and confirm RED**

Run: `go test ./internal/doctor -count=1`

- [ ] **Step 3: Implement checks and confirm GREEN**

Use the shared Samsung role parser. Send readiness is
`readReady && scrcpyReady && roleReady && layoutReady && modeReady`, with
`clipboardReady` included only for clipboard mode.

Run: `go test ./internal/doctor -count=1`

- [ ] **Step 4: Commit the doctor unit**

```sh
git add internal/doctor
git commit -m "feat: report mode-aware Samsung send readiness"
```

### Task 7: Verification hardening and guarded transport probes

**Files:**
- Modify: `internal/samsung/verify.go`
- Modify: `internal/samsung/verify_test.go`
- Modify: `internal/domain/errors.go`
- Modify: `internal/integration/real_send_test.go`

**Interfaces:**
- Preserves: exact SMS verification result.
- Produces: actionable redacted timeout and separate RCS/MMS send gates that cannot reuse the SMS recipient.

- [ ] **Step 1: Write failing verifier regression tests**

Add same-text consecutive sends, baseline duplicate, transient empty batch,
unsupported rows (already filtered by the provider), cancellation, timeout,
and offline failure. Assert timeout text contains a safe coordinate/transport
hint but neither private value.

- [ ] **Step 2: Run verifier tests and confirm RED**

Run: `go test ./internal/samsung -run Verify -count=1`

- [ ] **Step 3: Implement the minimal error hardening**

Wrap `ErrSendVerificationTimeout` with a constant safe hint. Do not retry the
tap and do not accept any weaker evidence.

- [ ] **Step 4: Add separately gated integration cases**

Create `TestRealSamsungRCSSend` and `TestRealSamsungMMSTextSend`. Each checks
its own enable flag and recipient, generates one timestamped test body, invokes
the sender once, and only passes when accessible provider evidence proves the
claimed transport. If the code cannot classify the transport, skip before
sending or return unsupported after a single gated observation; never print
the recipient/body.

- [ ] **Step 5: Compile integration and confirm GREEN/SKIP**

Run: `go test -tags=integration ./internal/integration -count=1`

- [ ] **Step 6: Commit the verification/integration unit**

```sh
git add internal/domain internal/samsung internal/integration
git commit -m "test: harden transport send verification gates"
```

### Task 8: Evidence-based documentation and final quality gate

**Files:**
- Modify: `README.md`

**Interfaces:**
- Produces: current input-mode, power, role, scrolling, and transport matrix documentation with PASS/SKIP/unsupported evidence only.

- [ ] **Step 1: Update README claims**

Document arrow-only list navigation, PageUp/PageDown/End, explicit input mode,
headless intent mode, Samsung role requirement, stdin privacy, and separate
transport gates. State SMS as verified only if the gated test ran; otherwise
distinguish prior reference evidence from current-run SKIP. State RCS/MMS as
unverified or unsupported unless their current gated tests pass.

- [ ] **Step 2: Format and tidy**

Run: `gofmt -w cmd internal`

Run: `go mod tidy`

- [ ] **Step 3: Run the complete quality gate**

```sh
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/msg
go test -tags=integration ./internal/integration -count=1
git diff --check
```

- [ ] **Step 4: Run enabled hardware gates only**

Run read/display/SMS/RCS/MMS integration commands only when their explicit
environment gates are already present. Do not synthesize recipients, set send
gates, or blind-retry a failed send.

- [ ] **Step 5: Perform the security checkpoint**

Inspect `git status`, staged/unstaged diff, and tracked filenames for `.env`,
private keys, certificates, credentials, recipients, bodies, recordings, and
test screenshots. Confirm no new public service, long-lived credential, or
secret storage was introduced.

- [ ] **Step 6: Commit documentation**

```sh
git add README.md
git commit -m "docs: document phase 3c transport evidence"
```
