# GalaxyTTY

**Read and send Samsung Messages from your terminal.**

> [!WARNING]
> GalaxyTTY is an early-stage, unofficial project and is not affiliated with
> Samsung. Real sending controls the installed Samsung Messages application on
> an authorized Galaxy and can incur normal carrier or data charges.

GalaxyTTY provides a Go CLI and Bubble Tea TUI for Samsung Galaxy messages on
macOS. Reads use Android Content Providers over ADB. Text sends use Samsung
Messages on a separate scrcpy virtual display; GalaxyTTY does not install an
Android application and does not call modem SMS APIs directly.

## Prerequisites

- macOS (`pbcopy`, `pbpaste`, and `osascript` are required only for the optional
  clipboard compatibility mode)
- adb from Android platform-tools
- scrcpy 4.1 or newer for real sending
- One authorized Samsung Galaxy connected through USB debugging, or an
  already-connected Wireless Debugging target
- Samsung Messages (`com.samsung.android.messaging`)
- Samsung Messages selected as the default SMS role holder for sending

GalaxyTTY never runs `adb pair`, issues a main-display wake or unlock command,
enters a PIN, changes the default SMS app, grants SMS permissions, or installs
an APK. Pair or authorize the device and choose Samsung Messages as the default
handler yourself before starting it.

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

When one physical Galaxy appears through USB and Wireless ADB, GalaxyTTY groups
the endpoints by hardware serial and selects USB by default. Select an exact
already-authorized target when multiple physical Galaxies are present:

```sh
go run ./cmd/msg --device synthetic-adb-target conversations
```

The `--mock`, `--json`, and `--device` flags may appear before or after a
subcommand.

## Real text sending

Send a one-to-one text message through Samsung Messages:

```sh
go run ./cmd/msg send --to 01012345678 --text '안녕하세요 😀'
go run ./cmd/msg send --to 01012345678 --text '안녕하세요 😀' --json
```

SMS JSON success contains the verified provider IDs:

```json
{"success":true,"message_id":12345,"thread_id":49}
```

The first real send lazily starts one reusable 1080x1920 scrcpy virtual
display. The default `intent_body` mode runs without video playback or a host
window. Samsung Messages is opened with a package-targeted, display-targeted
`SENDTO` intent, so another installed handler cannot receive the send. The
sender rechecks the public Android SMS role immediately before starting the
display.

Korean, other Unicode text, and emoji are supplied through Android's standard
`sms_body` intent extra. GalaxyTTY never uses `adb shell input text`. The remote
command is single-quote escaped, rejects NUL, and is sent to `adb shell` through
stdin so the recipient and body are not placed in the host process argument
list. Normal logs and errors redact both values.

The macOS `pbcopy`/`pbpaste` adapter, scrcpy clipboard synchronization, host
clipboard restoration, and display-specific `KEYCODE_PASTE` path remain
implemented and unit-tested for devices where the virtual-display clipboard is
available. The scrcpy bridge uses its documented public shortcut in a tiny
dedicated window and never opens the private control protocol.

Tapping Send is not considered success. GalaxyTTY records independent SMS and
MMS Provider baselines immediately before the tap and waits for newer exact
outgoing evidence: normalized recipient and body must both match. Incoming
self-echoes, rows before the baseline, and unrelated rows do not count.
Samsung Messages decides whether the conversation uses SMS, text MMS, or RCS;
GalaxyTTY does not force a transport and does not retry after an ambiguous
timeout.

The production send path does not use scrcpy `--keep-active`, inject a wake key,
or run a cleanup sleep command. All message UI input is sent only to the
runtime Android logical display ID parsed from scrcpy output. GalaxyTTY never
enters a PIN or intentionally unlocks the main display.

Reference-device validation currently has an open send-path limitation. A
non-sending A/B on SM-A376N found that both the default headless-like session
and the same session with `--keep-active` left the virtual display `OFF` with no
focused Samsung Messages window. `--keep-active` therefore remains disabled,
and the changed Phase 3-C production path is not claimed as actual-send
verified until the video-playback/window dependency is isolated. See the
[virtual-display power decision](docs/decisions/0002-virtual-display-power-behavior.md).

### Transport verification

| Transport | Send path | Machine verification | Evidence and limitation |
| --- | --- | --- | --- |
| SMS | Samsung Messages | Exact | New `content://sms` sent row with exact recipient and body |
| Text MMS | Samsung Messages | Exact when exposed | New sent `content://mms` send-request plus one exact `text/plain` part and one exact `TO` address |
| RCS / Chat+ | Samsung Messages | Transport classification unsupported | Accessible Samsung SMS extension columns do not provide a reliable SMS-versus-RCS discriminator on the reference device; GalaxyTTY does not label an unclassified success as RCS |

`transport` is emitted only when the verifier can classify it. For example, a
verified text MMS result includes `"transport":"mms"`; an exact row without a
reliable transport discriminator leaves the field absent.

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
| Up / Down | Move through conversations |
| PageUp / PageDown | Scroll toward older / newer chat history |
| End | Return to the latest message |
| Esc | Return from chat to the conversation list |
| `/help` | Show keyboard help |
| `/exit`, `/quit` | Gracefully shut down and stop scrcpy |
| Ctrl+C | Gracefully shut down and stop scrcpy |

