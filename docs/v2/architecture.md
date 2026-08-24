# GalaxyTTY v2 architecture

GalaxyTTY v2 keeps the existing Go, Bubble Tea, domain, application-service,
CLI, and mock layers. It replaces only the Android access adapters after the
new path is proven.

```text
Samsung Messages
        |
Galaxy Helper (Kotlin / native Android)
        |
length-prefixed encrypted JSON over authenticated local TCP
        |
Go remote adapters
        |
Application Service
        |
Bubble Tea TUI / CLI
```

The Galaxy is the TCP server and advertises `_galaxytty._tcp.local` through
Android NSD. The Mac is the client. A manually supplied address is a diagnostic
fallback, not the normal user experience.

## Phase 3-C disposition

PR #4 (`feat/phase-3c-stabilization`) was already closed without merge when the
v2 work began. Its changes were reviewed individually rather than cherry-picked.

The following transport-independent TUI behavior remains useful and is being
reimplemented against the redesigned UI:

- page-based history scrolling and lazy `BeforeID` loading;
- preservation of the visible history anchor when new events arrive;
- return to the latest message after a verified successful send;
- cancellation of active TUI work before shutdown;
- resize, duplicate suppression, and mock-backed regression tests.

The ADB/provider tolerance, scrcpy lifecycle, coordinate control, Samsung role,
legacy send-mode, and Mac-side transport evidence changes are intentionally not
carried into v2. They remain in legacy adapters only until replacement paths are
verified.

## Delivery phases and gates

1. Redesign the TUI without changing domain or application-service ownership.
2. Add a Helper notification-observation PoC. It may inspect notification and
   RemoteInput metadata but must not execute a reply.
3. Add versioned framing, `HELLO`, `PING`, `PONG`, and an authenticated,
   encrypted `MESSAGE_RECEIVED` event over local TCP.
4. Put remote store/sender adapters below the application service and feed push
   events into Bubble Tea. Retire polling only after reconnect behavior is
   covered.
5. Add pairing/authentication, sequence recovery, Android background lifecycle,
   sync, and only then transport-specific send fallbacks inside the Helper.
6. Remove Mac-side legacy packages only after real-device replacement evidence
   and regression coverage exist.

## Conversation reply boundary

Notification-backed conversations do not expose a verified phone number to the
Mac. The application service therefore uses an optional
`ConversationMessageSender` port for transports that can address an existing
opaque thread. The remote adapter maps that port to `SEND_REPLY`; legacy senders
continue to resolve a one-to-one participant and use their existing address
path.

The Helper owns every Samsung-specific object. It retains a bounded mapping from
thread ID to an active free-form RemoteInput action and removes it with the
notification. Reply execution defaults to blocked in every build. The Galaxy
user may enable or revoke normal replies through a persistent local preference;
the preference is evaluated at every dispatch. Debug builds may additionally
consume one private-file marker that expires after 60 seconds and is deleted
before the action runs.
The Mac test also requires explicit environment gates and an authenticated
capability preflight. PendingIntent acceptance is `accepted_unverified` until
independent outgoing evidence exists.

If that active action has expired, the Helper may resolve the selected opaque
thread through the existing one-to-one SMS Provider conversation or a safe
phone URI retained from its notification. It posts a privacy-safe local
notification whose activity PendingIntent opens Samsung Messages with an
`ACTION_SENDTO` draft. This respects Android background activity launch limits
and leaves the final send under explicit Galaxy user control. The Mac receives
`user_action_required`; it never promotes the draft to an outgoing message.

The Go application service owns a memory-only accepted outbox. It overlays an
outgoing bubble after `accepted_unverified` so UI refreshes do not hide an RCS
reply merely because the SMS Provider has no row. The bubble is explicitly
marked unverified and is removed only when a matching outgoing source record is
observed. It is not stored across process restarts and does not upgrade the send
result.

No default test command may send a real message. Any RemoteInput, `ACTION_SENDTO`,
or Accessibility send test requires all of the following explicit gates:

```text
GALAXYTTY_REAL_DEVICE_TEST=1
GALAXYTTY_ENABLE_SEND_TEST=1
GALAXYTTY_TEST_RECIPIENT=...
GALAXYTTY_TEST_TEXT=...
GALAXYTTY_ALLOW_SOLE_ACTIVE_REPLY=1  # only when the label is not exported verbatim
```

Send success must describe the evidence actually observed. In particular, an
accepted RemoteInput action is not itself proof that an RCS message was sent.

## SMS history boundary

The v2 history adapter runs inside the Helper. A read-only `SmsRepository`
queries Android's SMS conversations, canonical addresses, and bounded message
pages only after the user grants `READ_SMS`. A composite repository exposes
those records together with the in-memory notification cache through the same
`GET_CONVERSATIONS`/`GET_MESSAGES` contract, so the Go application and TUI do
not depend on Android APIs.

No Contacts permission or local database is introduced. Provider query failure
falls back to the live notification cache. Notification-derived opaque thread
IDs and SMS Provider thread IDs are not automatically merged because there is
not yet reliable cross-source identity evidence; a duplicate conversation is
preferable to joining unrelated conversations.

## Sequence recovery boundary

The Helper retains the most recent 512 event sequence records in memory. A
record exists even when a notification event contains only redacted shape
metadata, which lets the Helper prove whether an entire missing range is
available. `SYNC_REQUEST` and its correlated `SYNC_MESSAGE` run only inside the
authenticated encrypted session.

The Mac requests gaps immediately, including ranges learned from HELLO after a
reconnect, and suppresses duplicate recovered message IDs. If the bounded
journal has evicted part of the range, the Helper may include message-store
fallback records but returns `complete=false`; the TUI then performs its normal
conversation/history refresh. A sequence epoch moving backward means the
Helper process restarted and also triggers a full refresh rather than a false
replay-success claim.

## Connection lifecycle boundary

The Mac resolves the Helper service before every dial rather than caching the
first IP and dynamic port for the process lifetime. Disconnects, DNS-SD misses,
dial failures, and heartbeat timeouts enter a bounded exponential retry loop;
once any authenticated session has succeeded, subsequent attempts are exposed
as `reconnecting` rather than an initial `connecting` state.

The Remote Client implements the domain-level status provider and event source.
The Application Service forwards those status events, and the TUI subscribes to
them independently from message events. This keeps TCP and DNS-SD out of the UI
while allowing connection indicators to update immediately without polling the
message store.

The HMAC challenge performs mutual credential confirmation. All subsequent
protocol envelopes use directional HKDF-SHA256 session keys and AES-256-GCM
frames with monotonic counters. Notification content parsing, bounded
in-memory read sync, and the encrypted Mac adapter path have passed their first
non-sending device gate. SMS Provider history is implemented behind an Android
runtime permission and still requires its first count-only device validation.
Reliable transport classification and every messaging command still require
independent evidence gates.
