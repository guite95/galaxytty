# GalaxyTTY Phase 3-B Real Samsung Messages Send Integration Design

## Context

GalaxyTTY Phase 3-A already provides the real read path through the existing
presentation, application, domain-port, and adapter boundaries. Phase 3-B adds
real text sending without replacing or weakening that read path.

Samsung Messages remains the actual sender. GalaxyTTY does not call modem SMS
APIs, request or bypass `SEND_SMS`, mutate the default SMS application, install
an APK, unlock the main display, or implement the private scrcpy protocol.
Instead, scrcpy owns a separate Android logical display and clipboard bridge,
while public ADB commands open and control Samsung Messages on that display.

The reference environment is macOS with scrcpy 4.1 and an authorized Samsung
Galaxy SM-A376N running Android 16 and One UI 8.5. Device endpoints, executable
paths, display IDs, phone numbers, and message text are runtime data and must
not be embedded in production code or retained in logs.

## Goals

- Preserve all Phase 3-A real and mock read behavior.
- Start a reusable scrcpy virtual display lazily on the first real send.
- Parse the Android logical display ID from current scrcpy output at runtime.
- Open a one-to-one Samsung Messages conversation with a public `SENDTO`
  intent targeted to that logical display.
- Inject only display-specific tap and key events through the selected ADB
  target.
- Enter Korean, other Unicode text, punctuation, and emoji through the macOS
  clipboard and scrcpy clipboard synchronization.
- Treat a send as successful only after an exact outgoing Content Provider row
  appears after the pre-send baseline.
- Enable real `msg send` and real TUI sending through `app.Service` and the
  existing `MessageSender` boundary.
- Preserve the user's clipboard when the host clipboard operations succeed.
- Keep unit tests hardware-independent and isolate real sending behind a build
  tag and two explicit environment-variable gates.
- Shut down scrcpy and delete its temporary recording on every normal GalaxyTTY
  shutdown path.

## Non-goals and prohibited paths

Phase 3-B does not add group sending, image or attachment sending, a helper APK,
default-SMS-app changes, automatic PIN entry, main-display wake or unlock,
SurfaceFlinger display mapping, notification integration, a complete reconnect
state machine, or automatic USB-to-Wireless failover.

It does not use `adb shell input text`, `cmd isms`, `cmd phone`, `service call`,
Telephony Provider inserts, Samsung internal send services, permission grants,
UIAutomator selectors, scrcpy private control sockets, or a scrcpy fork.

## Live facts verified on 2026-08-14

The following non-sensitive facts were rechecked before implementation:

- The repository started on clean `main` at Phase 3-A merge commit `2e28740`.
- `go test ./...`, `go test -race ./...`, `go vet ./...`, and
  `go build ./cmd/msg` passed before Phase 3-B changes.
- One authorized SM-A376N is currently discoverable by the Phase 3-A ADB path.
- scrcpy 4.1 is installed.
- Its public help documents `--new-display`, `--start-app`, `--record`,
  `--no-video-playback`, `--no-audio`, and `--no-window`.
- The previously verified compatible headless-like combination remains the
  implementation contract: recording plus disabled video playback and audio.
- The user explicitly opted into an actual send test and supplied a dedicated
  recipient. The value is never copied into source, fixtures, documentation,
  commits, or the final report.

Live smoke tests after implementation remain the source of truth for current
Samsung Messages layout behavior, clipboard propagation, provider latency, and
main-display lock state.

## Architecture

```text
TUI / CLI
    -> app.Service
         -> domain.MessageSender
              -> samsung.Sender
                   -> scrcpy.Manager
                   -> samsung.Controller
                        -> selected adb.Target
                   -> clipboard.Mac
                        -> pbpaste / pbcopy
                   -> provider.Store
                        -> content://sms verification

bootstrap.Real
    -> one selected adb.Target
         -> provider.Store
         -> samsung.Controller
         -> scrcpy.Manager target selection
```

`internal/bootstrap` remains the only assembly boundary. CLI and TUI know only
`app.API`; neither package imports ADB, scrcpy, clipboard, provider, or Samsung
control packages.

The new code uses four focused adapters:

- `internal/scrcpy` owns subprocess and virtual-display lifecycle.
- `internal/samsung` owns Samsung Messages intents, layout coordinates,
  display-specific input, and the end-to-end sender orchestration.
- `internal/clipboard` owns macOS clipboard reads and writes.
- `internal/provider` remains the source for sent-row verification through the
  existing `domain.MessageStore` methods.

