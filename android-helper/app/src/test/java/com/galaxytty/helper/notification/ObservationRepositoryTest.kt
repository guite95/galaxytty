package com.galaxytty.helper.notification

import org.junit.Assert.assertNull
import org.junit.Test

class ObservationRepositoryTest {
    @Test
    fun retainsOnlyRedactedObservationShape() {
        ObservationRepository.clear()
        ObservationRepository.add(
            NotificationObservation(
                keyFingerprint = "safe-key",
                postedAtMillis = 100,
                category = "msg",
                extrasKeys = listOf("android.messages"),
                actionCount = 1,
                replyCandidates = emptyList(),
                message = CapturedMessage(
                    id = 1,
                    threadId = 2,
                    conversationTitle = "private title",
                    senderLabel = "private sender",
                    body = "private body",
                    postedAtMillis = 100,
                    source = NotificationContentSource.MESSAGING_STYLE,
                ),
            ),
        )

        assertNull(ObservationRepository.snapshot().single().message)
        ObservationRepository.clear()
    }
}
