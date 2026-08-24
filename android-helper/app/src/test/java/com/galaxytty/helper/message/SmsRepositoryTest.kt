package com.galaxytty.helper.message

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SmsRepositoryTest {
    @Test
    fun mapsConversationsWithoutRequestingContacts() {
        val source = FakeSmsDataSource(
            mapOf(
                "content://mms-sms/canonical-addresses" to listOf(
                    row("_id" to "12", "address" to "010-1234-5678"),
                    row("_id" to "34", "address" to null),
                ),
                "content://mms-sms/conversations?simple=true" to listOf(
                    row(
                        "_id" to "5",
                        "recipient_ids" to "12 34",
                        "unread_count" to "2",
                        "date" to "1700000000000",
                        "snippet" to "synthetic snippet",
                    ),
                ),
            ),
        )

        val conversations = SmsRepository(source) { true }.conversations()

        assertEquals(1, conversations.size)
        assertEquals(5L, conversations.single().threadId)
        assertEquals("010-1234-5678, Unknown participant", conversations.single().title)
        assertEquals(2, conversations.single().unreadCount)
        assertEquals("010-1234-5678", conversations.single().participants.first().phone)
        assertEquals(
            listOf(
                "content://mms-sms/conversations?simple=true",
                "content://mms-sms/canonical-addresses",
            ),
            source.queries.map(SmsQuery::uri),
        )
        assertEquals("date DESC LIMIT 500", source.queries.first().sortOrder)
    }

    @Test
    fun buildsBoundedSmsHistoryQueryAndMapsDirection() {
        val source = FakeSmsDataSource(
            mapOf(
                "content://sms" to listOf(
                    smsRow(id = "8", type = "2", read = "1", date = "1700000000000"),
                    smsRow(id = "7", type = "1", read = "0", date = "1699999999000"),
                ),
            ),
        )
        val repository = SmsRepository(source) { true }

        val messages = repository.messages(
            LiveMessageQuery(threadId = 2, limit = 5000, beforeId = 9, afterId = 6),
        )

        assertEquals(listOf(7L, 8L), messages.map(StoredMessage::id))
        assertEquals(listOf("incoming", "outgoing"), messages.map(StoredMessage::direction))
        assertFalse(messages.first().read)
        assertTrue(messages.last().read)
        assertEquals(listOf("sms", "sms"), messages.map(StoredMessage::messageType))
        val query = source.queries.single()
        assertEquals("type IN (?, ?) AND thread_id = ? AND _id < ? AND _id > ?", query.selection)
        assertEquals(listOf("1", "2", "2", "9", "6"), query.selectionArgs)
        assertEquals("_id DESC LIMIT 500", query.sortOrder)
    }

    @Test
    fun permissionGatePreventsProviderAccess() {
        val source = FakeSmsDataSource(emptyMap())
        val repository = SmsRepository(source) { false }

        assertFalse(repository.available())
        assertTrue(repository.conversations().isEmpty())
        assertTrue(repository.messages(LiveMessageQuery(threadId = 1)).isEmpty())
        assertEquals(null, repository.oneToOneAddress(1))
        assertTrue(source.queries.isEmpty())
    }

    @Test
    fun resolvesOnlyOneToOneConversationAddress() {
        val source = FakeSmsDataSource(
            mapOf(
                "content://mms-sms/canonical-addresses" to listOf(
                    row("_id" to "12", "address" to "010-1234-5678"),
                    row("_id" to "34", "address" to "010-9999-9999"),
                ),
                "content://mms-sms/conversations?simple=true" to listOf(
                    row(
                        "_id" to "5",
                        "recipient_ids" to "12",
                        "unread_count" to "0",
                        "date" to "1700000000000",
                        "snippet" to "synthetic snippet",
                    ),
                    row(
                        "_id" to "6",
                        "recipient_ids" to "12 34",
                        "unread_count" to "0",
                        "date" to "1699999999000",
                        "snippet" to "synthetic group snippet",
                    ),
                ),
            ),
        )
        val repository = SmsRepository(source) { true }

        assertEquals("010-1234-5678", repository.oneToOneAddress(5))
        assertEquals(null, repository.oneToOneAddress(6))
        assertEquals(null, repository.oneToOneAddress(404))
    }

    private class FakeSmsDataSource(
        private val responses: Map<String, List<Map<String, String?>>>,
    ) : SmsDataSource {
        val queries = mutableListOf<SmsQuery>()

        override fun query(query: SmsQuery): List<Map<String, String?>> {
            queries += query
            return responses[query.uri].orEmpty()
        }
    }

    private fun row(vararg values: Pair<String, String?>): Map<String, String?> = mapOf(*values)

    private fun smsRow(id: String, type: String, read: String, date: String) = row(
        "_id" to id,
        "thread_id" to "2",
        "address" to "01000000000",
        "date" to date,
        "type" to type,
        "read" to read,
        "body" to "synthetic body",
    )
}