No generic process framework, dependency-injection container, UI selector
engine, or reconnect state machine is added.

## Domain and application results

Sending returns the provider row that proved success. A small domain result
prevents CLI or TUI from querying provider internals after the send:

```go
type SendResult struct {
    MessageID int64 `json:"message_id"`
    ThreadID  int64 `json:"thread_id"`
}

type MessageSender interface {
    Send(context.Context, string, string) (SendResult, error)
}
```

`app.API.SendToAddress` and `SendToConversation` return the same result.
`SendToConversation` retains the one-participant check before calling the
sender. The existing group error becomes the exact user-facing text
`Group conversation sending is not supported yet.` through presentation-layer
formatting.

The mock sender returns the created synthetic message ID and thread ID so mock
CLI and TUI still exercise the same application path. The obsolete Phase 3-A
real read-only wiring is removed only from `bootstrap.Real`; the adapter may
remain for regression tests if useful.

## Virtual display manager

`scrcpy.Manager` implements `domain.VirtualDisplayManager` and is constructed
with the selected ADB target string plus a small config containing executable,
resolution, package, and startup timeout. Construction validates only static
configuration; executable lookup is deferred to `Start` so read-only commands
remain usable when scrcpy is absent. Production defaults are:

```text
scrcpy
-s <selected-target>
--new-display=1080x1920
--start-app=com.samsung.android.messaging
--record=<unique-temporary-file>.mp4
--no-video-playback
--no-audio
```

Arguments are passed directly to `exec.Command` without a host shell. The
temporary record file is created with `os.CreateTemp`, closed before scrcpy
starts, and deleted after every failed start or stop.

The manager routes both stdout and stderr into one parser stream because scrcpy
may emit server lifecycle lines on stderr. `ParseDisplayID` accepts the public
log shape containing `New display:` and `id=<digits>` and returns the Android
logical ID only. Width and height come from the configured virtual-display
profile; SurfaceFlinger IDs are neither populated nor queried.

`Start` is serialized:

1. Return the cached display when its process has not exited.
2. Clean up an exited session.
3. Create the temporary recording and start scrcpy.
4. Scan log data until a display ID appears, the process exits, the context is
   canceled, or the finite startup timeout expires.
5. On success, cache the process, display, completion channel, and temp path.
6. On failure, terminate and reap the process, delete the temp file, clear
   state, and return an actionable wrapped error.

`Healthy` is non-blocking and reports false when no session exists or the
process completion channel is closed. `Stop` is idempotent, detaches the cached
session under lock, requests termination, waits or escalates to kill within the
caller's deadline, reaps it, and removes the recording. Concurrent or repeated
shutdown calls must not panic or signal a reused process handle.

An unexpected scrcpy exit leaves the manager unhealthy. The next send gets one
normal `Start` opportunity, which cleans up the prior session and starts a new
one. This is intentionally smaller than a reconnect state machine.

## Samsung Messages controller

`samsung.Controller` depends on the already selected `adb.Target`, the current
`samsung.Layout`, and no process runner of its own. Every Android command is an
argument slice passed through `Target.Shell`.

Open conversation:

```text
am start
--display <android-logical-display-id>
-a android.intent.action.SENDTO
-d smsto:<recipient>
```

Input operations use the same display ID:

```text
input -d <id> tap <composer-x> <composer-y>
input -d <id> keyevent 279
input -d <id> tap <send-x> <send-y>
```

The composer tap is retained for deterministic focus because UIAutomator cannot
reliably inspect the virtual display. Key code 279 is `KEYCODE_PASTE`; the
controller never exposes or uses `input text`.

The default layout is the existing 1080x1920 profile with composer `(500,1800)`
and send `(1004,1273)`. Coordinates appear only in `samsung.DefaultLayout` and
configuration defaults. Controller errors identify the failed operation but do
not include recipient or message text.

## macOS clipboard adapter

`internal/clipboard.Mac` uses direct subprocess execution. Construction stores
command names without resolving them; lookup and execution happen only when a
send calls `Read` or `Set`, preserving read-only startup when clipboard tooling
is unavailable:

- `pbpaste` returns the current clipboard bytes.
- `pbcopy` receives the new bytes on stdin.

The domain clipboard port minimally expands to `Read` and `Set`. The sender owns
the temporary-clipboard transaction so the main send error remains primary:

