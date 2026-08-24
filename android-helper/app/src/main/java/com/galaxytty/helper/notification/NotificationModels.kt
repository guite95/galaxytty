package com.galaxytty.helper.notification

data class NotificationSnapshot(
    val packageName: String,
    val keyFingerprint: String,
    val postedAtMillis: Long,
    val category: String?,
    val extrasKeys: Set<String>,
    val actions: List<ActionSnapshot>,
    val message: CapturedMessage?,
)

data class CapturedMessage(
    val id: Long,
    val threadId: Long,
    val conversationTitle: String,
    val senderLabel: String,
    val body: String,
    val postedAtMillis: Long,
    val source: NotificationContentSource,
)

enum class NotificationContentSource(val safeName: String) {
    MESSAGING_STYLE("messaging_style"),
    TEXT_FALLBACK("text_fallback"),
}

data class ActionSnapshot(
    val index: Int,
    val semanticAction: Int,
    val contextual: Boolean,
    val authenticationRequired: Boolean,
    val remoteInputs: List<RemoteInputSnapshot>,
)

data class RemoteInputSnapshot(
    val resultKeyFingerprint: String,
    val allowsFreeFormText: Boolean,
    val choiceCount: Int,
    val allowedDataTypeCount: Int,
)

data class NotificationObservation(
    val keyFingerprint: String,
    val postedAtMillis: Long,
    val category: String?,
    val extrasKeys: List<String>,
    val actionCount: Int,
    val replyCandidates: List<ReplyCandidate>,
    val message: CapturedMessage?,
) {
    fun safeSummary(): String = buildString {
        append("key=")
        append(keyFingerprint)
        append(" category=")
        append(category ?: "none")
        append(" extras=")
        append(extrasKeys.size)
        append(" actions=")
        append(actionCount)
        append(" replyCandidates=")
        append(replyCandidates.size)
        append(" content=")
        append(message != null)
        message?.let { captured ->
            append(" contentSource=")
            append(captured.source.safeName)
        }
    }
}

data class ReplyCandidate(
    val actionIndex: Int,
    val semanticAction: Int,
    val contextual: Boolean,
    val authenticationRequired: Boolean,
    val remoteInputs: List<RemoteInputSnapshot>,
)
