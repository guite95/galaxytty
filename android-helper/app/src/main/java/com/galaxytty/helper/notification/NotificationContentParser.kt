package com.galaxytty.helper.notification

import java.nio.ByteBuffer
import javax.crypto.Mac
import javax.crypto.spec.SecretKeySpec

data class NotificationTextCandidate(
    val body: String,
    val senderLabel: String?,
    val postedAtMillis: Long,
    val fromCurrentUser: Boolean,
    val replyAddress: String? = null,
)

class NotificationContentParser(
    private val secretProvider: () -> ByteArray,
) {
    fun capture(
        threadKey: String,
        conversationTitle: String?,
        notificationTitle: String?,
        postedAtMillis: Long,
        candidates: List<NotificationTextCandidate>,
        fallbackText: String?,
    ): CapturedMessage? {
        if (threadKey.isBlank()) return null
        val messagingCandidate = candidates
            .asSequence()
            .filterNot { item -> item.fromCurrentUser }
            .filter { item -> item.body.isNotBlank() }
            .maxWithOrNull(compareBy<NotificationTextCandidate> { it.postedAtMillis })
        val fallbackCandidate = fallbackText
            ?.takeIf { candidates.none { item -> item.body.isNotBlank() } }
            ?.trim()
            ?.takeIf(String::isNotEmpty)
            ?.let { body ->
                NotificationTextCandidate(
                    body = body,
                    senderLabel = notificationTitle,
                    postedAtMillis = postedAtMillis,
                    fromCurrentUser = false,
                )
            }
        val candidate = messagingCandidate ?: fallbackCandidate ?: return null
        val source = if (messagingCandidate != null) {
            NotificationContentSource.MESSAGING_STYLE
        } else {
            NotificationContentSource.TEXT_FALLBACK
        }

        val timestamp = candidate.postedAtMillis.takeIf { it > 0 } ?: postedAtMillis
        val sender = firstNonBlank(candidate.senderLabel, notificationTitle, conversationTitle)
        val title = firstNonBlank(conversationTitle, notificationTitle, candidate.senderLabel)
            .ifBlank { DEFAULT_TITLE }
        val secret = secretProvider()
        return CapturedMessage(
            id = StableContentId.message(
                secret = secret,
                threadKey = threadKey,
                postedAtMillis = timestamp,
                senderLabel = sender,
                body = candidate.body,
            ),
            threadId = StableContentId.thread(secret, threadKey),
            conversationTitle = title,
            senderLabel = sender,
            body = candidate.body,
            postedAtMillis = timestamp,
            source = source,
            replyAddress = candidate.replyAddress,
        )
    }

    private fun firstNonBlank(vararg values: String?): String =
        values.firstOrNull { value -> !value.isNullOrBlank() }?.trim().orEmpty()

    companion object {
        private const val DEFAULT_TITLE = "Samsung Messages"
    }
}

object StableContentId {
    private const val HMAC = "HmacSHA256"
    private const val TIMESTAMP_BITS = 43
    private const val SUFFIX_BITS = 20
    private const val SUFFIX_MASK = (1L shl SUFFIX_BITS) - 1
    private const val TIMESTAMP_MASK = (1L shl TIMESTAMP_BITS) - 1

    fun thread(secret: ByteArray, threadKey: String): Long =
        positiveLong(digest(secret, "thread", threadKey))

    fun message(
        secret: ByteArray,
        threadKey: String,
        postedAtMillis: Long,
        senderLabel: String,
        body: String,
    ): Long {
        val suffix = positiveLong(
            digest(secret, "message", threadKey, postedAtMillis.toString(), senderLabel, body),
        ) and SUFFIX_MASK
        val timestamp = postedAtMillis.coerceAtLeast(1) and TIMESTAMP_MASK
        return (timestamp shl SUFFIX_BITS) or suffix
    }

    private fun digest(secret: ByteArray, vararg values: String): ByteArray {
        val mac = Mac.getInstance(HMAC)
        mac.init(SecretKeySpec(secret, HMAC))
        values.forEach { value ->
            val bytes = value.toByteArray(Charsets.UTF_8)
            mac.update(ByteBuffer.allocate(Int.SIZE_BYTES).putInt(bytes.size).array())
            mac.update(bytes)
        }
        return mac.doFinal()
    }

    private fun positiveLong(bytes: ByteArray): Long =
        ByteBuffer.wrap(bytes.copyOfRange(0, Long.SIZE_BYTES)).long and Long.MAX_VALUE
}
