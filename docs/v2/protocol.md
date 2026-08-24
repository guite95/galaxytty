# GalaxyTTY local protocol v1

Every frame is a four-byte unsigned big-endian payload length followed by one
UTF-8 JSON envelope. The current maximum frame is 1 MiB.

```json
{
  "version": 1,
  "sequence": 1842,
  "type": "MESSAGE_RECEIVED",
  "requestId": "correlation-id",
  "payload": {}
}
```

`requestId` correlates a request and response. Unsolicited events use a
monotonically increasing `sequence`; the Mac reports a gap and requests a sync
rather than silently assuming it received every event.

Known message types are:

```text
HELLO AUTH SECURE PING PONG MESSAGE_RECEIVED
GET_CONVERSATIONS CONVERSATIONS GET_MESSAGES MESSAGES
SEND_MESSAGE SEND_REPLY SEND_RESULT
SYNC_REQUEST SYNC_MESSAGE ERROR
```

There are deliberately no `SEND_SMS`, `SEND_RCS`, or `SEND_MMS` commands.
Samsung Messages and the Helper own transport selection.

## Current PoC boundary

The Helper advertises itself through `_galaxytty._tcp.local` and chooses an
ephemeral `ServerSocket(0)` port. Each connection begins with `HELLO` containing
a random 32-byte challenge and a persistent, non-hardware device ID. The Mac
sends `AUTH` with a client ID and HMAC-SHA256 proof over a domain-separated
transcript containing the device ID, client ID, and challenge. The 160-bit
base32 pairing credential is never transmitted over TCP.

The Helper's correlated `AUTH` response contains its own HMAC proof, providing
mutual key confirmation rather than trusting a bare `authenticated: true`
value. Failed or missing authentication closes the session, and an
unauthenticated session is never installed as the live notification consumer.

`HELLO.payload.eventSequence` reports the latest Helper event sequence. On an
initial connection it establishes the Mac baseline. On reconnect, a higher
value immediately identifies the missing range even if no new notification
arrives afterward. A lower value indicates a Helper process/sequence epoch
restart and requires a full read refresh.

After `AUTH`, both peers use HKDF-SHA256 with the shared credential and the
32-byte session challenge to derive separate client-to-server and
server-to-client AES-256 keys plus four-byte nonce prefixes. Each inner protocol
envelope is encrypted into an outer JSON frame:

```json
{
  "version": 1,
  "type": "SECURE",
  "payload": {
    "counter": 1,
    "ciphertext": "base64url-aes-gcm-ciphertext"
  }
}
```

The 96-bit GCM nonce is the direction-specific four-byte prefix followed by the
eight-byte network-order counter. The direction and counter are also bound as
additional authenticated data. A receiver accepts exactly the next counter;
plaintext, replayed, reordered, modified, or nested `SECURE` frames terminate
the session. Event sequence numbers remain a separate application-level gap
recovery mechanism.

The Helper emits redacted notification-shape events with
`contentIncluded: false` when no new incoming text can be extracted. When a
new structured or fallback text message is available, it emits the following
only inside the authenticated encrypted session:

```json
{
  "contentIncluded": true,
  "message": {
    "id": 1845493760000000001,
    "threadId": 731,
    "address": "display label",
    "body": "message text",
    "postedAt": 1777000000000,
    "direction": "incoming",
    "read": false,
    "messageType": "unknown",
    "attachments": []
  }
}
```

For notification events, `id` and `threadId` are Helper-local opaque
identifiers. `address` currently
contains only the notification's display label and must not be treated as a
verified phone number. `messageType` is deliberately `unknown`: notification
shape does not establish SMS, MMS, or RCS transport. `unreadCount` is currently
zero because notification presence is not authoritative Samsung provider read
state.

`GET_CONVERSATIONS` and `GET_MESSAGES` return a combined view after
authentication. It always includes the bounded in-memory notification cache.
When the user has granted `READ_SMS`, it also includes up to 500 SMS Provider
conversations and bounded SMS message pages; those records use the Provider's
native IDs, addresses, direction, read state, and `messageType: "sms"`. The
Helper advertises either `sms-history` or
`sms-history-permission-required` in `HELLO`. Notification and Provider thread
IDs are not guessed to be equivalent.

