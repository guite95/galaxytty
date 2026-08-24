# GalaxyTTY Helper

Native Kotlin Android Helper for GalaxyTTY v2. The current implementation is a
real-time notification, read-only SMS-history, and locally authorized reply PoC.

It accepts only notifications from `com.samsung.android.messaging` and records
the following redacted diagnostics:

- a short hash of the notification key;
- category and extras **key names**, never extras values;
- action count and RemoteInput capability metadata;
- hashed RemoteInput result keys.

Message bodies, phone numbers, notification titles, pairing credentials, and
reply text are not logged. Remote reply execution is blocked by default and can
be enabled only through an explicit confirmation on the Galaxy itself.

For the messaging path, the Helper reads Android's structured
`Notification.MessagingStyle.Message` bundles first and falls back to the
notification title/text only when no structured text exists. It ignores
structured entries attributed to the current user, deduplicates notification
updates, and keeps at most 100 conversations with 200 messages per conversation
in memory. There is no Helper database. Content is returned by
`GET_CONVERSATIONS`/`GET_MESSAGES` or pushed by `MESSAGE_RECEIVED` only after
authentication and secure-session establishment. The notification does not
provide reliable SMS/RCS classification, so this path reports transport as
`unknown` rather than guessing.

When the user separately grants **SMS history**, the Helper reads SMS
conversations and bounded message pages with Android `ContentResolver` inside
the Galaxy. It does not query the Provider from the Mac, request Contacts, add
a database, or infer RCS records from the SMS Provider. Provider content is
returned only inside the authenticated encrypted session. Without the
permission—or if the Provider fails—the notification cache remains available.

The local TCP server and NSD advertisement are owned by a `remoteMessaging`
foreground service. After the user starts the bridge once, that choice is
remembered for normal boot and app-update recovery. The service shows an
ongoing, non-sensitive status notification; no permanent wake lock is used.
NSD follows Wi-Fi network changes and retries transient registration failures.

The Helper keeps its root pairing key in Android Keystore. Tap **Show / hide
pairing code**, then run `go run ./cmd/msg pair` on the Mac and enter the code
there. The code and HMAC proofs are never logged, and the code itself is never
sent over TCP. Every TCP session performs mutual proof verification and derives
directional AES-256-GCM keys from a new challenge. All subsequent commands and
events are encrypted and replay-protected before even the heartbeat succeeds.
Read sync and content events are enabled only inside that encrypted session.
`SEND_REPLY` remains blocked until the local user enables **Allow replies from
paired Mac**. The setting can be revoked immediately from the Helper screen.
Other send commands remain disabled.

## Build

Install Android SDK Platform 36. The repository scripts discover common macOS
SDK locations, or `GALAXYTTY_ANDROID_SDK_ROOT` can select one explicitly. Run:

```sh
./gradlew testDebugUnitTest assembleDebug
```

## Device observation

Use `../scripts/deploy-helper.sh`. Android requires the user to grant
**Settings > Notifications > Advanced settings > Notification access >
GalaxyTTY notification bridge**. GalaxyTTY does not bypass this system screen.
From the Helper screen, tap **Start background bridge** and approve bridge
notifications. This explicit visible action satisfies modern Android
foreground-service launch policy.

For existing SMS history, tap **Allow SMS history (read-only)** and approve the
Android permission prompt. The deployment script and ADB checks do not grant
this permission. Contact names are not requested in this slice; conversation
participants use the Provider address until a later, separately scoped contact
permission decision.

To reply from the Mac TUI, tap **Allow replies from paired Mac** and confirm the
warning on the Galaxy. This permits a mutually authenticated paired client to
execute the active Samsung Messages RemoteInput reply action. PendingIntent
acceptance is shown as delivery-unverified; it is not promoted to delivered or
transport-verified without independent evidence.

After access is granted, receive a Samsung Messages notification and inspect
the Helper screen or the redacted tags:

```sh
adb logcat -s GalaxyTTY GalaxyTTY-Notification
```

This can establish whether a chat notification exposes a RemoteInput action. It
does not by itself establish that Samsung Messages will accept a reply; that
requires a separately authorized real-send test.

## Reference-device evidence