The `j`, `k`, and `q` keys remain ordinary composer input, not navigation or
quit shortcuts. PageUp lazily fetches older pages when the viewport reaches the
oldest loaded message. New polling results follow the bottom only while the
user is already at the bottom; they do not displace an older viewport. The
virtual display is reused for the GalaxyTTY process lifetime and is stopped on
normal shutdown; its temporary recording is deleted.

## Doctor

`msg doctor` performs non-mutating checks only. It does not start scrcpy, open a
conversation, change the clipboard, inject input, or send a character.

It reports independent readiness:

```text
Read: ready
Send: ready
```

Send readiness checks the default Samsung SMS role, scrcpy, the configured text
input mode, and the Samsung layout in addition to the selected Galaxy, Samsung
Messages, and SMS Provider. Clipboard compatibility is reported separately;
missing clipboard commands do not block `intent_body` mode or the read path,
but do block explicitly selected `clipboard` mode. Doctor also reports MMS
provider access and the visibility (not inferred meaning) of candidate RCS
extension columns.

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
text_input_mode = "intent_body"
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
the file. `text_input_mode` accepts only `intent_body` (the default) or
`clipboard`. Missing files use defaults; malformed files, unknown fields,
unsupported modes, non-positive timings, invalid dimensions, and out-of-bounds
coordinates return an error before runtime construction.

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
    |-- selected ADB target / package-targeted Samsung controller / sms_body
    |-- macOS clipboard adapter (supported compatibility path)
    `-- exact SMS and text-MMS Provider verification
```

- `internal/app` owns conversation targeting, group rejection, polling,
  notification policy, status, and lifecycle.
- `internal/adb` owns direct ADB subprocess execution, output parsing,
  USB/Wireless classification, and device selection.
- `internal/provider` owns read-only Content Provider query construction and
  SMS/conversation/contact mapping plus outgoing text-MMS evidence mapping.
- `internal/scrcpy` owns the lazy subprocess, runtime logical display ID,
  health, termination, reaping, and recording cleanup.
- `internal/samsung` owns the fixed layout profile, display-specific public ADB
  input, send sequence, and exact outgoing-row verification.
- `internal/clipboard` owns direct `pbcopy`/`pbpaste` execution.
- `internal/bootstrap` assembles mock or real adapters below the same app API.
- `internal/tui` and `internal/cli` never execute ADB, scrcpy, or clipboard
  commands directly.

### Decision records

- [RCS provider verification](docs/decisions/0001-rcs-provider-verification.md)
  explains why a single gated send is followed by sanitized provider
  observation after a verification timeout, without weakening production
  success semantics or retrying.
- [Virtual-display power behavior](docs/decisions/0002-virtual-display-power-behavior.md)
  records why production sends omit `--keep-active`, wake, and cleanup sleep
  mutations while gated integration tests retain read-only state checks.

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

RCS / Chat+ and text-MMS observations have separate opt-ins and never reuse the
SMS recipient automatically:

```sh
GALAXYTTY_ENABLE_RCS_SEND_TEST=1 \
GALAXYTTY_RCS_TEST_RECIPIENT='explicit-known-chat-plus-recipient' \
go test -tags=integration ./internal/integration -run TestRealSamsungRCSSend -count=1 -v

GALAXYTTY_ENABLE_MMS_SEND_TEST=1 \
GALAXYTTY_MMS_TEST_RECIPIENT='explicit-mms-test-recipient' \
go test -tags=integration ./internal/integration -run TestRealSamsungMMSTextSend -count=1 -v
```

Each test sends at most once. After a production verification timeout, the RCS
test performs one read-only query for rows after the pre-send baseline; it does
not tap Send again. The test cannot PASS unless accessible evidence reliably
classifies RCS, and otherwise reports the transport as unsupported. The MMS
test requires exact outgoing MMS message, text-part, and recipient evidence.

## Known limitations

- Samsung Messages composer/send coordinates are device- and app-layout
  dependent. Adjust the Samsung layout configuration after an app update if
  needed.
- Clipboard synchronization is asynchronous and device-dependent. It is an
  explicit compatibility mode; the reference configuration uses `intent_body`.
- Only one-to-one text sends are supported. Group sending and images or other
  attachments are deferred.
- RCS transport classification is unsupported on the reference device with the
  currently accessible provider fields. Exact unclassified outgoing evidence
  is not advertised as verified RCS.
- Text-MMS provider verification is implemented, but a carrier-billable live
  MMS send is run only under its explicit integration gate.
- A dead scrcpy process is restarted on the next send, but advanced reconnect
  and automatic USB-to-Wireless failover are deferred.
- Full MMS/RCS history, attachment handling, native notifications, image
  viewing, and Homebrew release packaging are deferred.

Licensed under MIT.
