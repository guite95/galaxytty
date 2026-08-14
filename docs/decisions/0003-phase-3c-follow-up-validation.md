# Bound Phase 3-C Follow-up Claims to Current Gated Evidence

## Context

The Phase 3-C follow-up required hardware-independent review fixes plus a
regression on the reference Galaxy. Real text sends can incur carrier or data
charges and require an explicit enable flag and a separately supplied
recipient for SMS, RCS, or MMS.

Reference environment on 2026-08-14:

- Samsung Galaxy SM-A376N
- Android 16
- One UI 8.5
- Samsung Messages `com.samsung.android.messaging`
- scrcpy 4.1
- Wireless ADB connection

## Previous assumption

Phase 3-B had verified an earlier Unicode SMS path, but Phase 3-C changed the
production path to package-targeted SENDTO, ADB shell stdin, explicit
`intent_body`, `--no-video-playback`, and no keep-active or power-key mutation.
The earlier successful send could not by itself prove this new path.

PREVIOUS ASSUMPTION: a fresh Phase 3-C SMS send would be available during the
follow-up if the explicit recipient gate was already configured.

## Live observation

The hardware-independent suite passed. The explicitly invoked read regression
passed against the reference Galaxy. The intent-body virtual-display smoke
also passed: scrcpy produced a runtime Android display ID, remained healthy,
preserved the observed main-display state, terminated cleanly, and left no new
temporary recording.

Doctor reported Samsung Messages installed and holding the default SMS role,
the SMS provider and conversations accessible, the MMS provider and parts
accessible, scrcpy 4.1, `intent_body`, a 1080x1920 layout, and both Read and
Send ready. Candidate RCS extension visibility remained partial and its mapping
unsupported.

CURRENT OBSERVATION: none of the SMS, RCS, or MMS enable-and-recipient gate
pairs was set in the process environment. No real message was sent.

CONCLUSION: record READ and virtual-display lifecycle as PASS, and record all
three actual-send regressions as SKIP rather than reusing contacts, history, or
another transport's recipient.

## Decision

Do not synthesize or infer a recipient and do not enable an actual-send test on
the user's behalf. Keep the Phase 3-B SMS evidence as historical evidence, but
do not claim it as a current Phase 3-C production-send regression. RCS and MMS
remain unsupported/unverified by this follow-up run.

## Why

This follows the repository's carrier-charge and duplicate-send safety gates.
It is more accurate to preserve an explicit validation gap than to send to an
unapproved recipient or upgrade a prior-path result into current evidence.

## Evidence

- `go test ./...`, `go test -race ./...`, `go vet ./...`, and
  `go build ./cmd/msg` passed.
- `go test -tags=integration ./internal/integration -count=1 -v` compiled and
  passed its synthetic tests while all hardware gates skipped by default.
- `GALAXYTTY_REAL_READ_TEST=1` passed `TestRealGalaxyReadPath`.
- `GALAXYTTY_REAL_DISPLAY_TEST=1` passed
  `TestRealVirtualDisplaySmoke` without a recipient.
- The actual-send environment check printed only SET/UNSET status and found no
  complete SMS, RCS, or MMS gate pair.

No recipient, message body, contact, credential, or raw provider row is stored
in this record.

## Alternatives rejected

- Selecting a recipient from contacts, recent conversations, or provider
  history violates the explicit opt-in contract.
- Reusing an SMS recipient for RCS or MMS violates transport-specific consent.
- Sending a test message without both variables could incur an unapproved
  charge or contact a real person.
- Treating the Phase 3-B send as proof of the changed Phase 3-C path would
  overstate current evidence.

## Revisit condition

Run exactly one fresh regression per transport when its enable flag and
explicit recipient are both deliberately supplied. Update this record after
Android, One UI, Samsung Messages, scrcpy, layout coordinates, or provider
shape changes.
