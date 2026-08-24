package com.galaxytty.helper.notification

import android.app.Notification
import android.os.Build
import android.os.Bundle
import android.service.notification.StatusBarNotification
import com.galaxytty.helper.samsung.AndroidRemoteInputReplyAction
import com.galaxytty.helper.samsung.ReplyAction
import com.galaxytty.helper.security.PairingCredential

class SamsungMessagesAdapter(
    private val analyzer: NotificationAnalyzer = NotificationAnalyzer(),
    private val contentParser: NotificationContentParser = NotificationContentParser(PairingCredential::secret),
) {
    fun observe(statusBarNotification: StatusBarNotification): NotificationObservation? =
        analyzer.analyze(snapshot(statusBarNotification))

    fun replyAction(statusBarNotification: StatusBarNotification): ReplyAction? {
        if (statusBarNotification.packageName != NotificationAnalyzer.SAMSUNG_MESSAGES_PACKAGE) return null
        return statusBarNotification.notification.actions
            ?.asSequence()
            ?.filter { action ->
                Build.VERSION.SDK_INT < Build.VERSION_CODES.S || !action.isAuthenticationRequired
            }
            ?.mapNotNull { action ->
                val inputs = action.remoteInputs
                    ?.filter { remoteInput -> remoteInput.allowFreeFormInput }
                    .orEmpty()
                if (inputs.isEmpty()) return@mapNotNull null
                val pendingIntent = action.actionIntent ?: return@mapNotNull null
                val replyPriority = if (
                    Build.VERSION.SDK_INT >= Build.VERSION_CODES.P &&
                    action.semanticAction == Notification.Action.SEMANTIC_ACTION_REPLY
                ) {
                    1
                } else {
                    0
                }
                replyPriority to AndroidRemoteInputReplyAction(pendingIntent, inputs.toTypedArray())
            }
            ?.maxByOrNull { (priority, _) -> priority }
            ?.second
    }

    private fun snapshot(statusBarNotification: StatusBarNotification): NotificationSnapshot {
        val notification = statusBarNotification.notification
        val extras = notification.extras
        val threadKey = notification.shortcutId
            ?.takeIf(String::isNotBlank)
            ?: statusBarNotification.key
        val messageShaped = notification.category == Notification.CATEGORY_MESSAGE ||
            extras?.containsKey(Notification.EXTRA_MESSAGES) == true
        val groupSummary = notification.flags and Notification.FLAG_GROUP_SUMMARY != 0
        return NotificationSnapshot(
            packageName = statusBarNotification.packageName,
            keyFingerprint = SafeFingerprint.of(statusBarNotification.key),
            postedAtMillis = statusBarNotification.postTime,
            category = notification.category,
            extrasKeys = extras?.keySet()?.toSet().orEmpty(),
            actions = notification.actions
                ?.mapIndexed(::actionSnapshot)
                .orEmpty(),
            message = if (messageShaped && !groupSummary) {
                runCatching {
                    contentParser.capture(
                        threadKey = threadKey,
                        conversationTitle = extras?.charSequence(Notification.EXTRA_CONVERSATION_TITLE),
                        notificationTitle = extras?.charSequence(Notification.EXTRA_TITLE),
                        postedAtMillis = statusBarNotification.postTime,
                        candidates = messagingCandidates(extras),
                        fallbackText = extras?.charSequence(Notification.EXTRA_TEXT),
                    )
                }.getOrNull()
            } else {
                null
            },
        )
    }

    private fun messagingCandidates(extras: Bundle?): List<NotificationTextCandidate> {
        if (extras == null || Build.VERSION.SDK_INT < Build.VERSION_CODES.R) return emptyList()
        @Suppress("DEPRECATION")
        val bundles = extras.getParcelableArray(Notification.EXTRA_MESSAGES)
        return runCatching {
            Notification.MessagingStyle.Message.getMessagesFromBundleArray(bundles)
                .mapNotNull { message ->
                    val body = message.text?.toString()?.trim().orEmpty()
                    if (body.isEmpty()) return@mapNotNull null
                    val sender = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
                        message.senderPerson?.name?.toString()
                    } else {
                        @Suppress("DEPRECATION")
                        message.sender?.toString()
                    }
                    NotificationTextCandidate(
                        body = body,
                        senderLabel = sender,
                        postedAtMillis = message.timestamp,
                        fromCurrentUser = sender.isNullOrBlank(),
                    )
                }
        }.getOrDefault(emptyList())
    }

    private fun Bundle.charSequence(key: String): String? =
        getCharSequence(key)?.toString()?.trim()?.takeIf(String::isNotEmpty)

    private fun actionSnapshot(index: Int, action: Notification.Action): ActionSnapshot =
        ActionSnapshot(
            index = index,
            semanticAction = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) action.semanticAction else 0,
            contextual = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.Q) action.isContextual else false,
            authenticationRequired = if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.S) {
                action.isAuthenticationRequired
            } else {
                false
            },
            remoteInputs = action.remoteInputs
                ?.map { remoteInput ->
                    RemoteInputSnapshot(
                        resultKeyFingerprint = SafeFingerprint.of(remoteInput.resultKey),
                        allowsFreeFormText = remoteInput.allowFreeFormInput,
                        choiceCount = remoteInput.choices?.size ?: 0,
                        allowedDataTypeCount = remoteInput.allowedDataTypes.size,
                    )
                }
                .orEmpty(),
        )
}
