# GalaxyTTY Phase 3-A Real Galaxy Read Integration Design

## Context

GalaxyTTY Phase 2 already separates Bubble Tea and CLI presentation from `app.Service`, domain ports, and mock adapters. Phase 3-A adds real Samsung Galaxy reads beneath those ports without replacing that architecture. The runtime remains foreground-only and read-only.

The reference device is a Samsung Galaxy SM-A376N running Android 16 and One UI 8.5. The application must discover its current ADB target at runtime; neither the known USB serial nor a wireless endpoint may be embedded in source, tests, documentation examples, or defaults.

## Goals

- Make `msg` use a real Galaxy by default while preserving `msg --mock`.
- Discover one eligible Samsung Galaxy over USB or an already-connected Wireless ADB transport.
- Read SMS conversations, participants, contact titles, message histories, unread state, and incremental SMS updates through Android Content Providers.
- Expose actionable diagnostics through `msg doctor`.
- Show USB, Wireless, or Offline status in the TUI.
- Make every real-mode send attempt fail before any device write or UI-input operation can occur.
- Keep unit tests independent of physical hardware and perform a separate read-only smoke test against the attached device.

## Non-goals and safety boundary

Phase 3-A does not execute SMS/RCS sending, `SENDTO`, `adb input`, clipboard commands, package mutation, settings mutation, pairing, unlock, PIN entry, display wake, APK installation, scrcpy startup, or Content Provider insert/update/delete. It does not install a daemon or helper application.

`msg send` in real mode returns `domain.ErrSendingNotImplemented`. The real TUI uses the same sender, so pressing Enter with composed text cannot fall through to any ADB command other than read-only provider access performed elsewhere.

MMS and RCS must not destabilize the SMS path. MMS provider accessibility and row relationships are diagnosed after SMS is operational, but MMS history is not merged into the Phase 3-A SMS cursor because SMS and MMS use separate provider ID namespaces. Restricted Samsung RCS providers are not queried or bypassed; only accessible standard-provider extension fields may be inspected structurally. Full MMS history merging and RCS mapping remain deferred unless a later design introduces an explicit cross-provider cursor.

## Live facts verified on 2026-08-14

The following facts were verified read-only on the current local machine and device without retaining personal values:

- ADB 1.0.41 / platform-tools 37.0.1 is installed at `/opt/homebrew/bin/adb`.
- scrcpy 4.1 is installed at `/opt/homebrew/bin/scrcpy`; Phase 3-A only inspects its version.
- The same physical SM-A376N appears as an authorized USB target with `usb:` metadata and as an already-connected `_adb-tls-connect._tcp` target.
- `adb mdns services` is supported but currently returns no discovered services, so normal runtime selection relies on `adb devices -l`. No automatic `adb connect` or `adb pair` is added.
- Samsung Messages package `com.samsung.android.messaging` is installed.
- SMS, simple conversations, canonical addresses, and contacts phone providers accept the required projections.
- SMS `date` and conversation `date` values are Unix milliseconds on this device.
- Observed SMS types are `1` (incoming) and `2` (outgoing).
- Group `recipient_ids` are space-delimited.
- `_id DESC LIMIT 1` and numeric `WHERE` clauses work when the multi-word remote argument is transmitted with literal quotes.
- Android's `content` command can print `[ERROR]` while the enclosing `adb shell` process exits successfully. Provider output must therefore be checked independently of exit status.

## Architecture

```text
cmd/msg
  -> internal/cli
       -> internal/bootstrap
            -> internal/app.Service
                 -> internal/domain ports
                      <- internal/mock (mock mode)
                      <- internal/provider.Store (real reads)
                      <- internal/readonly.Sender (real send rejection)
                      <- internal/adb.Target (connection and shell)
       -> internal/doctor.Service
            -> ADB, provider probes, scrcpy inspection

internal/tui -> internal/app.API only
```

`internal/adb` owns host-process execution and device selection. `internal/provider` owns Android `content query` construction and mapping. `internal/bootstrap` is the only package that knows how concrete adapters are assembled. CLI and TUI operate on application-level interfaces and results.

No generic dependency-injection container, command DSL, event bus, database, plugin system, or additional repository abstraction is introduced.

## ADB client and command boundary

`adb.Client` resolves an executable, runs commands with `exec.CommandContext`, applies a finite timeout, and captures stdout and stderr separately. Its public operations are:

```go
type Client struct { /* private executable, timeout, and runner */ }

func NewClient(path string, timeout time.Duration) (*Client, error)
func (c *Client) Version(ctx context.Context) (string, error)
func (c *Client) Devices(ctx context.Context) ([]adb.Device, error)
func (c *Client) MDNSServices(ctx context.Context) ([]adb.MDNSService, error)
func (c *Client) Shell(ctx context.Context, target string, args ...string) ([]byte, error)
```

Production uses an `execRunner`; tests inject a runner that records argv and returns synthetic stdout/stderr. `Shell` passes the selected target using `-s` and never uses host `sh -c`.

