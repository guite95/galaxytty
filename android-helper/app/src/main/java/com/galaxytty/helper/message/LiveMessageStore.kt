package com.galaxytty.helper.message

import com.galaxytty.helper.notification.CapturedMessage

data class LiveConversation(
    val threadId: Long,
    val title: String,
    val participants: List<MessageParticipant> = emptyList(),
    val snippet: String,
    val updatedAtMillis: Long,
    val unreadCount: Int,
)

data class MessageParticipant(
    val id: Long,
    val displayName: String,
    val phone: String,
)

data class StoredMessage(
    val id: Long,
    val threadId: Long,
    val address: String,
    val body: String,
    val postedAtMillis: Long,
    val direction: String,
    val read: Boolean,
    val messageType: String,
)

fun CapturedMessage.toStoredMessage(): StoredMessage = StoredMessage(
    id = id,
    threadId = threadId,
    address = senderLabel,
    body = body,
    postedAtMillis = postedAtMillis,
    direction = "incoming",
    read = false,
    messageType = "unknown",
)

data class LiveMessageQuery(
    val threadId: Long = 0,
    val limit: Int = DEFAULT_LIMIT,
    val beforeId: Long = 0,
    val afterId: Long = 0,
    val latest: Boolean = false,
) {
    companion object {
        const val DEFAULT_LIMIT = 200
        const val MAX_LIMIT = 500
    }
}

interface MessageRepository {
    fun conversations(): List<LiveConversation>

    fun messages(query: LiveMessageQuery): List<StoredMessage>
}

interface HistoryMessageRepository : MessageRepository {
    fun available(): Boolean
}

class LiveMessageStore(
    private val maxThreads: Int = 100,
    private val maxMessagesPerThread: Int = 200,
) : MessageRepository {
    private data class ThreadState(
        var title: String,
        val messages: MutableList<CapturedMessage> = mutableListOf(),
    )

    private val threads = LinkedHashMap<Long, ThreadState>()
    private val messageIds = mutableSetOf<Long>()

    @Synchronized
    fun add(message: CapturedMessage): Boolean {
        if (!messageIds.add(message.id)) return false
        val state = threads.getOrPut(message.threadId) { ThreadState(message.conversationTitle) }
        if (message.conversationTitle.isNotBlank()) state.title = message.conversationTitle
        state.messages.add(message)
        state.messages.sortWith(compareBy(CapturedMessage::postedAtMillis, CapturedMessage::id))
        while (state.messages.size > maxMessagesPerThread) {
            messageIds.remove(state.messages.removeAt(0).id)
        }
        evictOldestThreads()
        return true
    }

    @Synchronized
    override fun conversations(): List<LiveConversation> = threads
        .mapNotNull { (threadId, state) ->
            val latest = state.messages.lastOrNull() ?: return@mapNotNull null
            LiveConversation(
                threadId = threadId,
                title = state.title,
                snippet = latest.body,
                updatedAtMillis = latest.postedAtMillis,
                // Notification presence is not authoritative provider read state.
                unreadCount = 0,
            )
        }
        .sortedWith(compareByDescending<LiveConversation> { it.updatedAtMillis }.thenByDescending { it.threadId })

    @Synchronized
    override fun messages(query: LiveMessageQuery): List<StoredMessage> {
        val limit = query.limit.coerceIn(1, LiveMessageQuery.MAX_LIMIT)
        val source = if (query.threadId != 0L) {
            threads[query.threadId]?.messages.orEmpty()
        } else {
            threads.values.flatMap(ThreadState::messages)
        }
        val filtered = source
            .asSequence()
            .filter { message -> query.beforeId <= 0 || message.id < query.beforeId }
            .filter { message -> query.afterId <= 0 || message.id > query.afterId }
            .sortedWith(compareBy(CapturedMessage::postedAtMillis, CapturedMessage::id))
            .toList()
        return filtered.takeLast(limit).map(CapturedMessage::toStoredMessage)
    }

    private fun evictOldestThreads() {
        while (threads.size > maxThreads) {
            val oldest = threads.minByOrNull { (_, state) -> state.messages.lastOrNull()?.postedAtMillis ?: 0L }
                ?: return
            threads.remove(oldest.key)?.messages?.forEach { message -> messageIds.remove(message.id) }
        }
    }
}
