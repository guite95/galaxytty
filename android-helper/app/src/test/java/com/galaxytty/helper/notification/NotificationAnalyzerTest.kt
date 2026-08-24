package com.galaxytty.helper.notification

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertNull
import org.junit.Assert.assertTrue
import org.junit.Test

class NotificationAnalyzerTest {
    private val analyzer = NotificationAnalyzer()

    @Test
    fun ignoresNonSamsungMessagesNotifications() {
        assertNull(analyzer.analyze(snapshot(packageName = "com.example.other")))
    }

    @Test
    fun reportsRemoteInputShapeWithoutContent() {
        val observation = requireNotNull(analyzer.analyze(snapshot()))

        assertEquals(2, observation.actionCount)
        assertEquals(1, observation.replyCandidates.size)
        assertEquals(listOf("android.messages", "android.title"), observation.extrasKeys)
        assertEquals("input-key-fingerprint", observation.replyCandidates.single().remoteInputs.single().resultKeyFingerprint)
        assertFalse(observation.safeSummary().contains("private message body"))
        assertTrue(observation.safeSummary().contains("replyCandidates=1"))
    }

    @Test
    fun carriesCapturedContentButNeverIncludesItInSafeSummary() {
        val privateBody = "private message body"
        val observation = requireNotNull(
            analyzer.analyze(
                snapshot(
                    message = CapturedMessage(
                        id = 7,
                        threadId = 3,
                        conversationTitle = "private conversation",
                        senderLabel = "private sender",
                        body = privateBody,
                        postedAtMillis = 1_777_000_000_000,
                        source = NotificationContentSource.MESSAGING_STYLE,
                    ),
                ),
            ),
        )

        assertEquals(privateBody, observation.message?.body)
        assertTrue(observation.safeSummary().contains("content=true"))
        assertTrue(observation.safeSummary().contains("contentSource=messaging_style"))
        assertFalse(observation.safeSummary().contains(privateBody))
        assertFalse(observation.safeSummary().contains("private sender"))
        assertFalse(observation.safeSummary().contains("private conversation"))
    }

    private fun snapshot(
        packageName: String = NotificationAnalyzer.SAMSUNG_MESSAGES_PACKAGE,
        message: CapturedMessage? = null,
    ) =
        NotificationSnapshot(
            packageName = packageName,
            keyFingerprint = "notification-key",
            postedAtMillis = 1_777_000_000_000,
            category = "msg",
            extrasKeys = setOf("android.title", "android.messages"),
            actions = listOf(
                ActionSnapshot(
                    index = 0,
                    semanticAction = 0,
                    contextual = false,
                    authenticationRequired = false,
                    remoteInputs = emptyList(),
                ),
                ActionSnapshot(
                    index = 1,
                    semanticAction = 1,
                    contextual = true,
                    authenticationRequired = false,
                    remoteInputs = listOf(
                        RemoteInputSnapshot(
                            resultKeyFingerprint = "input-key-fingerprint",
                            allowsFreeFormText = true,
                            choiceCount = 0,
                            allowedDataTypeCount = 0,
                        ),
                    ),
                ),
            ),
            message = message,
        )
}
