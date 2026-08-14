# Keep Production Sends Free of Display Power Mutation

## Context

Phase 3-B debugging introduced scrcpy `--keep-active`, a virtual-display wake
key (`KEYCODE_WAKEUP`, 224), main-display state reads, and a cleanup sleep key
for display 0. Phase 3-C changed the default to a headless-like `intent_body`
session and removed those actions from the production sender.

Reference environment for the recorded observations:

- Samsung Galaxy SM-A376N
- Android 16
- One UI 8.5
- Samsung Messages `com.samsung.android.messaging`
- scrcpy 4.1

## Previous assumption

The Phase 3-B debugging sequence assumed the virtual display might need an
explicit wake event and that display 0 could be restored by sleeping it during
cleanup.

PREVIOUS ASSUMPTION: `--keep-active`, wake key 224, or a cleanup sleep might be
required for reliable virtual-display control.

## Live observation

Earlier reference-device work controlled Samsung Messages while the physical
display remained off and locked. Later wake-key experiments showed that the
Samsung firmware power group can briefly activate logical display 0. An
unconditional cleanup sleep can also race with a user who turns on and unlocks
the phone during a send.

CURRENT OBSERVATION: the power mutations have a demonstrated physical-display
side effect and no current production caller requires them. Read-only
`dumpsys trust`, `dumpsys window policy`, and `dumpsys power` checks remain in
gated integration tests to detect regressions.

The first actual Phase 3-C SMS regression later reached the send/verification
path after successful virtual-display startup, Samsung Messages role checks,
and provider baseline capture. Verification timed out. No SMS row newer than
the baseline matched the diagnostic attempt, no matching MMS text evidence was
found, Samsung Messages showed no outgoing message from the attempt, and no
persistent draft was visible.

CURRENT OBSERVATION: successful virtual-display startup alone does not prove
that Samsung Messages has a focused, input-capable window on that display.

CONCLUSION: the explicit Android wake/sleep mutation remains rejected, but the
assumption that production interaction is reliable without `--keep-active` is
not yet validated. Compare the current arguments against an otherwise
identical `--keep-active` variant without tapping Send before selecting a
production change.

### Current mode observation

The gated non-sending diagnostic started scrcpy without `--keep-active`, parsed
a runtime display ID, and kept the Samsung Messages task and focused
application associated with that display. The virtual display reported `OFF`
after the startup delay, after package-targeted SENDTO with `sms_body`, and
after the composer-focus input. Samsung Messages was not resumed on the
virtual display, there was no focused Samsung Messages window, and composer
readiness was false.

### Keep-active observation

The otherwise identical diagnostic with only `--keep-active` added produced
the same result. The virtual display reported `OFF`; the Samsung task and
focused application were associated with the display, but no resumed Samsung
activity or focused Samsung window appeared. Composer readiness remained
false. scrcpy 4.1 documents `--keep-active` as simulating user activity to keep
the screen on, but it did not change this virtual-display state on the
reference device.

### Focus parser correction

The first diagnostic parser treated any matching `FocusedDisplayId`,
`FocusedApplications`, or `FocusedWindows` entry in `dumpsys input` as current
state. That could incorrectly accept entries nested below `FocusRequests:`.
The parser now reads only the current snapshot before that section and reports
current focused display, application, and window separately. Composer readiness
also now requires a resumed Samsung activity in addition to display `ON`, the
Samsung task, and all three current-focus signals.

The corrected reference-device output still had current focused display and
Samsung application on the runtime display, but display `OFF`, no resumed
Samsung activity, and no current focused Samsung window. The correction makes
the previous result stricter; it does not support restoring `--keep-active`.

### Video playback observation

The next gated non-sending comparison used the production intent-body arguments
as the current variant. The playback variant removed only
`--no-video-playback`; it did not add `--keep-active`, a window size or border
option, a wake/sleep action, a coordinate change, or a send action.

Both variants produced the same reference-device result: virtual display `OFF`
after startup, SENDTO, and composer focus; Samsung task and current focused
application on the runtime display; no resumed Samsung activity; no current
focused Samsung window; composer readiness false. The sanitized result was
`VIDEO_PLAYBACK_DIFFERENCE=BOTH_NOT_READY`.