1. Read the old value.
2. Set the outgoing text.
3. Wait only the configured clipboard-sync delay.
4. Paste on the virtual display.
5. Wait the short send-settle delay and tap send.
6. Restore the old value after Android has consumed the outgoing clipboard.

Clipboard read or set failures stop before paste. A restore failure is returned
only when no earlier send-stage error exists; otherwise it is joined as cleanup
context without replacing the primary failure. Neither adapter errors nor
debug output includes clipboard contents.

## Send sequence and verification

`samsung.Sender` is the concrete real `domain.MessageSender`. It depends on the
virtual-display manager, Samsung controller, clipboard, message store, timing
configuration, and an injected wait function used by tests.

The exact sequence is:

1. Validate non-empty normalized recipient and non-blank text.
2. `Start` or reuse the lazy virtual display.
3. Open the recipient conversation on that display.
4. Wait the bounded internal conversation-ready delay.
5. Tap the configured composer coordinate.
6. Read the host clipboard and set the outgoing text.
7. Wait `clipboard_sync_delay`.
8. Send display-specific `KEYCODE_PASTE`.
9. Wait `send_settle_delay`.
10. Read `LatestMessageID` immediately before the send action.
11. Tap the configured send coordinate.
12. Poll `MessagesAfter(baselineID)` until an exact match appears or the
    verification timeout/context expires.
13. Restore the previous host clipboard and return the verified IDs.

The verification predicate requires all of:

- `Direction == domain.DirectionOutgoing`;
- `domain.NormalizePhone(row.Address) == domain.NormalizePhone(recipient)`;
- `row.Body == submittedText`;
- `row.ID > baselineID` by construction.

Incoming self-message echoes, unrelated new rows, wrong bodies, and rows for a
different normalized address are ignored. An empty poll does not fail early.
Default verification timing is 10 seconds with a 300 millisecond poll interval.
Timeout returns `domain.ErrSendVerificationTimeout`; a provider or ADB error is
returned immediately and never treated as success.

Controller/input failures stop the current display session so a later send can
start a clean session after a likely disconnect. Clipboard failures do not
invalidate an otherwise healthy display. A verification timeout is not by
itself proof that scrcpy died and therefore does not force-stop it.

## Configuration

The current Samsung display and layout settings become active runtime inputs.
The only new user-tunable timings are those most likely to vary by host/device:

```toml
[samsung]
display_width = 1080
display_height = 1920
clipboard_sync_delay = "300ms"
send_settle_delay = "200ms"
verification_timeout = "10s"

[samsung.layout]
composer_x = 500
composer_y = 1800
send_x = 1004
send_y = 1273
```

Startup timeout, verification poll interval, package name, and the short
conversation-ready delay remain focused internal defaults. Config validation
rejects non-positive dimensions, out-of-bounds points, and non-positive timing
values before a real send runtime is built.

## CLI behavior

The Phase 3-A real-send guard is removed. `msg send --to PHONE --text TEXT`
constructs the same real runtime and calls `app.Service.SendToAddress`.

Plain success prints a concise confirmation without echoing the recipient or
body. JSON success writes exactly:

```json
{"success":true,"message_id":12345,"thread_id":49}
```

Errors go to the existing command error path with actionable messages for
scrcpy lookup/startup/display-ID failures, Samsung Messages control failures,
clipboard failures, disconnected ADB, and send-verification timeout. JSON mode
does not emit partial success JSON on failure.

## TUI behavior

The Bubble Tea model gains a `sending` flag. Submitting non-command text while
idle captures the text, sets `sending=true`, shows `Sending…`, and returns one
asynchronous send command. Enter presses while `sending` are ignored.

On success, the model:

- clears `sending` and the composer;
- clears the error;
- reloads the selected message history and conversation list;
- restores the live connection label.

On failure, it clears only `sending`, retains the exact composer contents, and
shows an actionable error. It does not automatically retry. Group conversation
rejection continues at the application boundary and never starts scrcpy.

`/exit`, `/quit`, Ctrl+C, and the CLI defer all call the existing service
shutdown. The lifecycle now owns the real scrcpy manager, making those paths
terminate and reap the child and remove its recording. Esc remains a normal
chat-to-list operation and does not stop the reusable display session.

## Doctor behavior

Doctor remains non-mutating. It does not start scrcpy, create a display, open a
conversation, change the clipboard, inject input, or send a character.

The existing scrcpy version inspection becomes a required send-readiness check
while provider and device checks remain read readiness. A report distinguishes:

