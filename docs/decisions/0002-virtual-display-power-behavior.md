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

## Decision

Production scrcpy arguments continue to omit `--keep-active` while the
non-sending A/B diagnostic is pending. The Samsung controller does not expose
methods for waking the virtual display, sleeping display 0, or querying
display power as part of a send. The sender never injects key codes 224 or 223.

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

## Alternatives rejected

- Keeping unused wake/sleep methods for possible future use preserves an
  intrusive capability with no production requirement.
- Unconditionally sleeping display 0 during cleanup can override direct user
  interaction.
- Adding a larger Android power-manager abstraction is unnecessary while no
  mutation is required.

## Revisit condition

Complete the gated current-versus-`--keep-active` composer diagnostic on the
reference device. Restore `--keep-active` only if the single-variable comparison
shows a reproducible display, activity, focus, or composer-readiness difference.
Any explicit Android power reintroduction must separately record pre-send and
current lock/power state, avoid sleeping an unlocked user session, and include a
regression test for the exact minimal mutation required. Re-run the decision
after Android, One UI, Samsung Messages, or scrcpy updates.
