package com.galaxytty.helper.notification

object ObservationRepository {
    private const val MAX_OBSERVATIONS = 50
    private val observations = ArrayDeque<NotificationObservation>()

    @Synchronized
    fun add(observation: NotificationObservation) {
        // The activity only needs redacted diagnostic shape. Message content is
        // retained exclusively by the bounded in-memory messaging store.
        observations.addLast(observation.copy(message = null))
        while (observations.size > MAX_OBSERVATIONS) {
            observations.removeFirst()
        }
    }

    @Synchronized
    fun snapshot(): List<NotificationObservation> = observations.toList()

    @Synchronized
    fun clear() = observations.clear()
}