scrcpy documents `--no-video-playback` as disabling host video playback and
documents `--record` separately. The result therefore ends the playback/window
hypothesis for this device rather than assuming playback must activate the
virtual display. See the upstream [virtual display documentation](https://github.com/Genymobile/scrcpy/blob/v4.1/doc/virtual-display.md)
and [device power documentation](https://github.com/Genymobile/scrcpy/blob/v4.1/doc/device.md).

## Decision

Production scrcpy arguments continue to omit `--keep-active`; the single-variable
A/B diagnostic found no readiness benefit. The Samsung controller does not
expose methods for waking the virtual display, sleeping display 0, or querying
display power as part of a send. The sender never injects key codes 224 or 223.

The current headless-like path is not considered send-ready on the reference
device merely because scrcpy starts. Video playback did not supply the missing
active display or focused window. Before another actual-send regression, the
next investigation must isolate virtual-display power/activation itself without
adding an Android power mutation in this phase.

The integration tests may read main-display lock and power state before and
after a gated display/send run. They do not mutate that state.

## Why

The smallest mutation set avoids activating the user's physical screen and
eliminates the cleanup race that could turn off a phone the user intentionally
unlocked. Removing dead controller methods also prevents an accidental future
caller from reintroducing the behavior without an explicit design change.

## Evidence

- Sender and bootstrap regression tests assert zero wake/sleep mutations.
- scrcpy argument tests assert the intent-body mode contains
  `--no-video-playback` and omits `--keep-active` and window arguments.
- Integration display/send tests compare sanitized lock and power state before
  and after the virtual-display lifecycle.
- Repository caller audit found the removed controller methods referenced only
  by their own unit tests.
- On 2026-08-14, the gated SM-A376N virtual-display smoke passed with scrcpy
  4.1 using the production intent-body arguments. It resolved a runtime logical
  display ID, remained healthy, preserved the observed main-display state, and
  stopped without a new temporary recording.
- The first actual Phase 3-C SMS regression performed one send action and did
  not retry. Its exact SMS verification timed out, and subsequent read-only SMS,
  MMS, Samsung Messages, and draft observations found no evidence that the
  diagnostic message had been created or sent.
- `TestRealSamsungComposerDiagnostic` passed on the reference device. Both
  variants accepted package-targeted SENDTO and composer-focus commands without
  invoking a send action. Both reported the Samsung task and focused application
  on the runtime display, but display state `OFF`, no resumed Samsung activity,
  no focused window, and composer readiness false.
- The diagnostic used an explicitly supplied fictional reserved recipient and
  synthetic Unicode marker. It found no matching outgoing SMS or MMS evidence
  and no matching SMS-provider row of any type.
- Each variant stopped and reaped scrcpy, removed its runtime display, removed
  its temporary recording, and preserved the observed physical-display state.
- Current-focus parser regression tests prove that `FocusRequests:` entries do
  not count as current focused display, application, or window state.
- `TestRealSamsungVideoPlaybackComposerDiagnostic` passed on the reference
  device without invoking a send action. Removing only `--no-video-playback`
  produced no readiness difference and left no scrcpy process, recording, or
  virtual display. Accessible provider correlation again reported no actual
  message creation.

## Alternatives rejected

- Keeping unused wake/sleep methods for possible future use preserves an
  intrusive capability with no production requirement.
- Unconditionally sleeping display 0 during cleanup can override direct user
  interaction.
- Adding a larger Android power-manager abstraction is unnecessary while no
  mutation is required.
- Restoring `--keep-active` is rejected because the controlled A/B produced no
  display, task, focus, or composer-readiness improvement.
- Tapping Send to distinguish UI readiness is rejected because display and
  focused-window state can be observed without creating a carrier message.
- Restoring host video playback in production is rejected because the
  single-variable A/B did not improve virtual-display state, activity, focus,
  or composer readiness and would add a visible-host-window dependency.
- KEYCODE wake/power, `cmd display power-on`, private scrcpy control, and helper
  APK experiments are deferred to a separate virtual-display activation task.

## Revisit condition

Before any actual-send retry, design one separately approved non-sending
virtual-display activation investigation. Do not re-run playback or
`--keep-active` unless Android, One UI, Samsung Messages, or scrcpy updates
produce different evidence. Any explicit Android power reintroduction must
separately record pre-send and current lock/power state, avoid sleeping an
unlocked user session, and include a regression test for the exact minimal
mutation required.
