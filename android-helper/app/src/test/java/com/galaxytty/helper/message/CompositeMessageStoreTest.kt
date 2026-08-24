package com.galaxytty.helper.message

import org.junit.Assert.assertEquals
import org.junit.Assert.assertTrue
import org.junit.Test

class CompositeMessageStoreTest {
    @Test
    fun combinesHistoryAndNotificationMemoryLatestFirst() {
        val history = FakeHistory(
            available = true,
            conversations = listOf(conversation(2, 200)),
            messages = listOf(message(20, 2, 200, "sms")),
        )
        val live = FakeRepository(
            conversations = listOf(conversation(3, 300)),
            messages = listOf(message(30, 3, 300, "unknown")),
        )
        val store = CompositeMessageStore(live, history)

        assertEquals(listOf(3L, 2L), store.conversations().map(LiveConversation::threadId))
        assertEquals(
            listOf(20L, 30L),
            store.messages(LiveMessageQuery()).map(StoredMessage::id),
        )
    }

    @Test
    fun keepsLiveCacheAvailableWhenHistoryFails() {
        var failures = 0
        val history = object : HistoryMessageRepository {
            override fun available() = true
            override fun conversations(): List<LiveConversation> = error("provider unavailable")
            override fun messages(query: LiveMessageQuery): List<StoredMessage> = error("provider unavailable")
        }
        val live = FakeRepository(
            conversations = listOf(conversation(3, 300)),
            messages = listOf(message(30, 3, 300, "unknown")),
        )
        val store = CompositeMessageStore(live, history) { failures++ }

        assertEquals(listOf(3L), store.conversations().map(LiveConversation::threadId))
        assertEquals(listOf(30L), store.messages(LiveMessageQuery()).map(StoredMessage::id))
        assertEquals(2, failures)
    }

    @Test
    fun skipsHistoryCompletelyWithoutPermission() {
        val history = FakeHistory(false, emptyList(), emptyList())
        val store = CompositeMessageStore(FakeRepository(emptyList(), emptyList()), history)

        assertTrue(store.conversations().isEmpty())
        assertTrue(store.messages(LiveMessageQuery()).isEmpty())
        assertEquals(0, history.calls)
    }

    private open class FakeRepository(
        private val conversations: List<LiveConversation>,
        private val messages: List<StoredMessage>,
    ) : MessageRepository {
        override fun conversations() = conversations
        override fun messages(query: LiveMessageQuery) = messages
    }

    private class FakeHistory(
        private val available: Boolean,
        conversations: List<LiveConversation>,
        messages: List<StoredMessage>,
    ) : FakeRepository(conversations, messages), HistoryMessageRepository {
        var calls = 0

        override fun available() = available
        override fun conversations(): List<LiveConversation> {
            calls++
            return super.conversations()
        }

        override fun messages(query: LiveMessageQuery): List<StoredMessage> {
            calls++
            return super.messages(query)
        }
    }

    private fun conversation(id: Long, updatedAt: Long) = LiveConversation(
        threadId = id,
        title = "thread-$id",
        snippet = "snippet-$id",
        updatedAtMillis = updatedAt,
        unreadCount = 0,
    )

    private fun message(id: Long, threadId: Long, postedAt: Long, type: String) = StoredMessage(
        id = id,
        threadId = threadId,
        address = "",
        body = "body-$id",
        postedAtMillis = postedAt,
        direction = "incoming",
        read = false,
        messageType = type,
    )
}
