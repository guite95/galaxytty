package com.galaxytty.helper.message

import android.Manifest
import android.content.ContentResolver
import android.content.Context
import android.content.pm.PackageManager
import android.net.Uri

data class SmsQuery(
    val uri: String,
    val projection: List<String>,
    val selection: String? = null,
    val selectionArgs: List<String> = emptyList(),
    val sortOrder: String? = null,
)

fun interface SmsDataSource {
    fun query(query: SmsQuery): List<Map<String, String?>>
}

class ContentResolverSmsDataSource(
    private val resolver: ContentResolver,
) : SmsDataSource {
    override fun query(query: SmsQuery): List<Map<String, String?>> {
        val cursor = resolver.query(
            Uri.parse(query.uri),
            query.projection.toTypedArray(),
            query.selection,
            query.selectionArgs.takeIf(List<String>::isNotEmpty)?.toTypedArray(),
            query.sortOrder,
        ) ?: return emptyList()
        return cursor.use {
            buildList {
                while (cursor.moveToNext()) {
                    add(
                        query.projection.associateWith { column ->
                            val index = cursor.getColumnIndexOrThrow(column)
                            if (cursor.isNull(index)) null else cursor.getString(index)
                        },
                    )
                }
            }
        }
    }
}

class SmsRepository(
    private val source: SmsDataSource,
    private val hasPermission: () -> Boolean,
) : HistoryMessageRepository {
    constructor(context: Context) : this(
        source = ContentResolverSmsDataSource(context.contentResolver),
        hasPermission = {
            context.checkSelfPermission(Manifest.permission.READ_SMS) == PackageManager.PERMISSION_GRANTED
        },
    )

    override fun available(): Boolean = hasPermission()

    override fun conversations(): List<LiveConversation> {
        if (!available()) return emptyList()
        val conversationRows = source.query(
            SmsQuery(
                uri = CONVERSATIONS_URI,
                projection = listOf("_id", "recipient_ids", "unread_count", "date", "snippet"),
                sortOrder = "date DESC LIMIT $MAX_CONVERSATIONS",
            ),
        )
        if (conversationRows.isEmpty()) return emptyList()
        val canonical = source.query(
            SmsQuery(
                uri = CANONICAL_ADDRESSES_URI,
                projection = listOf("_id", "address"),
            ),
        ).associate { row -> requiredLong(row, "_id") to row["address"].orEmpty().trim() }
        return conversationRows.map { row -> conversation(row, canonical) }
            .sortedWith(compareByDescending<LiveConversation> { it.updatedAtMillis }.thenByDescending { it.threadId })
    }

    override fun messages(query: LiveMessageQuery): List<StoredMessage> {
        if (!available()) return emptyList()
        require(query.threadId >= 0) { "thread ID cannot be negative" }
        require(query.beforeId >= 0) { "before ID cannot be negative" }
        require(query.afterId >= 0) { "after ID cannot be negative" }
        val limit = query.limit.coerceIn(1, LiveMessageQuery.MAX_LIMIT)
        val conditions = mutableListOf("type IN (?, ?)")
        val arguments = mutableListOf(INBOX_TYPE.toString(), SENT_TYPE.toString())
        if (query.threadId > 0) {
            conditions += "thread_id = ?"
            arguments += query.threadId.toString()
        }
        if (query.beforeId > 0) {
            conditions += "_id < ?"
            arguments += query.beforeId.toString()
        }
        if (query.afterId > 0) {
            conditions += "_id > ?"
            arguments += query.afterId.toString()
        }
        val requestedLimit = if (query.latest) 1 else limit
        return source.query(
            SmsQuery(
                uri = SMS_URI,
                projection = listOf("_id", "thread_id", "address", "date", "type", "read", "body"),
                selection = conditions.joinToString(" AND "),
                selectionArgs = arguments,
                sortOrder = "_id DESC LIMIT $requestedLimit",
            ),
        ).map(::message)
            .sortedWith(compareBy(StoredMessage::postedAtMillis, StoredMessage::id))
    }

    private fun conversation(
        row: Map<String, String?>,
        canonical: Map<Long, String>,
    ): LiveConversation {
        val participantIds = row["recipient_ids"].orEmpty()
            .split(Regex("\\s+"))
            .filter(String::isNotBlank)
            .map { value -> value.toLongOrNull() ?: throw IllegalArgumentException("invalid recipient ID") }
        val participants = participantIds.map { id ->
            val address = canonical[id].orEmpty()
            MessageParticipant(
                id = id,
                displayName = address.ifBlank { UNKNOWN_PARTICIPANT },
                phone = address,
            )
        }
        val title = participants
            .joinToString(", ") { participant -> participant.displayName }
            .ifBlank { UNKNOWN_PARTICIPANT }
        return LiveConversation(
            threadId = requiredLong(row, "_id"),
            title = title,
            participants = participants,
            snippet = row["snippet"].orEmpty(),
            updatedAtMillis = requiredLong(row, "date"),
            unreadCount = requiredInt(row, "unread_count").also { count ->
                require(count >= 0) { "invalid unread count" }
            },
        )
    }

    private fun message(row: Map<String, String?>): StoredMessage {
        val type = requiredInt(row, "type")
        val read = requiredInt(row, "read")
        require(read == 0 || read == 1) { "invalid SMS read flag" }
        val direction = when (type) {
            INBOX_TYPE -> "incoming"
            SENT_TYPE -> "outgoing"
            else -> throw IllegalArgumentException("unsupported SMS type")
        }
        return StoredMessage(
            id = requiredLong(row, "_id"),
            threadId = requiredLong(row, "thread_id"),
            address = row["address"].orEmpty(),
            body = row["body"].orEmpty(),
            postedAtMillis = requiredLong(row, "date"),
            direction = direction,
            read = read == 1,
            messageType = "sms",
        )
    }

    private fun requiredLong(row: Map<String, String?>, field: String): Long =
        row[field]?.toLongOrNull()?.takeIf { value -> value >= 0 }
            ?: throw IllegalArgumentException("invalid SMS field $field")

    private fun requiredInt(row: Map<String, String?>, field: String): Int =
        row[field]?.toIntOrNull()
            ?: throw IllegalArgumentException("invalid SMS field $field")

    companion object {
        private const val SMS_URI = "content://sms"
        private const val CONVERSATIONS_URI = "content://mms-sms/conversations?simple=true"
        private const val CANONICAL_ADDRESSES_URI = "content://mms-sms/canonical-addresses"
        private const val INBOX_TYPE = 1
        private const val SENT_TYPE = 2
        private const val MAX_CONVERSATIONS = 500
        private const val UNKNOWN_PARTICIPANT = "Unknown participant"
    }
}