The client maps executable lookup failures, context cancellation/deadline, unauthorized/offline target errors, and generic non-zero exits into recognizable errors while retaining safe stderr context. It never includes provider row data in formatted errors.

## Device parsing and selection

`ParseDevices` parses the header and each `adb devices -l` record into target, raw state, and metadata. USB classification uses the presence of `usb:` metadata. Wireless classification uses the `_adb-tls-connect._tcp` service suffix or a valid host-and-port target when no USB metadata is present; it does not classify every colon-containing string as wireless.

Discovery works as follows:

1. Read `adb devices -l`.
2. If an explicit CLI/config target is supplied, inspect only that exact target and return an actionable state error if it is unauthorized, offline, or absent.
3. For authorized candidates, query manufacturer, model, hardware serial, and Samsung Messages package presence.
4. Exclude non-Samsung devices and Samsung devices without the Messages package.
5. Group endpoints by hardware serial so USB and Wireless endpoints for the same phone are one eligible Galaxy.
6. If more than one physical Galaxy remains, return `ErrMultipleDevices` rather than selecting arbitrarily.
7. For one physical Galaxy with both endpoints, select USB when `prefer_usb=true` and Wireless when it is false. If only one endpoint is usable, select it.

Already-connected wireless targets therefore work without pairing or reconnect side effects. mDNS results are diagnostic only in Phase 3-A.

`adb.Target` implements the existing `domain.Device` port and exposes immutable `domain.DeviceInfo`. It also maintains a small cached status. Successful shell reads mark it connected; recognized disconnect, offline, and authorization errors update the cached state so application status can become Offline without a busy-looping probe.

## Provider query boundary

`provider.Store` depends only on a device shell interface:

```go
type Sheller interface {
    Shell(context.Context, ...string) ([]byte, error)
}
```

A focused query helper accepts a URI, ordered projection, numeric-only selection generated inside the adapter, and a fixed sort clause. It emits:

```text
content query --uri URI --projection a:b:c [--where "..."] [--sort "..."]
```

The literal remote quotes are part of the argument given to `adb shell`; this matches the verified local behavior. Dynamic thread/message IDs are parsed as `int64` before query construction. Device-derived contact or message strings are never interpolated into remote shell clauses.

Output handling checks, in order:

- propagated ADB errors;
- `Permission Denial`, `SecurityException`, or permission-denied text;
- `[ERROR]` content-tool output, even after a zero process exit;
- `No result found.` as an empty result;
- strict projected-row parsing for all other non-empty output.

Free-form values are always the final projected column: `body` for SMS, `snippet` for conversations, `address` for canonical addresses, and `display_name` for contacts.

## SMS MessageStore

The real store implements all existing `domain.MessageStore` methods.

### `Messages(threadID, query)`

- Projection: `_id:thread_id:address:date:type:read:body`.
- Selection: `thread_id = N`, plus `_id < BeforeID` when requested.
- Sort: `_id DESC LIMIT N`, where the default limit is 200 and the maximum accepted limit is 1000.
- Mapping: parse integer fields strictly; use `time.UnixMilli`; map type 1 to incoming and type 2 to outgoing; set `MessageSMS`; reverse the provider result so callers receive oldest-to-newest order.

### `LatestMessageID()`

- Projection: `_id`.
- Sort: `_id DESC LIMIT 1`.
- Return zero when no rows exist.
- Never fetch the complete SMS table.

### `MessagesAfter(lastID)`

- Projection: the full SMS projection above.
- Selection: `_id > N`.
- Sort: `_id ASC LIMIT 500`.
- Return increasing IDs. If a burst exceeds 500 rows, the existing poller advances to the last returned ID and retrieves the next page on the next tick.

The application poller's current baseline semantics remain unchanged: initialization records the latest SMS ID without replaying old messages.

## Conversation, canonical address, and contact mapping

Conversation query:

- URI: `content://mms-sms/conversations?simple=true`.
- Projection: `_id:recipient_ids:unread_count:date:snippet`.
- Sort: `date DESC`.

Canonical query:

- URI: `content://mms-sms/canonical-addresses`.
- Projection: `_id:address`.

Contact query:

- URI: `content://com.android.contacts/data/phones`.
- Projection: `data1:data4:display_name`.

`recipient_ids` uses `strings.Fields`, which supports the verified space delimiter and arbitrary surrounding whitespace. Every ID resolves through the canonical map. Contact `data1` and `data4` are normalized with `domain.NormalizePhone` and indexed in memory. Contact rows are loaded once per process; conversation and canonical metadata are refreshed only at startup and after polling reports new SMS rows. This avoids re-reading contacts or all messages on every one-second tick.

Each participant is retained. A contact display name becomes the participant title when available; otherwise the address is used. Group titles join all resolved participant titles in provider order. Unknown canonical IDs remain visible as a non-sensitive `Unknown participant` label rather than causing the whole conversation list to fail.

