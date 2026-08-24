# GalaxyTTY

**Read and send Samsung Messages from your terminal.**

> [!WARNING]
> GalaxyTTY is an early-stage, unofficial project and is not affiliated with
> Samsung. Real sending controls the installed Samsung Messages application on
> an authorized Galaxy and can incur normal carrier or data charges.

GalaxyTTY provides a Go CLI and Bubble Tea TUI for Samsung Galaxy messages on
macOS.

> [!IMPORTANT]
> The `feat/tui-redesign` branch is the GalaxyTTY v2 transition. It adds a
> native Kotlin `android-helper`, local TCP/DNS-SD protocol, and remote Go
> adapters while retaining the existing application/domain/TUI/CLI layers.
> The default real mode remains the verified legacy ADB/scrcpy path until the
> Helper notification, history, pairing, and send paths pass real-device gates.

Legacy reads use Android Content Providers over ADB and text sends use Samsung
Messages on a separate scrcpy virtual display. The v2 Helper path moves Android
access onto the Galaxy and is selected explicitly with `--helper` during the
transition.

## Prerequisites

- macOS (`pbcopy`, `pbpaste`, and `osascript` enable the clipboard compatibility path)
- adb from Android platform-tools
- scrcpy 4.1 or newer for real sending
- One authorized Samsung Galaxy connected through USB debugging, or an
  already-connected Wireless Debugging target
- Samsung Messages (`com.samsung.android.messaging`)

The legacy client never runs `adb pair`, targets the main display with a wake
command, unlocks it, enters a PIN, changes the default SMS app, or grants SMS
permissions. The v2 deployment script only performs an APK update install; it
does not bypass Android's Notification Access or other user approval screens.
Pair or authorize the device yourself before starting it.

## Real mode

Real mode is the default:

```sh
go run ./cmd/msg doctor
go run ./cmd/msg
go run ./cmd/msg conversations
go run ./cmd/msg conversations --json
go run ./cmd/msg unread --json
go run ./cmd/msg messages 42 --json
```

## v2 Helper PoC

Build and update-install the native Helper without uninstalling its data:

```sh
make helper-test
make helper-deploy
```

Android SDK Platform 36 is required. The user must grant Notification Access
from the Helper screen. Existing SMS history additionally requires the Helper's
**Allow SMS history (read-only)** button. The deployment script never bypasses
either Android approval UI.

The Mac discovers `_galaxytty._tcp.local` automatically:

```sh
go run ./cmd/msg pair
go run ./cmd/msg --helper
```

To measure the next real notification without printing its content:

```sh
go run ./cmd/msg --helper latency
```

The probe calibrates Galaxy/Mac clock offset with encrypted PING/PONG samples,
then reports notification-post to Mac event-decode latency for one new
content-bearing event. It does not measure the final terminal redraw.

On first setup, reveal the high-entropy pairing code from the Helper screen and
enter it at the hidden prompt. The code is never sent over TCP; the Mac proves
possession with a fresh HMAC challenge and stores the resulting shared
credential in `~/.config/galaxytty/credentials.json` with mode `0600`.

`--helper-address host:port` is available only as a debug fallback. The current
authenticated PoC is intentionally **non-sending by default**. It extracts the latest
incoming `MessagingStyle` text from Samsung Messages notifications, keeps a
bounded in-memory-only conversation cache, and exposes read sync plus live
`MESSAGE_RECEIVED` events. A correlated `SEND_REPLY` request reaches a
default-blocked execution policy. Normal replies are possible only after the
user confirms **Allow replies from paired Mac** on the Galaxy; the permission
can be blocked immediately from the same Helper screen. A debuggable APK also
retains a private 60-second one-shot gate for explicit integration tests. With user-granted
`READ_SMS`, bounded SMS conversations/history are
queried by the Helper's `ContentResolver`; the Mac no longer reads that
Provider for the v2 path. Message content is available only after mutual HMAC key
confirmation, inside AES-256-GCM `SECURE` frames using directional HKDF-derived
keys and monotonic replay-protected counters. Notification transport remains
`unknown` until the Helper has evidence for SMS, MMS, or RCS.

The Helper also retains a bounded 512-event in-memory sequence journal. The Mac
requests only a missing sequence range after a live gap or reconnect, suppresses
duplicate recovered messages, and falls back to a full encrypted read refresh
when the Helper process has restarted or the journal cannot prove completeness.

The persistent client resolves DNS-SD again before every connection attempt,
so a Helper port or Galaxy IP change is not pinned to the previous endpoint.
Connection, disconnection, and reconnection states flow through the application
service to the TUI without restoring message polling. A half-open session that
stops answering encrypted heartbeats is closed and enters the same bounded
rediscovery loop.

