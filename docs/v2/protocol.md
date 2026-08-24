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

`SEND_MESSAGE` and unimplemented send fallbacks
still receive `ERROR` with `POC_READ_ONLY`. No unauthenticated peer can invoke
read commands or receive message content, and the production reply execution
gate prevents every peer from invoking a PendingIntent through this PoC.

`SEND_REPLY` addresses the active notification conversation rather than a
transport or recipient:

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

The implemented production policy currently returns a correlated `SEND_RESULT`
with `outcome: "failed"` before invoking any PendingIntent. Tests can inject an
enabled policy with a fake reply action; a successful fake dispatch maps to
`accepted_unverified` with `remote_input_pending_intent_accepted` evidence.
`accepted_unverified` is intentionally surfaced as an error by the Mac adapter,
so the composer is not cleared and success is not claimed.

The secure channel provides confidentiality, integrity, replay protection, and
mutual credential confirmation for protocol frames. Notification cache state
is not durable and remains bounded to 100 conversations and 200 messages per
conversation. SMS history remains owned by Android's existing Provider; the
Helper adds no database.

When sending is enabled later, `SEND_RESULT.outcome` must be one of:

- `verified`: outgoing evidence was observed and the Mac may report success;
- `accepted_unverified`: an action was accepted but transport evidence was not
  available; the Mac reports an unverified result, not success;
- `failed`: the Helper observed a failure.
