# GalaxyTTY v2 progress

## Baseline (2026-08-24)

- Branch at investigation start: `main` at `3971923`.
- `main` matched `origin/main` after `git fetch --prune origin`.
- Working tree was clean.
- PR #4 was already closed without merge; its remote branch still existed.
- Baseline `go test ./...`, `go test -race ./...`, `go vet ./...`, and
  `go build ./cmd/msg` passed.
- Wireless ADB reported one connected `SM-A376N`.
- Existing layering is retained: `internal/domain`, `internal/app`,
  `internal/tui`, `internal/cli`, and `internal/mock` remain the stable core.

## Real-device safety

Read-only device inspection, APK update installation, notification metadata,
and redacted logcat checks are permitted during development. No real SMS/RCS/MMS
send has been authorized for this work.

## Implemented after baseline

- Modern single-screen conversation/chat TUI with no outer boxes or direction
  arrows, cell-aware Korean/emoji wrapping, whitespace alignment, and local
  `/search` navigation.
- Lazy history paging, stable scroll anchors, stale-result protection, and
  cancellation of active UI work during shutdown.
- Native Kotlin Helper project with a redacted Samsung Messages notification
  observer and RemoteInput shape analysis. The production reply execution gate
  remains hard-disabled.
- Versioned length-prefixed JSON codecs, heartbeat, request correlation,
  sequence-gap reporting, reconnect, DNS-SD discovery, and remote Go adapters.
- The Mac reports a connection only after a valid Helper `HELLO`, and shutdown
  cancellation owns and closes the persistent client connection.
- A read-only Android TCP/NSD PoC that exposes only HELLO/PING/PONG and redacted
  notification metadata before pairing.
- Wi-Fi network callbacks that bind NSD to an available Android `Network`,
  re-register on network replacement, retry transient registration failures,
  and unregister callbacks when the foreground runtime stops.
- Android Keystore-backed pairing credentials, fresh per-session challenges,
  cross-platform HMAC-SHA256 proof verification, and a private mode-`0600` Mac
  credential store. Protocol commands are rejected until authentication.

## Current external validation state

- Go unit/race tests for the TUI, protocol, reconnect, discovery seams, event
  stream, and remote adapters pass.
- `SM-A376N` is connected again over Wireless ADB and reports Android 16/API 36.
- Android Command-line Tools, Platform 36, and Build Tools 36.0.0 are installed;
  Helper unit tests and `assembleDebug` pass.
- Helper `0.1.0-poc` is update-installed and running on the reference device.
- Notification Access is granted and Android has bound the Helper listener.
  The Helper chooses an ephemeral TCP port, and the Mac doctor command discovers
  it through DNS-SD and completes a read-only `HELLO` session.
- The Helper subsequently observed one active Samsung Messages notification.
  It exposed action index 2 with one `RemoteInput`, semantic action 1
  (`SEMANTIC_ACTION_REPLY` on API 36), and no authentication requirement.
- This proves a Samsung Messages notification reply action exists without
  executing it. The observed notification was not classified as chat+ versus
  SMS, so chat+-specific behavior and actual reply acceptance remain unverified.
- Helper `0.2.0-poc` moves TCP/NSD ownership into a `remoteMessaging`
  foreground service with an ongoing status notification, explicit user
  start/stop, sticky process recovery, and remembered boot/app-update restart.
  No permanent wake lock is used.
- Foreground-service policy tests, Android unit tests, debug APK assembly, and
  Android lint pass. The version is update-installed with Notification Access
  preserved, and its first explicit user start succeeded.
- A five-minute locked-screen soak sampled the foreground service plus a fresh
  DNS-SD discovery and TCP `HELLO` every 30 seconds. All 11 samples passed with
  the screen locked; no permanent wake lock was used.
- Android NSD and macOS system DNS-SD advertised/resolved the correct dynamic
  port, but the prior raw-multicast Go resolver timed out against an already
  registered service. macOS discovery now uses the built-in `dns-sd` process
  through `exec.CommandContext`; parser/unit/race tests and live automatic
  discovery pass. The raw Go resolver remains the non-macOS fallback.
- After the user reported a second, likely RCS notification, the system's
  active Samsung Messages notification count increased from one to two. The
  unlocked Helper screen contained three in-memory observations: one
  non-message-shaped record with no actions, followed by two `category=msg`
  records. Both message-shaped records exposed three actions and exactly one
  reply candidate at action index 2, with one `RemoteInput`, semantic action 1
  (`SEMANTIC_ACTION_REPLY`), and no authentication requirement.
- The new likely-RCS sample therefore exposes the same notification reply shape
  as the earlier sample. RCS is user-provided context rather than a
  protocol-derived classification, and notification shape alone does not prove
  Samsung Messages will accept or successfully send a reply. No action was
  invoked during either observation.
- Helper `0.3.0-poc` survived an `adb install -r` update without reopening the
  activity: the remembered foreground service restarted, selected a new TCP
  port, received the current Wi-Fi callback, registered NSD, and completed a
  fresh automatic Mac discovery/HELLO session.
- Helper `0.4.0-poc` adds mandatory challenge authentication. Before pairing,
  the Mac doctor command was rejected with `pairing is required`. After the
  user entered the code locally, the credential file existed with mode `0600`;
  two independent doctor processes then discovered the Helper and authenticated
  successfully with distinct TCP sessions. The device remained in a
  `remoteMessaging` foreground service and logged three successful sessions.
- An actual Wi-Fi off/on transition was not forced because that would also
  sever the active Wireless ADB development link. Network replacement and
  fallback behavior are unit-tested; a real Wi-Fi transition remains an
  explicit non-sending device check.