The application layer can now route an existing notification conversation by
opaque `threadId` through `SEND_REPLY`; the Mac does not need a phone number and
still does not know Samsung-specific details. The Helper retains the matching
free-form RemoteInput action only in memory. Release and debug builds both
default to blocked and require a persistent local opt-in for normal replies.
The gated debug test independently proves that exactly one active reply action matches
the user-authorized current notification, then creates a private one-shot token
that expires after 60 seconds. RemoteInput acceptance produces
`accepted_unverified`, never a verified-send result. The TUI clears the composer
to prevent an accidental duplicate, keeps a session-only outgoing echo, and
labels it `전송 요청됨 · 미검증`. If independent outgoing Provider evidence
later appears, it replaces the local echo. The echo is never persisted as proof
of delivery.

If the selected conversation no longer has an active RemoteInput action, the
Helper reuses its existing one-to-one conversation data to prepare a Samsung
Messages `ACTION_SENDTO` composer. Android does not allow the background bridge
to open that activity directly, so the Helper posts a generic local notification.
Tapping it opens Samsung Messages with the reply prefilled; the user reviews it
and taps Samsung's send button. The protocol reports `user_action_required`, the
Mac does not draw an outgoing bubble, and neither the phone number nor message
body appears in the Helper notification or logs. Notification-derived RCS
conversations can use the same fallback only when Samsung exposes a safe phone
URI; no recipient is guessed from a display name.

The real Helper reply test can deliver a message and must never be run without
explicit permission for the recipient and text:

```sh
GALAXYTTY_REAL_DEVICE_TEST=1 \
GALAXYTTY_ENABLE_SEND_TEST=1 \
GALAXYTTY_TEST_RECIPIENT='authorized-current-notification' \
GALAXYTTY_TEST_TEXT='authorized-test-text' \
GALAXYTTY_ALLOW_SOLE_ACTIVE_REPLY=1 \
./scripts/test-helper-send.sh
```

The script disables Go test caching, performs a read-only target/capability
preflight, arms exactly one debug execution, removes any leftover marker, and
checks for exact outgoing SMS Provider evidence without printing private data.
`GALAXYTTY_ALLOW_SOLE_ACTIVE_REPLY=1` is needed only when Samsung does not
export the user-visible target label verbatim; it allows selection only when
exactly one active reply-capable notification exists.
No Provider match means the result remains unverified; it is not evidence of
RCS delivery.

When one physical Galaxy appears through USB and Wireless ADB, GalaxyTTY groups
the endpoints by hardware serial and selects USB by default. Select an exact
already-authorized target when multiple physical Galaxies are present:

```sh
go run ./cmd/msg --device synthetic-adb-target conversations
```

The `--mock`, `--helper`, `--json`, `--device`, and debug-only
`--helper-address` flags may appear before or after a subcommand.

## Real text sending

Send a one-to-one text message through Samsung Messages:

```sh
go run ./cmd/msg send --to 01012345678 --text '안녕하세요 😀'
go run ./cmd/msg send --to 01012345678 --text '안녕하세요 😀' --json
```

JSON success contains only the verified provider IDs:

```json
{"success":true,"message_id":12345,"thread_id":49}
```

The first real send lazily starts one reusable 1080x1920 scrcpy virtual
display. Samsung Messages is opened there with a display-targeted `SENDTO`
intent. On the reference Android 16 / One UI 8.5 device, Samsung Messages could
not read the clipboard set by scrcpy from its virtual display, so the active
compatibility path supplies Korean, other Unicode text, and emoji through
Android's standard `sms_body` intent extra. It never uses `adb shell input
text`. The intent argument is remote-shell quoted and controller errors redact
the recipient and body.

The macOS `pbcopy`/`pbpaste` adapter, scrcpy clipboard synchronization, host
clipboard restoration, and display-specific `KEYCODE_PASTE` path remain
implemented and unit-tested for devices where the virtual-display clipboard is
available. The scrcpy bridge uses its documented public shortcut in a tiny
dedicated window and never opens the private control protocol.

Tapping Send is not considered success. GalaxyTTY records the latest SMS
Provider ID immediately before the tap and waits for a newer row whose
direction is outgoing and whose normalized recipient and body both match
exactly. Samsung Messages decides whether the conversation uses SMS, MMS, or
RCS; GalaxyTTY does not force a transport.

The main display remains locked and GalaxyTTY never enters a PIN. All message
UI input is sent to the runtime Android logical display ID parsed from scrcpy
output. On Samsung firmware that puts the virtual display in the main display's
power group, a display-specific wake event may also activate logical display 0
during the send; GalaxyTTY records the original state and restores display 0 to
sleep after both success and failure.

Group conversations and image/attachment sending are not supported. A group
send attempt fails before starting the virtual display.

## TUI

Run `go run ./cmd/msg`, select a one-to-one conversation, enter text, and press
Enter. While a send is active, the composer shows `Sending…` and ignores more
input to prevent duplicate sends. On verified success GalaxyTTY clears the
composer and refreshes conversations and history. On failure it preserves the
text for a deliberate retry.

| Key | Action |
| --- | --- |
| Enter | Open the selected conversation or submit the composer |
| Esc | Return from chat to the conversation list |
| `/help` | Show keyboard help |
| `/exit`, `/quit` | Gracefully shut down and stop scrcpy |
| Ctrl+C | Gracefully shut down and stop scrcpy |

The `q` key remains ordinary composer input, not a quit shortcut. The virtual
display is reused for the GalaxyTTY process lifetime and is stopped on normal
shutdown; its temporary recording is deleted.

## Doctor

`msg doctor` performs non-mutating checks only. It does not start scrcpy, open a
conversation, change the clipboard, inject input, or send a character.

It reports independent readiness:

```text
Read: ready
Send: ready
```

Send readiness checks scrcpy and the configured Samsung layout in addition to
the selected Galaxy, Samsung Messages, and SMS Provider. Clipboard compatibility
is reported separately; missing clipboard commands do not block the active
intent-body send path or the existing read path.

## Mock mode

```sh
go run ./cmd/msg --mock
go run ./cmd/msg conversations --mock
go run ./cmd/msg unread --mock --json
go run ./cmd/msg messages 1 --mock
go run ./cmd/msg send --mock --to 01012345678 --text 'synthetic message'
go run ./cmd/msg send --mock --to 01012345678 --text 'synthetic message' --json
go run ./cmd/msg doctor --mock
```

Mock fixtures contain only synthetic data and use the same application and
presentation paths as real mode.

## Configuration

The optional TOML file is loaded from
`$XDG_CONFIG_HOME/galaxytty/config.toml`, or
`~/.config/galaxytty/config.toml`.

```toml
[connection]
prefer_usb = true
device = "synthetic-adb-target"

