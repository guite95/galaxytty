package com.galaxytty.helper.samsung

import com.galaxytty.helper.message.ConversationAddressResolver
import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class ReplyFallbackTest {
    @Test
    fun dispatcherUsesFallbackOnlyWhenActiveActionIsUnavailable() {
        var fallbackCalls = 0
        var actionCalls = 0
        val actions = ReplyActionRegistry(policy = ReplyExecutionPolicy { true })
        val dispatcher = ReplyDispatcher(
            actions = actions,
            fallback = ReplyFallback { _, _ ->
                fallbackCalls++
                ReplyDispatchResult(ReplyDispatchStatus.USER_ACTION_REQUIRED)
            },
        )

        assertEquals(ReplyDispatchStatus.USER_ACTION_REQUIRED, dispatcher.dispatch(7, "first").status)
        actions.register("opaque-notification", 7) { actionCalls++ }
        assertEquals(ReplyDispatchStatus.ACCEPTED_UNVERIFIED, dispatcher.dispatch(7, "second").status)
        assertEquals(1, fallbackCalls)
        assertEquals(1, actionCalls)
    }

    @Test
    fun disabledExecutionPolicyCannotBeBypassedByFallback() {
        var fallbackCalls = 0
        val dispatcher = ReplyDispatcher(
            actions = ReplyActionRegistry(),
            fallback = ReplyFallback { _, _ ->
                fallbackCalls++
                ReplyDispatchResult(ReplyDispatchStatus.USER_ACTION_REQUIRED)
            },
        )

        assertEquals(ReplyDispatchStatus.DISABLED, dispatcher.dispatch(7, "synthetic").status)
        assertEquals(0, fallbackCalls)
    }

    @Test
    fun rejectedRetainedActionFallsBackToComposeHandoff() {
        var fallbackCalls = 0
        val actions = ReplyActionRegistry(policy = ReplyExecutionPolicy { true })
        actions.register("opaque-notification", 7) {
            throw IllegalStateException("canceled token")
        }
        actions.markInactive("opaque-notification")
        val dispatcher = ReplyDispatcher(
            actions = actions,
            fallback = ReplyFallback { _, _ ->
                fallbackCalls++
                ReplyDispatchResult(ReplyDispatchStatus.USER_ACTION_REQUIRED)
            },
        )

        assertEquals(ReplyDispatchStatus.USER_ACTION_REQUIRED, dispatcher.dispatch(7, "synthetic").status)
        assertEquals(1, fallbackCalls)
        assertFalse(actions.cachedAvailable(7))
    }

    @Test
    fun rejectedActionKeepsOriginalFailureWhenComposeTargetIsUnavailable() {
        val actions = ReplyActionRegistry(policy = ReplyExecutionPolicy { true })
        actions.register("opaque-notification", 7) {
            throw IllegalStateException("private detail")
        }
        val dispatcher = ReplyDispatcher(actions = actions)

        val result = dispatcher.dispatch(7, "synthetic")

        assertEquals(ReplyDispatchStatus.FAILED, result.status)
        assertTrue(result.error?.contains("IllegalStateException") == true)
        assertFalse(result.error?.contains("private detail") == true)
    }

    @Test
    fun preparesComposeHandoffWithoutClaimingSendSuccess() {
        var publishedAddress = ""
        var publishedText = ""
        val fallback = AddressedComposeReplyFallback(
            addresses = ConversationAddressResolver { threadId ->
                if (threadId == 7L) "01000000000" else null
            },
            publisher = ComposeRequestPublisher { address, text ->
                publishedAddress = address
                publishedText = text
                true
            },
        )

        val result = fallback.prepare(7, "synthetic reply")

        assertEquals(ReplyDispatchStatus.USER_ACTION_REQUIRED, result.status)
        assertEquals("samsung_compose_notification_posted", result.evidence)
        assertEquals("01000000000", publishedAddress)
        assertEquals("synthetic reply", publishedText)
    }

    @Test
    fun unavailableAddressDoesNotPublish() {
        var published = false
        val fallback = AddressedComposeReplyFallback(
            addresses = ConversationAddressResolver { null },
            publisher = ComposeRequestPublisher { _, _ -> published = true; true },
        )

        val result = fallback.prepare(7, "synthetic reply")

        assertEquals(ReplyDispatchStatus.ACTION_UNAVAILABLE, result.status)
        assertFalse(published)
    }

    @Test
    fun publisherFailureDoesNotExposeMessageOrAddress() {
        val fallback = AddressedComposeReplyFallback(
            addresses = ConversationAddressResolver { "01000000000" },
            publisher = ComposeRequestPublisher { _, _ -> throw IllegalStateException("private detail") },
        )

        val result = fallback.prepare(7, "private synthetic body")

        assertEquals(ReplyDispatchStatus.FAILED, result.status)
        assertTrue(result.error?.contains("IllegalStateException") == true)
        assertFalse(result.error?.contains("private detail") == true)
        assertFalse(result.error?.contains("private synthetic body") == true)
        assertFalse(result.error?.contains("01000000000") == true)
    }

    @Test
    fun addressLookupFailureDoesNotCloseTheProtocolSessionOrLeakDetails() {
        val fallback = AddressedComposeReplyFallback(
            addresses = ConversationAddressResolver { throw IllegalArgumentException("private target") },
            publisher = ComposeRequestPublisher { _, _ -> true },
        )

        val result = fallback.prepare(7, "private synthetic body")

        assertEquals(ReplyDispatchStatus.FAILED, result.status)
        assertTrue(result.error?.contains("IllegalArgumentException") == true)
        assertFalse(result.error?.contains("private target") == true)
        assertFalse(result.error?.contains("private synthetic body") == true)
    }
}
