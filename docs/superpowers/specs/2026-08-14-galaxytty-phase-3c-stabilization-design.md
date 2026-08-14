# GalaxyTTY Phase 3-C Stabilization Design

## Scope and evidence rule

Phase 3-C stabilizes the existing read and Samsung Messages send paths. It
does not add attachment sending, full MMS/RCS history, a helper APK, private
scrcpy control, default-role mutation, or main-display unlock automation.

The reference device is an SM-A376N running Android 16 / One UI 8.5 with
Samsung Messages `com.samsung.android.messaging` and scrcpy 4.1. Live device
behavior is authoritative. Transport support is documented only when an
explicitly gated send produces machine-readable evidence. A successful UI tap
is never sufficient evidence.

## Work streams

The phase is delivered as three independently reviewable work streams:

1. Provider and TUI history robustness.
2. Samsung target, role, input, scrcpy, power, and privacy stabilization.
3. Transport observation, guarded integration tests, doctor output, and
   evidence-based documentation.

Each behavior change follows a failing-test-first cycle and ends in a local
functional commit. Existing user changes are preserved and no branch is
pushed.

## SMS provider robustness

Android SMS types are classified as follows:

- `1` is inbox and maps to `DirectionIncoming`.
- `2` is sent and maps to `DirectionOutgoing`.
- `3` draft, `4` outbox, `5` failed, and `6` queued are non-history states and
  are skipped.
- Any other type is unsupported and is skipped per row.

Malformed fields on a supported history row remain errors because silently
accepting corrupt recipient, timestamp, read, or identifier data would weaken
the read contract. History, latest-ID, and incremental queries constrain their
provider selection to types `1` and `2`; row mapping still skips known and
unknown unsupported types defensively. This prevents one unsupported row from
failing a query and prevents polling from repeatedly treating a non-history ID
as readable history.

The send verifier continues to accept only a newer type-2 outgoing message
whose normalized recipient and exact body match the request.

## TUI navigation and history viewport

The conversation list uses only arrow keys for selection. Plain `j`, `k`, and
`q` always pass through the composer, including slash-command arguments.
Enter, Esc, Ctrl+C, `/exit`, and `/quit` retain their current semantics.

The chat viewport stores a message offset measured from the latest loaded
message. Offset zero means follow the bottom. PageUp increases the offset by
one visible page, PageDown decreases it, and End resets it to zero. No plain
alphabet key is consumed for scrolling.

Opening a conversation replaces the loaded history, resets the offset, and
shows the latest message. When PageUp reaches the oldest loaded boundary, the
model requests an older page with `BeforeID` set to the oldest loaded message
ID. Older rows are prepended after ID de-duplication. Prepending does not alter
the offset, so the visual anchor is preserved; a subsequent PageUp exposes the
new page.

Poll results for the selected thread merge directly into loaded history.
While at the bottom, new rows remain visible. While scrolled up, the offset is
increased by the count of newly appended rows so the current visual messages
do not jump. Conversation refresh preserves selection by thread ID. A
successful user-initiated send resets to the latest view before reloading the
conversation history. Resize clamps, but does not reset, the offset.

## Samsung Messages target and default role

All SENDTO intents are constrained to
`com.samsung.android.messaging` with Android's public package option. The
controller never relies solely on the implicit handler.

The Samsung adapter reads the public SMS role holder through
`cmd role get-role-holders android.app.role.SMS`. Doctor reports installed and
default-handler checks separately. Send readiness requires Samsung Messages to
be the holder, and every send performs the same non-mutating role check before
starting scrcpy. GalaxyTTY never changes the role.

## Explicit text input mode

The compatibility boolean is replaced by `TextInputMode` with exactly two
values:

- `intent_body` is the default.
- `clipboard` is the compatibility path.

Configuration uses `[samsung] text_input_mode`. Missing configuration retains
the default and any other value fails validation before runtime construction.
The selected mode is passed explicitly to the sender and scrcpy manager.

## scrcpy and power behavior

Intent-body mode builds a recording-backed virtual display with local IME,
Samsung Messages startup, no audio, and no video playback. It has no playback
window, AppleScript shortcut bridge, or `--keep-active` argument.

Clipboard mode retains the tiny borderless playback window and AppleScript
shortcut bridge required for the documented scrcpy paste shortcut. These
dependencies affect doctor send readiness only when clipboard mode is
selected.

The production send sequence initially removes main-display state reads,
virtual-display wake key 224, and display-0 sleep restoration. This is the
smallest mutation set and avoids the user-interaction restore race entirely.
The guarded virtual-display integration smoke is the authority for whether the
reference device can keep this sequence. If that gated test later proves a
power action necessary, the action must be reintroduced with before/current
state and lock-aware restoration tests rather than unconditional sleep.

## Private intent delivery

Recipient and message body must not appear in repository logs, normal errors,
or host process arguments. The ADB adapter therefore provides a remote-shell
stdin operation. SENDTO commands are constructed with NUL rejection and
single-quote escaping, then written to `adb -s <target> shell` standard input;
the command is not passed as a host argv item. Controller errors retain only a
safe operation category.

Regression fixtures cover Korean, emoji, single and double quotes, dollar
signs, backticks, spaces, and newlines. No test, document, or commit contains a
real recipient or body.

## Verification and transport evidence

The existing baseline-tap-poll flow remains intentionally small. SMS proof is
a newer outgoing row with exact normalized recipient and body. Unsupported
rows, unrelated rows, incoming echoes, pre-baseline duplicates, provider
empty results, cancellation, timeout, and disconnect remain non-success.

Provider observation helpers may inspect non-sensitive SMS extension columns
and MMS message/part structure. They do not bypass provider permissions or
infer a transport from undocumented values. `SendResult` gains a backward-
compatible optional transport only when evidence can assign `sms`, `rcs`, or
`mms`; otherwise it remains omitted or unknown.

Separate integration tests require both variables for their transport:

- SMS: `GALAXYTTY_ENABLE_SEND_TEST=1` and
  `GALAXYTTY_TEST_RECIPIENT`.
- RCS: `GALAXYTTY_ENABLE_RCS_SEND_TEST=1` and
  `GALAXYTTY_RCS_TEST_RECIPIENT`.
- MMS text: `GALAXYTTY_ENABLE_MMS_SEND_TEST=1` and
  `GALAXYTTY_MMS_TEST_RECIPIENT`.

No recipient is selected from contacts or provider history, and each test
attempts at most one send. If a reliable accessible proof cannot be built,
the implementation reports verification as unsupported or times out; it does
not convert the tap into success.

## Doctor and diagnostics

Doctor remains non-mutating. It reports ADB, Galaxy connection, Samsung
Messages installation and default role, required read providers, optional MMS
and RCS extension visibility, scrcpy, text input mode, layout profile, and
mode-specific clipboard readiness. Intent-body readiness does not depend on
`pbcopy`, `pbpaste`, `osascript`, or Accessibility automation.

Verification timeout errors remain redacted and add a safe actionable hint
that the Samsung Messages layout coordinates or unsupported transport may be
responsible.

## Testing and completion

Default tests are hardware-independent. Integration tests compile with the
`integration` tag and skip unless their explicit gates are present. Completion
requires:

```text
gofmt -w cmd internal
go mod tidy
go test ./...
go test -race ./...
go vet ./...
go build ./cmd/msg
go test -tags=integration ./internal/integration
git diff --check
```

Live findings distinguish PASS, SKIP, and unsupported. SMS, RCS, and MMS text
claims are never upgraded beyond the evidence produced in the current run.
