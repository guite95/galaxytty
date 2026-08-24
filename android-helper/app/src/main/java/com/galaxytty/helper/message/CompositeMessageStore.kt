package com.galaxytty.helper.message

class CompositeMessageStore(
    private val live: MessageRepository,
    private val history: HistoryMessageRepository,
    private val onHistoryError: (RuntimeException) -> Unit = {},
) : MessageRepository {
    override fun conversations(): List<LiveConversation> {
        val combined = LinkedHashMap<Long, LiveConversation>()
        safeHistory { history.conversations() }
            .forEach { conversation -> combined[conversation.threadId] = conversation }
        live.conversations().forEach { conversation -> combined.putIfAbsent(conversation.threadId, conversation) }
        return combined.values.sortedWith(
            compareByDescending<LiveConversation> { conversation -> conversation.updatedAtMillis }
                .thenByDescending { conversation -> conversation.threadId },
        )
    }

    override fun messages(query: LiveMessageQuery): List<StoredMessage> {
        val combined = LinkedHashMap<Long, StoredMessage>()
        safeHistory { history.messages(query) }
            .forEach { message -> combined[message.id] = message }
        live.messages(query).forEach { message -> combined.putIfAbsent(message.id, message) }
        val limit = query.limit.coerceIn(1, LiveMessageQuery.MAX_LIMIT)
        return combined.values
            .sortedWith(compareBy(StoredMessage::postedAtMillis, StoredMessage::id))
            .takeLast(limit)
    }

    private fun <T> safeHistory(block: () -> List<T>): List<T> {
        if (!history.available()) return emptyList()
        return try {
            block()
        } catch (error: RuntimeException) {
            onHistoryError(error)
            emptyList()
        }
    }
}
