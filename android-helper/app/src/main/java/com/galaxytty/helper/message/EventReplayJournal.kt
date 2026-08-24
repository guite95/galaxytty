package com.galaxytty.helper.message

data class EventReplayResult(
    val complete: Boolean,
    val messages: List<StoredMessage>,
)

class EventReplayJournal(
    private val maxEvents: Int = DEFAULT_MAX_EVENTS,
) {
    private data class Record(
        val sequence: Long,
        val message: StoredMessage?,
    )

    private val records = ArrayDeque<Record>()

    init {
        require(maxEvents > 0) { "event replay bound must be positive" }
    }

    @Synchronized
    fun append(sequence: Long, message: StoredMessage?) {
        require(sequence > 0) { "event sequence must be positive" }
        if (records.lastOrNull()?.sequence?.let { previous -> sequence <= previous } == true) {
            records.clear()
        }
        records.addLast(Record(sequence, message))
        while (records.size > maxEvents) records.removeFirst()
    }

    @Synchronized
    fun recover(fromSequence: Long, throughSequence: Long): EventReplayResult {
        require(fromSequence > 0) { "recovery start sequence must be positive" }
        require(throughSequence >= fromSequence) { "recovery range is invalid" }
        val selected = records.filter { record -> record.sequence in fromSequence..throughSequence }
        val expectedCount = throughSequence - fromSequence + 1
        val complete = selected.size.toLong() == expectedCount &&
            selected.firstOrNull()?.sequence == fromSequence &&
            selected.lastOrNull()?.sequence == throughSequence
        return EventReplayResult(
            complete = complete,
            messages = selected.mapNotNull(Record::message),
        )
    }

    companion object {
        const val DEFAULT_MAX_EVENTS = 512
    }
}
