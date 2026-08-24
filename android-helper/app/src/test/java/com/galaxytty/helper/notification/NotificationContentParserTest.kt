package com.galaxytty.helper.notification

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNotEquals
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class NotificationContentParserTest {
    private val parser = NotificationContentParser { ByteArray(32) { index -> index.toByte() } }

    @Test
    fun selectsLatestIncomingMessagingStyleCandidate() {
        val result = requireNotNull(
            parser.capture(
                threadKey = "opaque-notification-key",
                conversationTitle = "대화방",
                notificationTitle = "보낸 사람",
                postedAtMillis = 300,
                candidates = listOf(
                    NotificationTextCandidate("내가 보낸 내용", null, 200, fromCurrentUser = true),
                    NotificationTextCandidate("이전 수신", "보낸 사람", 100, fromCurrentUser = false),
                    NotificationTextCandidate(
                        "최신 수신 😀",
                        "보낸 사람",
                        300,
                        fromCurrentUser = false,
                        replyAddress = "+821012345678",
                    ),
                ),
                fallbackText = "fallback",
            ),
        )

        assertEquals("최신 수신 😀", result.body)
        assertEquals("보낸 사람", result.senderLabel)
        assertEquals("대화방", result.conversationTitle)
        assertEquals(300, result.postedAtMillis)
        assertEquals(NotificationContentSource.MESSAGING_STYLE, result.source)
        assertEquals("+821012345678", result.replyAddress)
    }

    @Test
    fun fallsBackToNotificationTextWithoutGuessingTransport() {
        val result = requireNotNull(
            parser.capture(
                threadKey = "opaque-notification-key",
                conversationTitle = null,
                notificationTitle = "표시 이름",
                postedAtMillis = 1_777_000_000_000,
                candidates = emptyList(),
                fallbackText = "본문",
            ),
        )

        assertEquals("본문", result.body)
        assertEquals("표시 이름", result.conversationTitle)
        assertEquals(NotificationContentSource.TEXT_FALLBACK, result.source)
        assertTrue(result.id > 0)
        assertTrue(result.threadId > 0)
    }

    @Test
    fun idsAreStableAndMessageIdsPreserveTimestampOrder() {
        val secret = ByteArray(32) { index -> index.toByte() }
        val thread = StableContentId.thread(secret, "opaque")
        assertEquals(thread, StableContentId.thread(secret, "opaque"))
        assertNotEquals(thread, StableContentId.thread(secret, "another"))

        val earlier = StableContentId.message(secret, "opaque", 1_000, "sender", "one")
        val later = StableContentId.message(secret, "opaque", 1_001, "sender", "two")
        assertTrue(earlier < later)
    }

    @Test
    fun ignoresEmptyAndCurrentUserOnlyContent() {
        assertNull(
            parser.capture(
                threadKey = "opaque",
                conversationTitle = "title",
                notificationTitle = "sender",
                postedAtMillis = 100,
                candidates = listOf(NotificationTextCandidate("outgoing", null, 100, true)),
                fallbackText = null,
            ),
        )
    }
}