Phone normalization handles national digits, punctuation, `+82 10...`, and the common `+82 (0)10...` form without storing or logging actual phone values.

## Runtime wiring and configuration

CLI startup performs:

```text
config.Path -> config.Load -> argument override -> bootstrap mock/real
```

Missing config retains defaults. Read errors, unknown TOML fields, invalid types, and invalid durations return an explicit error. `connection.device` is optional; `--device TARGET` overrides it. `connection.prefer_usb` remains true by default.

`bootstrap.Mock` builds the existing mock backend, sender, notifier, and lifecycle. `bootstrap.Real` builds one selected `adb.Target`, `provider.Store`, `readonly.Sender`, no-op notifier, lifecycle, and `app.Service`. Both return the same application API.

`go mod tidy` generates and retains `go.sum`; no new third-party dependency is needed.

## CLI behavior

Supported real commands are:

- `msg doctor`
- `msg conversations [--json]`
- `msg unread [--json]`
- `msg messages THREAD_ID [--json]`
- `msg send ...`, which returns the read-only unsupported error without attempting device discovery or ADB execution
- `msg`, which starts the real TUI

`--mock`, `--json`, and `--device TARGET` are accepted before or after the subcommand. JSON commands write exactly one JSON value to stdout. Returned errors are printed by `cmd/msg` to stderr.

## Doctor behavior

`doctor.Service` returns structured checks which the CLI formats. It checks:

- ADB executable and version/server invocation;
- parsed devices and selected Galaxy/connection kind;
- Samsung Messages package presence;
- one-ID projections for SMS, conversations, contacts, MMS, and MMS parts;
- structural accessibility of standard-provider RCS extension columns without exposing values;
- scrcpy executable and version only.

Doctor queries do not project message bodies, phone numbers, contact names, or snippets. A failed required check makes `Read mode not ready`; otherwise the summary is `Read mode ready. Sending not implemented yet.` MMS and RCS checks are informational and do not block SMS readiness.

## TUI behavior

The existing Bubble Tea model remains. It receives real conversations and SMS history through `app.API`, renders unread markers and incoming/outgoing arrows, and schedules the existing one-second poll command. A non-empty poll result refreshes conversation metadata and, when a chat is open, that thread's history.

The header label is `USB`, `Wireless`, or `Offline`. Provider/ADB errors are rendered as concise status text rather than stack traces. Poll errors schedule the next normal tick and never start a retry loop. Real send errors leave the composer text intact and show `Sending is not available yet.` Mock sends continue to append and refresh normally.

## Error model

Sentinel or typed errors cover:

- ADB executable not found;
- no ADB targets;
- unauthorized target;
- offline target;
- multiple eligible physical Galaxy devices;
- Samsung Messages missing;
- provider permission denial;
- malformed or content-tool error output;
- unavailable wireless mDNS discovery;
- context cancellation/deadline;
- real sending not implemented.

The CLI wraps these with one actionable sentence. It does not include raw provider rows in errors. For example, no eligible device advises enabling USB debugging or connecting an already-paired Wireless Debugging device.

## Privacy

No live provider output is written to repository files, fixtures, README examples, logs, or error strings. Unit tests use synthetic names, numbers, message bodies, serials, and endpoints. Manual smoke reporting records only structural counts and success/failure states and never includes bodies, phone numbers, or contact names.

## Testing strategy

Unit tests use injected process runners and shellers. `go test ./...` does not require ADB or hardware. Coverage includes:

- mock phone-to-thread send regression;
- config loading at CLI startup and device override precedence;
- ADB argv, timeout/cancellation, stdout/stderr, and error mapping;
- devices parser, connection classification, physical-device dedupe, preference, selector, and multiple-device errors;
- provider query argv including literal remote quotes;
- SMS row/date/direction/read mapping and malformed output;
- history order, limit, before-ID, latest-ID, and after-ID behavior;
- recipient parsing, canonical mapping, phone normalization, contact fallback, and group title mapping;
- provider permission/content-tool errors;
- doctor readiness and non-sensitive projections;
- CLI JSON purity and real send rejection without runtime construction;
- TUI status and read-only send feedback;
- existing mock CLI/TUI behaviors.

After unit checks pass, a separate manual integration sequence runs only the approved read operations against the currently selected target. It verifies provider access, real CLI output structurally, locked/screen-off reads if the device is already in that state, and two consecutive polling reads without creating a message. Incremental polling command behavior is verified against a baseline with zero-or-more naturally arriving messages; the test does not cause a message to be sent.

## Acceptance criteria

Phase 3-A is accepted when real default mode discovers the current Galaxy, `doctor` reports SMS read readiness, real conversation and SMS history commands work, the TUI displays real read data and connection kind, polling uses incremental SMS IDs, real sending is impossible, mock regressions pass, and `go test ./...`, `go test -race ./...`, `go vet ./...`, and `go build ./cmd/msg` all succeed.
