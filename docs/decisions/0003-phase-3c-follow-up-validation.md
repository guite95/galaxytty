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

CURRENT OBSERVATION FOR THE ORIGINAL FOLLOW-UP: none of the SMS, RCS, or MMS
enable-and-recipient gate pairs was set in the process environment. No real
message was attempted during that run.

A later explicitly gated Phase 3-C SMS regression performed one send action and
did not retry. The virtual display started, Samsung Messages held the default
SMS role, readiness and provider baseline checks passed, and the sender reached
the send/verification path. Verification timed out. Read-only follow-up found
no matching SMS row newer than the baseline, no matching MMS text part, no
outgoing message visible in Samsung Messages, and no persistent draft from the
attempt.

Immediately before that regression, one provider baseline query timed out near
the five-second ADB command limit. Subsequent read-only SMS queries covering all
rows and the supported history types completed in approximately 1.3 to 1.7
seconds, and doctor again reported Read and Send ready. This is recorded as a
transient observation, not as evidence for automatic retries or a global
timeout increase.

CURRENT OBSERVATION: the changed Phase 3-C production send path is not yet
validated. A successful display lifecycle smoke does not establish that the
Samsung Messages composer owns a focused, input-capable window on the runtime
virtual display.

The subsequent non-sending current-versus-`--keep-active` diagnostic isolated
that variable. Both modes started and parsed a runtime display ID. Both kept a
Samsung Messages task and focused application associated with the virtual
display, accepted SENDTO and composer-focus commands, but reported display
state `OFF`, no resumed Samsung activity, no focused Samsung window, and
composer readiness false. `--keep-active` produced no difference.

The focus parser was then hardened to read only the current `dumpsys input`
snapshot before `FocusRequests:`. It no longer treats stale/request entries as
current focus, and composer readiness now requires current focused display,
current Samsung application, current Samsung window, and a resumed Samsung
activity. The corrected current observation retained current focused display and
application, but display `OFF`, no resumed Samsung activity, and no current
focused Samsung window.

The following non-sending current-versus-video-playback diagnostic removed only
`--no-video-playback` from the playback variant. Both variants produced the
same not-ready state and `VIDEO_PLAYBACK_DIFFERENCE=BOTH_NOT_READY`. No message
send, wake/sleep injection, coordinate change, clipboard action, or verifier
change occurred.

CONCLUSION: retain the READ and virtual-display lifecycle PASS results, record
the single SMS regression as an unverified failed attempt with no retry, and do
not restore `--keep-active`. The next investigation must remain non-sending and
isolate virtual-display activation before another actual-send regression.
RCS and MMS actual-send regressions remain SKIP.

## Decision

Do not synthesize or infer a recipient and do not enable an actual-send test on
the user's behalf. Do not retry the failed Phase 3-C SMS attempt. Compare
current and `--keep-active` virtual-display behavior without tapping Send and
without restoring explicit Android wake/sleep injection. Keep the Phase 3-B SMS
evidence as historical evidence, not proof of the changed Phase 3-C path. RCS
and MMS remain unsupported/unverified by this follow-up run.

The A/B results reject both `--keep-active` and host video playback as the
production fix. Keep production arguments unchanged until a separately designed
non-sending virtual-display activation test has evidence for another minimal
change.

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
- The later explicitly gated SMS regression produced no matching accessible
  provider or Samsung Messages evidence and was not retried.
- The non-sending composer diagnostic passed both variants and reported
  `KEEP_ACTIVE_DIFFERENCE=BOTH_NOT_READY`.
- Provider correlation reported `ACTUAL_MESSAGE_CREATED=false`; cleanup left no
  scrcpy process, temporary recording, or virtual display, and the physical
  display remained `OFF` in the post-cleanup observation.
- Current-focus parser tests prove that `FocusRequests:` cannot make composer
  readiness true.
- The playback diagnostic passed with
  `VIDEO_PLAYBACK_DIFFERENCE=BOTH_NOT_READY`, no actual outgoing evidence, and
  the same clean scrcpy/display cleanup.

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

Design one gated non-sending virtual-display activation investigation under a
new explicit scope. Only after a variant proves a focused, input-capable Samsung
Messages window should one fresh actual-send regression be designed under its
own explicit approval and gate. Update this record after Android, One UI,
Samsung Messages, scrcpy, layout coordinates, or provider shape changes.
