package com.galaxytty.helper.samsung

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ReplyActionRegistryTest {
    @Test
    fun disabledPolicyNeverInvokesRegisteredAction() {
        var invoked = false
        val registry = ReplyActionRegistry()
        registry.register("opaque-notification", 7) { invoked = true }

        val result = registry.dispatch(7, "must not send")

        assertEquals(ReplyDispatchStatus.DISABLED, result.status)
        assertTrue(result.error?.contains("enable replies locally") == true)
        assertFalse(invoked)
    }

    @Test
    fun enabledFakeActionIsOnlyAcceptedUnverified() {
        var deliveredText = ""
        val registry = ReplyActionRegistry(policy = ReplyExecutionPolicy { true })
        registry.register("opaque-notification", 7) { text -> deliveredText = text }

        val result = registry.dispatch(7, "synthetic reply")

        assertEquals(ReplyDispatchStatus.ACCEPTED_UNVERIFIED, result.status)
        assertEquals("remote_input_pending_intent_accepted", result.evidence)
        assertEquals("synthetic reply", deliveredText)
    }

    @Test
    fun validatesRequestsAndRemovesExpiredNotificationActions() {
        val registry = ReplyActionRegistry(policy = ReplyExecutionPolicy { true })
        registry.register("opaque-notification", 7) {}

        assertTrue(registry.available(7))
        assertEquals(ReplyDispatchStatus.INVALID_REQUEST, registry.dispatch(7, " ").status)
        registry.unregister("opaque-notification")

        assertFalse(registry.available(7))
        assertEquals(ReplyDispatchStatus.ACTION_UNAVAILABLE, registry.dispatch(7, "reply").status)
    }

    @Test
    fun boundsRetainedActions() {
        val invoked = mutableListOf<Long>()
        val registry = ReplyActionRegistry(policy = ReplyExecutionPolicy { true }, maxActions = 2)
        for (threadId in 1L..3L) {
            registry.register("notification-$threadId", threadId) { invoked += threadId }
        }

        assertEquals(ReplyDispatchStatus.ACTION_UNAVAILABLE, registry.dispatch(1, "reply").status)
        assertEquals(ReplyDispatchStatus.ACCEPTED_UNVERIFIED, registry.dispatch(3, "reply").status)
        assertTrue(invoked == listOf(3L))
    }

    @Test
    fun failureReportsOnlyExceptionClass() {
        val registry = ReplyActionRegistry(policy = ReplyExecutionPolicy { true })
        registry.register("opaque-notification", 7) {
            throw IllegalStateException("private implementation detail")
        }

        val result = registry.dispatch(7, "reply")

        assertEquals(ReplyDispatchStatus.FAILED, result.status)
        assertTrue(result.error?.contains("IllegalStateException") == true)
        assertFalse(result.error?.contains("private implementation detail") == true)
    }
}