On `SM-A376N` / Android 16 (API 36), after Notification Access was granted, the
Helper captured two message-shaped Samsung Messages observations. Each had
three actions and one RemoteInput reply candidate on action index 2. Its
semantic action was `SEMANTIC_ACTION_REPLY` and it did not require
authentication. The user identified the newer sample as likely chat+/RCS; the
Helper does not yet classify the underlying transport. No notification action
was invoked.

This evidence is notification-level only: the sample was not classified as SMS
or chat+, and the presence of the action does not prove that executing it would
send successfully.

Helper `0.6.0-poc` subsequently parsed the currently active sample through the
structured `MessagingStyle` path. The Mac discovered the Helper, authenticated,
and read one cached conversation and one message through the encrypted protocol.
The validation printed counts only, not the private title, sender, or body. A
new live notification has not yet been available to time the physical
notification-to-TUI push path.

The first live measurement exposed an Android threading defect: the
NotificationListener main thread attempted the socket write and Android rejected
it with `NetworkOnMainThreadException`. Helper `0.6.1-poc` moves unsolicited
events onto an ordered single-thread network queue. After update-install, the
next real structured notification reached the Mac event decoder in 303 ms with
an 11 ms encrypted clock-calibration RTT; no threading exception recurred. This
measurement ends at Mac event decode, before the final Bubble Tea redraw.

Helper `0.7.0-poc` adds the non-sending portion of the RemoteInput reply path.
For each active structured notification, it selects the authentication-free,
free-form action and retains its PendingIntent and RemoteInput result keys in a
bounded memory-only registry keyed by the opaque conversation ID. Notification
removal expires the matching action. `SEND_REPLY` validates the thread and text,
but production construction always injects `DisabledReplyExecutionPolicy`;
there is no UI or preference that enables execution. Unit tests use fake
actions to cover selection outcomes and prove the disabled policy never invokes
an action. A future explicitly authorized device test must introduce the
separate activation gate before `PendingIntent.send()` can execute.

Helper `0.8.0-poc` adds the permission-gated Android SMS history repository.
On the reference device, user-approved `READ_SMS` survived alongside
Notification Access and the foreground bridge. Automatic discovery, mutual
authentication, encrypted conversation sync, and a bounded SMS message-page
read succeeded. Device validation printed counts and structural types only;
private content and identifiers were not logged or displayed.

Helper `0.9.0-poc` adds a bounded 512-event in-memory replay journal. Every
notification observation advances the sequence, including redacted events with
no message content. Authenticated clients can request a missing range through
`SYNC_REQUEST`; `SYNC_MESSAGE.complete` is true only when every sequence in the
range is still present. A bounded message-store fallback may return useful
records but never upgrades an incomplete replay to complete.

Helper `0.10.1-poc` adds the separately gated real RemoteInput test path.
Release APKs remain unable to execute replies. A debug APK requires a private
one-shot file created through an authorized ADB test, consumes it before
`PendingIntent.send()`, and rejects it after 60 seconds. The Mac-side test also
requires device, send, recipient, and text environment gates, disables Go test
caching, and never prints private message data. If Samsung does not export the
authorized label verbatim, a separate sole-active confirmation gate is required
and succeeds only when exactly one reply-capable notification thread exists.

The first authorized device attempt exposed a caller-context bug and was
rejected before PendingIntent acceptance. After passing the NotificationListener
application context as required for a non-null fill-in Intent, Samsung accepted
the one-shot RemoteInput action. The result remains `accepted_unverified`:
there was no exact outgoing SMS Provider row, and the Helper does not infer RCS
delivery from notification behavior. The user then confirmed that Samsung
Messages showed the authorized reply as successfully sent through chat+/RCS.
That is valid human-confirmed real-device PoC evidence, but the runtime still
cannot automatically classify or promote a future accepted action to `verified`
without independent outgoing evidence.

Helper `0.11.0-poc` replaces the debug-only limitation for normal use with a
persistent, locally controlled reply permission. It defaults to blocked after
installation, is read at every dispatch so revocation takes effect immediately,
and does not weaken session authentication or encryption. The debug one-shot
gate remains separate for deliberately gated integration tests.
