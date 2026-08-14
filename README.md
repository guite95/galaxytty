# GalaxyTTY

**Read Samsung Messages in your terminal.**

> [!WARNING]
> GalaxyTTY is an early-stage, unofficial project. Real-device mode is read-only in Phase 3-A and is not affiliated with Samsung.

GalaxyTTY exposes SMS conversations from an authorized Samsung Galaxy through a Go CLI and Bubble Tea TUI on macOS. It reads Android Content Providers over ADB; it does not install an Android application.

## Prerequisites

- macOS
- adb from Android platform-tools
- One authorized Samsung Galaxy connected through USB debugging, or an already-connected Wireless Debugging target
- Samsung Messages (com.samsung.android.messaging)
- scrcpy is optional and only reported by msg doctor; GalaxyTTY does not start it in Phase 3-A

GalaxyTTY never runs adb pair. Pair or authorize the device yourself before starting it.

## Real read mode

Real mode is the default:

```sh
go run ./cmd/msg doctor
go run ./cmd/msg
go run ./cmd/msg conversations
go run ./cmd/msg conversations --json
go run ./cmd/msg unread --json
go run ./cmd/msg messages 42 --json
```

When one physical Galaxy appears through both USB and Wireless ADB, GalaxyTTY groups the endpoints by the device-reported hardware serial and selects USB by default. Select an exact already-authorized ADB target when multiple physical Galaxies are present:

```sh
go run ./cmd/msg --device synthetic-adb-target conversations
```

The mock, json, and device flags may appear before or after a subcommand.

## Read-only safety

Real sending is deliberately disabled:

```sh
go run ./cmd/msg send --to synthetic-recipient --text 'synthetic text'
# msg: sending is not available yet (Phase 3-A read-only mode)
```

The real TUI displays “Sending is not available yet.” and keeps the composer text. Phase 3-A does not execute send intents, tap/text/key events, clipboard synchronization, Content Provider writes, device pairing, unlock, wake, package changes, APK installation, or scrcpy display/control commands.

Mock sending remains available for development.

## Mock mode

```sh
go run ./cmd/msg --mock
go run ./cmd/msg conversations --mock
go run ./cmd/msg unread --mock --json
go run ./cmd/msg messages 1 --mock
go run ./cmd/msg send --mock --to 01012345678 --text 'synthetic message'
go run ./cmd/msg doctor --mock
```

The fixture data is synthetic. Mock mode uses the same application service and presentation layers as real mode.

| Key | Action |
| --- | --- |
| Enter | Open the selected conversation or submit the composer |
| Esc | Return from chat to the conversation list |
| /help | Show keyboard help |
| /exit, /quit | Gracefully shut down |
| Ctrl+C | Gracefully shut down |

The q key remains ordinary composer input, not a quit shortcut.

## Configuration

The optional TOML file is loaded from XDG_CONFIG_HOME/galaxytty/config.toml, or ~/.config/galaxytty/config.toml.

```toml
[connection]
prefer_usb = true
device = "synthetic-adb-target"

[polling]
interval = "1s"

[notifications]
enabled = true
show_when_focused = false
```

Omit connection.device for automatic selection. A CLI device flag overrides the file. Missing files use defaults; malformed files and unknown fields return an error.

## Architecture

```text
TUI / CLI
    |
Application Service
    |
Domain Ports
    |
ADB Target / Android Provider / Read-only Sender
```

- internal/app owns conversation targeting, unread filtering, polling baseline, notification policy, status, and lifecycle.
- internal/adb owns exec.CommandContext, ADB output parsing, USB/Wireless classification, and device selection.
- internal/provider owns read-only content query construction and SMS/conversation/contact mapping.
- internal/bootstrap assembles mock or real adapters below the same application API.
- internal/doctor performs non-sensitive capability checks.
- internal/tui and internal/cli do not execute ADB commands directly.

Provider queries use explicit projections. SMS bodies and conversation snippets are only read for the requested application view and are never stored by GalaxyTTY. Doctor does not project bodies, phone numbers, contact names, or snippets.

## Current provider scope

Phase 3-A supports:

- SMS conversations and unread state
- Canonical-address participant mapping
- Contact-name resolution with normalized Korean phone numbers
- SMS history with incoming/outgoing direction
- Incremental SMS polling using message IDs
- USB and already-connected Wireless ADB targets
- MMS metadata/parts accessibility reporting

MMS and MMS-part providers are accessible on the reference device, but their separate ID namespace is not merged into the SMS polling cursor. Standard-provider RCS extension fields were not sufficient on the reference device, and restricted Samsung RCS providers are not queried. MMS history merging and RCS reading remain deferred.

## Development

```sh
gofmt -w cmd internal
go mod tidy
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/msg
```

The default test suite is hardware-independent. The explicitly gated integration test performs read-only checks against the currently discovered Galaxy:

```sh
GALAXYTTY_REAL_READ_TEST=1 go test -tags=integration ./internal/integration -run TestRealGalaxyReadPath -v
```

It does not send a message and does not print message bodies, phone numbers, or contact names.

## Deferred to Phase 3-B

- scrcpy Virtual Display startup
- Samsung Messages Virtual Display control
- Unicode clipboard paste
- display-specific input
- send button interaction
- actual SMS/RCS sending and sent-row verification

Native notifications, attachment viewing, release packaging, background daemons, and helper APKs are also outside Phase 3-A.

Licensed under MIT.
