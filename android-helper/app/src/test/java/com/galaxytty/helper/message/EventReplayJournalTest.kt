package com.galaxytty.helper.message

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class EventReplayJournalTest {
    @Test
    fun recoversCompleteRangeIncludingContentlessEvents() {
        val journal = EventReplayJournal(maxEvents = 4)
        journal.append(10, null)
        journal.append(11, message(11))
        journal.append(12, null)

        val result = journal.recover(10, 12)

        assertTrue(result.complete)
        assertEquals(listOf(11L), result.messages.map(StoredMessage::id))
    }

    @Test
    fun marksRangeIncompleteAfterBoundedEviction() {
        val journal = EventReplayJournal(maxEvents = 2)
        journal.append(10, message(10))
        journal.append(11, message(11))
        journal.append(12, message(12))

        val result = journal.recover(10, 12)

        assertFalse(result.complete)
        assertEquals(listOf(11L, 12L), result.messages.map(StoredMessage::id))
    }

    @Test
    fun resetsJournalWhenSequenceEpochMovesBackward() {
        val journal = EventReplayJournal(maxEvents = 4)
        journal.append(20, message(20))
        journal.append(1, message(1))

        assertFalse(journal.recover(20, 20).complete)
        assertTrue(journal.recover(1, 1).complete)
    }

    private fun message(id: Long) = StoredMessage(
        id = id,
        threadId = 1,
        address = "",
        body = "synthetic-$id",
        postedAtMillis = id,
        direction = "incoming",
        read = false,
        messageType = "unknown",
    )
}