```text
Read:  ready
Send:  ready
```

Send readiness requires an eligible Samsung Galaxy, Samsung Messages, the SMS
provider used for verification, scrcpy executable/version inspection, macOS
clipboard tools, and a valid layout profile. Missing scrcpy or clipboard tools
does not incorrectly mark the Phase 3-A read path unavailable.

## Errors and privacy

Recognizable errors cover:

- scrcpy executable missing;
- scrcpy startup failure or exit before readiness;
- virtual display ID timeout;
- unexpected scrcpy exit;
- Samsung Messages unavailable or conversation-open failure;
- clipboard read, set, paste, or restore failure;
- composer or send tap failure;
- send verification timeout;
- ADB unauthorized, offline, or disconnected states;
- unsupported group send.

Every stage wraps its cause with `%w`. Normal CLI/TUI output contains no stack
trace. Process arguments or error text that could contain the recipient are
sanitized before presentation. Message bodies, phone numbers, clipboard values,
provider rows, and personal contact data are never added to debug logs.

## Testing strategy

All production behavior is developed test-first. Default tests use synthetic
recipients and text and require no device.

### Unit tests

- `internal/scrcpy`: argv construction, display parser variants, startup
  timeout, exit-before-ID, successful state, unexpected exit health, reusable
  start, idempotent stop, terminate/kill fallback, and temp-file cleanup.
- `internal/samsung` controller: exact `am start`, display-specific composer
  tap, paste key event 279, and send tap argv; each failure is wrapped without
  leaking recipient data.
- `internal/clipboard`: `pbpaste`, `pbcopy` stdin, empty clipboard, and process
  error mapping without content leakage.
- `internal/samsung` sender: ordered sequence, lazy display reuse, early stop on
  every stage failure, clipboard restore precedence, and result mapping.
- Verification: matching outgoing row, wrong recipient, wrong body, incoming
  echo, unrelated duplicate rows, provider failure, cancellation, and timeout.
- Config: defaults, custom timings, invalid timing/dimensions/coordinates.
- Bootstrap: one selected target is reused by reads and all send adapters; real
  startup does not start scrcpy.
- CLI: real runtime construction for send, plain and JSON success, JSON failure
  purity, actionable errors, and mock regressions.
- TUI: idle send, duplicate Enter suppression, `Sending…`, success clear and
  two refreshes, failure preservation/error, group failure, shutdown cleanup,
  and existing navigation/polling behavior.
- Doctor: separate read/send readiness and no mutating commands.

### Integration tests

Hardware tests live behind `//go:build integration`. The virtual-display smoke
test starts and stops only the display manager and may open a conversation only
when an explicitly supplied test recipient exists. It never pastes or taps Send.

The actual-send test skips unless both are present:

```text
GALAXYTTY_ENABLE_SEND_TEST=1
GALAXYTTY_TEST_RECIPIENT=<explicit-recipient>
```

It generates `GalaxyTTY integration test <timestamp>` as the full body, records
the provider baseline, calls the real application service, verifies the exact
outgoing provider row, and does not print recipient or body. Because the
approved recipient is the device owner's own number, an incoming echo may
arrive immediately; the assertion accepts only the outgoing matching row.

Before and after the smoke/send tests, read-only `dumpsys` output records whether
the main display remains off and the device remains locked. Tests never wake,
unlock, or enter a PIN. Every test defers shutdown and verifies the temporary
recording is removed.

## Functional commit boundaries

Implementation is committed without pushing, in independently reviewable
units:

1. approved design and implementation plan;
2. scrcpy virtual-display lifecycle;
3. Samsung controller and macOS clipboard adapter;
4. verified Samsung sender and domain/application result changes;
5. bootstrap, configuration, lifecycle, CLI, TUI, and doctor integration;
6. integration safety tests and documentation;
7. any isolated fixes discovered by live-device verification.

No commit includes a Codex co-author trailer or generated-by footer.

## Acceptance criteria

Phase 3-B is accepted when the real default path still reads conversations and
messages, the first real send lazily starts a reusable scrcpy virtual display,
Samsung Messages receives Unicode text through clipboard paste on the runtime
logical display, a send succeeds only after an exact outgoing provider row is
observed, CLI/TUI/mock/doctor behaviors match this design, shutdown reaps and
cleans scrcpy, the explicit real-device integration succeeds without changing
the main display lock state, and all requested Go test/race/vet/build commands
pass.
