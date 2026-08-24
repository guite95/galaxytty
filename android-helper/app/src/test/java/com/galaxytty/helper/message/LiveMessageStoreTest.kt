package com.galaxytty.helper.message

import com.galaxytty.helper.notification.CapturedMessage
import com.galaxytty.helper.notification.NotificationContentSource
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class LiveMessageStoreTest {
    @Test
    fun deduplicatesAndBuildsLatestFirstConversations() {
        val store = LiveMessageStore()
        assertTrue(store.add(message(id = 10, threadId = 1, body = "older", postedAt = 100)))
        assertFalse(store.add(message(id = 10, threadId = 1, body = "older", postedAt = 100)))
        assertTrue(store.add(message(id = 20, threadId = 2, body = "newer", postedAt = 200)))

        val conversations = store.conversations()
        assertEquals(listOf(2L, 1L), conversations.map(LiveConversation::threadId))
        assertEquals("newer", conversations.first().snippet)
        assertEquals(0, conversations.last().unreadCount)
    }

    @Test
    fun pagesMessagesAndBoundsPerThreadMemory() {
        val store = LiveMessageStore(maxThreads = 2, maxMessagesPerThread = 2)
        store.add(message(id = 10, threadId = 1, body = "one", postedAt = 10))
        store.add(message(id = 20, threadId = 1, body = "two", postedAt = 20))
        store.add(message(id = 30, threadId = 1, body = "three", postedAt = 30))

        assertEquals(
            listOf(20L, 30L),
            store.messages(LiveMessageQuery(threadId = 1)).map(StoredMessage::id),
        )
        assertEquals(
            listOf(20L),
            store.messages(LiveMessageQuery(threadId = 1, beforeId = 30)).map(StoredMessage::id),
        )
        assertEquals(
            listOf(30L),
            store.messages(LiveMessageQuery(threadId = 1, afterId = 20)).map(StoredMessage::id),
        )
    }

    @Test
    fun evictsOldestConversationAtThreadBound() {
        val store = LiveMessageStore(maxThreads = 2)
        store.add(message(id = 10, threadId = 1, body = "oldest", postedAt = 10))
        store.add(message(id = 20, threadId = 2, body = "middle", postedAt = 20))
        store.add(message(id = 30, threadId = 3, body = "latest", postedAt = 30))

        assertEquals(listOf(3L, 2L), store.conversations().map(LiveConversation::threadId))
        assertTrue(store.messages(LiveMessageQuery(threadId = 1)).isEmpty())
    }

    private fun message(id: Long, threadId: Long, body: String, postedAt: Long) = CapturedMessage(
        id = id,
        threadId = threadId,
        conversationTitle = "thread-$threadId",
        senderLabel = "sender-$threadId",
        body = body,
        postedAtMillis = postedAt,
        source = NotificationContentSource.MESSAGING_STYLE,
    )
}