For a detected gap where sequence 10 and 11 are missing before sequence 12, the
Mac sends the following only inside the secure session:

```json
{
  "version": 1,
  "type": "SYNC_REQUEST",
  "requestId": "correlation-id",
  "payload": {
    "fromSequence": 10,
    "throughSequence": 11,
    "afterMessageId": 1845493760000000001,
    "limit": 500
  }
}
```

The Helper responds once with correlated `SYNC_MESSAGE` containing `items`,
the echoed range, a non-sensitive `source`, and `complete`. `complete: true`
means every event sequence in the requested range was retained, including
contentless events. If the 512-event journal cannot prove that, the response is
`complete: false`; message-store fallback items may still be included, and the
Mac requests a normal full read refresh instead of assuming recovery succeeded.

`SEND_MESSAGE` and unimplemented send fallbacks
still receive `ERROR` with `POC_READ_ONLY`. No unauthenticated peer can invoke
read commands or receive message content. Reply execution remains blocked in
every build until the Galaxy user enables the local permission. Debuggable APKs
also retain a fresh private one-shot marker for deliberately gated integration
tests.

`GET_REPLY_CAPABILITY` accepts only an opaque `threadId` and returns a correlated
`REPLY_CAPABILITY` containing `available: true|false` and
`cachedAvailable: true|false`. `available` means the Samsung notification is
currently active. `cachedAvailable` means a genuine Samsung action was retained
after its notification disappeared and has not reached its 24-hour TTL. The
response reveals no title, number, body, action key, or execution-gate state and
does not consume the one-shot marker.

`SEND_REPLY` addresses the selected opaque conversation rather than a transport
or recipient. An active notification action is preferred when available:

```json
{
  "version": 1,
  "type": "SEND_REPLY",
  "requestId": "correlation-id",
  "payload": {
    "threadId": 731,
    "text": "reply text"
  }
}
```

The default policy returns a correlated `SEND_RESULT` with `outcome: "failed"`
before invoking any PendingIntent. A locally authorized or explicitly armed
debug action maps to `accepted_unverified` with
`remote_input_pending_intent_accepted` evidence. The Mac treats this as a
completed action, clears the composer to prevent an accidental duplicate, and
visibly reports that delivery remains unverified. The application service keeps
a session-only outgoing echo so an RCS reply does not disappear when the Helper
history has no outgoing record. It is reconciled only against a matching,
time-bounded outgoing source record and is never treated as delivery evidence.

A retained Samsung action uses the same outcome with
`retained_remote_input_pending_intent_accepted` evidence. Samsung can cancel the
underlying PendingIntent; rejection evicts it and attempts the compose handoff.
The retained action is bounded, memory-only, and cannot help a conversation
whose reply-capable notification was never observed by the current Helper
process.

When no active RemoteInput action exists, a locally authorized request may use
the Helper's existing one-to-one conversation address to post a generic compose
handoff notification. Its correlated result is:

```json
{
  "version": 1,
  "type": "SEND_RESULT",
  "requestId": "correlation-id",
  "payload": {
    "outcome": "user_action_required",
    "evidence": "samsung_compose_notification_posted",
    "threadId": 731
  }
}
```

The user must tap the Helper notification, review the prefilled Samsung
Messages composer, and tap send. This outcome creates no outgoing echo and is
not send or delivery evidence.

The secure channel provides confidentiality, integrity, replay protection, and
mutual credential confirmation for protocol frames. Notification cache state
is not durable and remains bounded to 100 conversations and 200 messages per
conversation. SMS history remains owned by Android's existing Provider; the
Helper adds no database.

When sending is enabled later, `SEND_RESULT.outcome` must be one of:

- `verified`: outgoing evidence was observed and the Mac may report success;
- `accepted_unverified`: an action was accepted but transport evidence was not
  available; the Mac reports acceptance and an unverified delivery state;
- `user_action_required`: Samsung Messages composition was prepared on the
  Galaxy, but the user must review and send it;
- `failed`: the Helper observed a failure.