- Helper `0.5.0-poc` completes that transport gate. The Helper returns a
  credential-bound server proof, both peers derive separate client-to-server
  and server-to-client keys with HKDF-SHA256, and all post-authentication frames
  use AES-256-GCM with direction-bound additional data and monotonic counters.
  Plaintext, reordered, replayed, modified, and fake-server frames are covered
  by hardware-independent rejection tests.
- The existing Android Keystore key and Mac credential survived the update.
  On the reference Galaxy, the foreground service restarted on a new dynamic
  port, NSD re-registered, and `msg --helper doctor` completed automatic
  discovery, mutual proof verification, and an immediate encrypted PING/PONG.
  No notification action or message send was executed.
- Helper `0.6.0-poc` adds a data-minimized Samsung notification content parser.
  It prefers Android `MessagingStyle.Message` bundles, ignores structured
  current-user entries, uses title/text only as a fallback, and never logs or
  renders private content in the Helper diagnostics. Duplicate updates are
  suppressed and retained state is memory-only, bounded to 100 conversations
  and 200 messages per conversation.
- Notification-derived transport is represented as `unknown`; the likely-RCS
  user context is not promoted into an unverified protocol fact. The existing
  active device notification was parsed via the structured `messaging_style`
  route. The Mac then completed automatic discovery, mutual authentication,
  encrypted `GET_CONVERSATIONS`, and encrypted `GET_MESSAGES`; count-only
  validation found one cached conversation with one message.
- Content-bearing `MESSAGE_RECEIVED` decoding through the application service
  into Bubble Tea is hardware-independently covered, including duplicate merge,
  closed-channel handling, and disabling the legacy poll tick while the event
  stream is active. A newly arriving physical Samsung notification was not
  available in this phase, so notification-to-TUI latency on the reference
  device remains unmeasured. No notification reply or real message send was
  executed.
- The first physical push attempt revealed `NetworkOnMainThreadException` at
  the Helper socket write because `NotificationListenerService` delivered its
  callback on Android's main thread. The failed event was not reported as a
  latency result. Helper `0.6.1-poc` now dispatches unsolicited events through
  an ordered single-thread background network queue.
- A privacy-safe `msg --helper latency` probe calibrates Galaxy/Mac wall-clock
  offset using seven encrypted PING/PONG samples and waits for one new
  content-bearing event without printing content or identifiers. On the next
  real Samsung Messages notification, calibration RTT was 11 ms, estimated
  Galaxy-minus-Mac clock offset was 47 ms, and notification-post to Mac event
  decode latency was 303 ms. No `NetworkOnMainThreadException` recurred. The
  303 ms result meets the 0.1-0.5 second engineering target but does not include
  the final Bubble Tea Update/render and terminal redraw interval.
- The first non-sending Phase F slice adds an optional
  `ConversationMessageSender` domain port. The application service uses it for
  notification-backed conversations with no Mac-visible phone participant; the
  remote implementation maps it to `SEND_REPLY(threadId,text)`. Legacy phone
  senders and their tests remain unchanged.
- Helper `0.7.0-poc` can select and retain an active authentication-free,
  free-form Samsung RemoteInput action, expire it when its notification is
  removed, validate `SEND_REPLY`, and map fake action acceptance only to
  `accepted_unverified`. Production wiring always uses
  `DisabledReplyExecutionPolicy`, and unit tests prove that this policy never
  invokes the registered action. No real reply action was executed.
- Go unit/race tests, vet, build, and Android unit/lint/build passed for this
  slice. After Wireless ADB was reconnected, Helper `0.7.0-poc` was
  update-installed without removing app data. Notification Access remained
  granted, the foreground bridge restarted, DNS-SD discovery found the new
  dynamic port, and the Mac completed a mutually authenticated encrypted
  doctor session.
- The connected device had zero active Samsung Messages notification records
  after this install, so live registration of an action by the new registry was
  not observed. The safe diagnostic now records only the boolean
  `replyActionRegistered`; it does not log content, phone numbers, notification
  keys, or RemoteInput keys. No notification action or message send was
  executed.
- Wireless ADB mDNS serials can contain spaces (for example, a duplicate-name
  suffix). The deploy and device-test scripts now parse the tab-delimited
  `adb devices` format, so such a connected device is not miscounted as zero.
- Helper `0.8.0-poc` moves the first historical SMS read into Android. A
  permission-gated `ContentResolver` repository maps conversations, canonical
  addresses, incoming/outgoing messages, read state, and bounded pagination to
  the existing encrypted protocol DTOs. It does not request Contacts, infer
  RCS, add a database, or merge opaque notification threads with Provider
  threads without evidence.
- The SMS repository is composed with the bounded notification cache; missing
  permission or a runtime Provider failure leaves live notification reads
  available. Unit tests cover permission gating, query bounds, mapping,
  ordering, and fallback. Helper `0.8.0-poc` was update-installed, preserving
  Notification Access, the foreground bridge, discovery, pairing, and encrypted
  doctor connectivity.
- After the user approved `READ_SMS` on the reference Galaxy, the read-only
  device check reported the permission granted while Notification Access and
  the foreground bridge remained active. The Mac then completed automatic
  discovery, mutual authentication, and encrypted conversation/history reads.
  The conversation query reached its configured bound, and a non-empty sample
  page mapped entirely to SMS with a valid direction. Validation output
  contained counts only—no SMS body, participant, title, phone number, or
  identifier. No notification action or message send was executed.
