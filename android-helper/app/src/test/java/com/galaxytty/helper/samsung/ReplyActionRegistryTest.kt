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
    fun validatesRequestsAndSupportsExplicitRemoval() {
        val registry = ReplyActionRegistry(policy = ReplyExecutionPolicy { true })
        registry.register("opaque-notification", 7) {}

        assertTrue(registry.available(7))
        assertEquals(ReplyDispatchStatus.INVALID_REQUEST, registry.dispatch(7, " ").status)
        registry.unregister("opaque-notification")

        assertFalse(registry.available(7))
        assertEquals(ReplyDispatchStatus.ACTION_UNAVAILABLE, registry.dispatch(7, "reply").status)
    }

    @Test
    fun removedNotificationActionRemainsAvailableAsRetainedCapability() {
        var now = 10_000L
        var deliveredText = ""
        val registry = ReplyActionRegistry(
            policy = ReplyExecutionPolicy { true },
            retainedActionTtlMillis = 1_000,
            nowMillis = { now },
        )
        registry.register("opaque-notification", 7) { text -> deliveredText = text }

        assertTrue(registry.markInactive("opaque-notification"))
        assertFalse(registry.available(7))
        assertTrue(registry.cachedAvailable(7))
        assertEquals(ReplyActionCounts(active = 0, cached = 1), registry.counts())

        now += 500
        val result = registry.dispatch(7, "synthetic retained reply")

        assertEquals(ReplyDispatchStatus.ACCEPTED_UNVERIFIED, result.status)
        assertEquals("retained_remote_input_pending_intent_accepted", result.evidence)
        assertEquals("synthetic retained reply", deliveredText)
        assertTrue(registry.cachedAvailable(7))
    }

    @Test
    fun retainedActionExpiresFromRemovalTimeButActiveActionDoesNot() {
        var now = 1_000L
        val registry = ReplyActionRegistry(
            policy = ReplyExecutionPolicy { true },
            retainedActionTtlMillis = 1_000,
            nowMillis = { now },
        )
        registry.register("retained-notification", 7) {}
        registry.register("active-notification", 8) {}
        registry.markInactive("retained-notification")

        now += 999
        assertTrue(registry.cachedAvailable(7))
        assertTrue(registry.available(8))

        now += 1
        assertFalse(registry.cachedAvailable(7))
        assertEquals(ReplyDispatchStatus.ACTION_UNAVAILABLE, registry.dispatch(7, "reply").status)
        assertTrue(registry.available(8))
    }

    @Test
    fun freshNotificationReplacesRetainedActionForTheSameThread() {
        val invoked = mutableListOf<String>()
        val registry = ReplyActionRegistry(policy = ReplyExecutionPolicy { true })
        registry.register("old-notification", 7) { invoked += "old" }
        registry.markInactive("old-notification")

        registry.register("new-notification", 7) { invoked += "new" }

        assertTrue(registry.available(7))
        assertFalse(registry.cachedAvailable(7))
        assertEquals(ReplyDispatchStatus.ACCEPTED_UNVERIFIED, registry.dispatch(7, "reply").status)
        assertEquals(listOf("new"), invoked)
    }

    @Test
    fun disabledPolicyDoesNotConsumeRetainedAction() {
        var allowed = false
        val registry = ReplyActionRegistry(policy = ReplyExecutionPolicy { allowed })
        registry.register("opaque-notification", 7) {}
        registry.markInactive("opaque-notification")

        assertEquals(ReplyDispatchStatus.DISABLED, registry.dispatch(7, "reply").status)
        assertTrue(registry.cachedAvailable(7))

        allowed = true
        assertEquals(ReplyDispatchStatus.ACCEPTED_UNVERIFIED, registry.dispatch(7, "reply").status)
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
        assertFalse(registry.available(7))
        assertFalse(registry.cachedAvailable(7))
    }
}
