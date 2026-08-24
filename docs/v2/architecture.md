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
notification. The production registry is constructed with a hard-disabled
execution policy. Tests may inject an enabled policy only with fake actions.
Even after a future authorized action execution, PendingIntent acceptance is
`accepted_unverified` until independent outgoing evidence exists.

No default test command may send a real message. Any RemoteInput, `ACTION_SENDTO`,
or Accessibility send test requires all of the following explicit gates:

```text
GALAXYTTY_REAL_DEVICE_TEST=1
GALAXYTTY_ENABLE_SEND_TEST=1
GALAXYTTY_TEST_RECIPIENT=...
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

The HMAC challenge performs mutual credential confirmation. All subsequent
protocol envelopes use directional HKDF-SHA256 session keys and AES-256-GCM
frames with monotonic counters. Notification content parsing, bounded
in-memory read sync, and the encrypted Mac adapter path have passed their first
non-sending device gate. SMS Provider history is implemented behind an Android
runtime permission and still requires its first count-only device validation.
Reliable transport classification and every messaging command still require
independent evidence gates.