[polling]
interval = "1s"

[notifications]
enabled = true
show_when_focused = false

[samsung]
display_width = 1080
display_height = 1920
clipboard_sync_delay = "300ms"
send_settle_delay = "500ms"
verification_timeout = "10s"

[samsung.layout]
composer_x = 500
composer_y = 1800
send_x = 1004
send_y = 955
```

Omit `connection.device` for automatic selection. A CLI device flag overrides
the file. Missing files use defaults; malformed files, unknown fields,
non-positive timings, invalid dimensions, and out-of-bounds coordinates return
an error before runtime construction.

## Architecture

```text
TUI / CLI
    |
Application Service
    |
MessageSender + MessageStore ports
    |
Samsung sender
    |-- scrcpy virtual-display manager
    |-- selected ADB target / Samsung controller / sms_body compatibility
    |-- macOS clipboard adapter (supported compatibility path)
    `-- SMS Provider verification
```

- `internal/app` owns conversation targeting, group rejection, polling,
  notification policy, status, and lifecycle.
- `internal/adb` owns direct ADB subprocess execution, output parsing,
  USB/Wireless classification, and device selection.
- `internal/provider` owns read-only Content Provider query construction and
  SMS/conversation/contact mapping.
- `internal/scrcpy` owns the lazy subprocess, runtime logical display ID,
  health, termination, reaping, and recording cleanup.
- `internal/samsung` owns the fixed layout profile, display-specific public ADB
  input, send sequence, and exact outgoing-row verification.
- `internal/clipboard` owns direct `pbcopy`/`pbpaste` execution.
- `internal/bootstrap` assembles mock or real adapters below the same app API.
- `internal/tui` and `internal/cli` never execute ADB, scrcpy, or clipboard
  commands directly.

## Development and integration safety

The default suite is hardware-independent and never sends a message:

```sh
gofmt -w cmd internal
go mod tidy
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/msg
```

The Phase 3-A read integration remains separately gated:

```sh
GALAXYTTY_REAL_READ_TEST=1 \
go test -tags=integration ./internal/integration -run TestRealGalaxyReadPath -count=1 -v
```

Virtual-display lifecycle smoke testing starts scrcpy but never pastes or taps
Send. It opens a conversation only when an explicit recipient is also set:

```sh
GALAXYTTY_REAL_DISPLAY_TEST=1 \
GALAXYTTY_TEST_RECIPIENT='explicit-test-recipient' \
go test -tags=integration ./internal/integration -run TestRealVirtualDisplaySmoke -count=1 -v
```

An actual message is sent only when both gates are present:

```sh
GALAXYTTY_ENABLE_SEND_TEST=1 \
GALAXYTTY_TEST_RECIPIENT='explicit-test-recipient' \
go test -tags=integration ./internal/integration -run TestRealSamsungSend -count=1 -v
```

The actual-send test generates a clearly labeled timestamp body, accepts only
the exact outgoing provider row, ignores an incoming self-message echo, and
does not print the body or recipient.

## Known limitations

- Samsung Messages composer/send coordinates are device- and app-layout
  dependent. Adjust the Samsung layout configuration after an app update if
  needed.
- Clipboard synchronization is asynchronous; only the focused sync and settle
  delays are configurable. The reference Android 16 device uses the standard
  `sms_body` compatibility path because its virtual display cannot consume the
  scrcpy-populated global clipboard.
- Only one-to-one text sends are supported. Group sending and images or other
  attachments are deferred.
- A dead scrcpy process is restarted on the next send, but advanced reconnect
  and automatic USB-to-Wireless failover are deferred.
- MMS/RCS attachment polish, native notifications, image viewing, history
  scrolling polish, and Homebrew release packaging are deferred.

Licensed under MIT.
