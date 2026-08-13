# GalaxyTTY

**Samsung Messages in your terminal.**

> [!WARNING]
> GalaxyTTY is an early-stage project. It is not affiliated with, endorsed by, or sponsored by Samsung.

GalaxyTTY will expose Samsung Messages through the `msg` CLI/TUI on macOS. This phase provides a hardware-independent application service, a Bubble Tea TUI, and fixture-backed development mode; it does **not** send real messages.

## Try the mock

```sh
go run ./cmd/msg --mock
go run ./cmd/msg conversations --mock
go run ./cmd/msg unread --mock --json
go run ./cmd/msg messages 1 --mock
go run ./cmd/msg send --mock --to 01012345678 --text '지금 출발합니다.'
go run ./cmd/msg doctor --mock
```

The mock command opens an interactive Bubble Tea terminal UI. The mock renders image attachments as `🖼 이미지` and exercises the same application service that future real adapters will use.

| Key | Action |
| --- | --- |
| `Enter` | Open the selected conversation or send the composer text |
| `Esc` | Return from chat to the conversation list |
| `/help` | Show keyboard help |
| `/exit`, `/quit` | Gracefully shut down |
| `Ctrl+C` | Gracefully shut down |

`q` is ordinary composer input, never a quit shortcut. Other registered slash commands currently report `not implemented yet` rather than pretending to perform device operations.

## Architecture

`internal/domain` defines models and hardware ports. `internal/app` owns conversation targeting, polling baseline, notification decisions, status, and lifecycle policy. `internal/tui` and `internal/cli` call the application API only; `internal/mock` supplies fixture ports below it. Provider, scrcpy, and Samsung packages are isolated adapter seams. Neither presentation layer executes ADB or scrcpy or resolves recipients.

The program is foreground-only: no daemon or LaunchAgent is installed. Configuration defaults to `$XDG_CONFIG_HOME/galaxytty/config.toml`, or `~/.config/galaxytty/config.toml`, and is optional. Samsung UI coordinates are a replaceable layout preset because app updates may invalidate them.

## Hardware status and roadmap

The reported reference environment is Samsung Galaxy / Samsung Messages, Android 16, One UI 8.5, scrcpy 4.1, and macOS. It is a reference, not a general compatibility promise. Future production runtime dependencies are `android-platform-tools` and `scrcpy`.

Real-device work intentionally remains: real USB/already-paired wireless ADB selection, Android Content Provider adapter, scrcpy Virtual Display manager, Samsung Messages controller/direct conversation intent, clipboard/paste sender, display-specific input, actual sent-row verification, native macOS notifications, MMS/RCS attachment viewer, and a production Homebrew release. No automatic wireless pairing, Android helper APK, private scrcpy protocol, or background daemon is planned.

## Release shape

The GoReleaser skeleton builds the installed `msg` binary for Darwin arm64 and amd64. A future separate `homebrew-tap` repository will provide the `galaxytty` formula (`brew install <tap>/galaxytty`).

## Development

```sh
make fmt
make test
make vet
make build
```

Licensed under MIT.
