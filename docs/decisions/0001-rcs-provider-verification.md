# Preserve RCS Evidence Without Weakening Send Verification

## Context

GalaxyTTY delegates transport choice to Samsung Messages. The production send
path must report success only after exact, machine-readable provider evidence,
while the explicitly gated RCS integration test also needs to inspect what the
reference device exposes after a single authorized send action.

Reference environment for the recorded observation:

- Samsung Galaxy SM-A376N
- Android 16
- One UI 8.5
- Samsung Messages `com.samsung.android.messaging`
- scrcpy 4.1

## Previous assumption

The first RCS integration test assumed `SendToAddress` had to return success
before post-send provider extensions could be inspected. A verification timeout
therefore ended the test immediately, even though the UI send action might
already have occurred.

PREVIOUS ASSUMPTION: production verification had to succeed before the test
could collect useful RCS observations.

## Live observation

The reference Galaxy exposes candidate columns such as `teleservice_id`,
`app_id`, `chat_type`, and `correlation_tag` through the SMS provider. The
observed accessible values have not established a reliable SMS-versus-RCS
discriminator. A UI tap alone is not evidence of successful delivery or of the
selected transport.

CURRENT OBSERVATION: the accessible provider shape is useful for sanitized
post-send investigation but is insufficient to classify RCS reliably.

CONCLUSION: preserve the observation after a verification timeout without
changing production success semantics.

## Decision

The gated RCS integration test invokes `SendToAddress` exactly once. When that
single call returns `ErrSendVerificationTimeout`, the test performs one
read-only query for rows after the pre-send baseline, correlates exact recipient
and body values in memory, and inspects supported extension fields for the
correlated row. It never prints the values.

Non-verification controller, ADB, or provider failures remain test failures. An
exact outgoing row without a reliable RCS discriminator remains SKIP /
unsupported. Production `SendToAddress` continues to reject an unverified tap.

## Why

This preserves evidence from the only user-authorized send action without
retrying or falsely converting an ambiguous UI action into verified success. It
also keeps recipient, body, and raw extension values out of test output and the
repository.

## Evidence

- Integration regression tests prove a verification timeout triggers one
  observation after exactly one send call.
- A controller failure bypasses observation and remains an error.
- Synthetic provider rows prove correlation requires a newer exact recipient
  and body, and only Android SMS type `2` is called outgoing.
- The live RCS send remains separately gated by
  `GALAXYTTY_ENABLE_RCS_SEND_TEST` and `GALAXYTTY_RCS_TEST_RECIPIENT`.

## Alternatives rejected

- Treating a successful send-button tap as success weakens the production
  verification contract.
- Retrying after timeout can send a duplicate message.
- Assigning RCS from a merely non-empty extension field would encode an
  undocumented guess.
- Reading restricted Samsung providers would require a permission bypass.

## Revisit condition

Revisit when Android, One UI, Samsung Messages, or scrcpy changes provider
behavior, or when repeated controlled SMS and Chat+ observations establish a
stable, independently testable RCS discriminator accessible without elevated
permissions.
