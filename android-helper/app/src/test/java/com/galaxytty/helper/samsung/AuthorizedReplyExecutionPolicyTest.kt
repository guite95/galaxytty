package com.galaxytty.helper.samsung

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class AuthorizedReplyExecutionPolicyTest {
    @Test
    fun rejectsWhenNeitherLocalPermissionNorOneShotAllowsExecution() {
        val policy = AuthorizedReplyExecutionPolicy(
            locallyAllowed = { false },
            oneShot = ReplyExecutionPolicy { false },
        )

        assertFalse(policy.allowsExecution())
    }

    @Test
    fun localPermissionAllowsRepeatedExecutionWithoutConsumingOneShot() {
        var oneShotCalls = 0
        val policy = AuthorizedReplyExecutionPolicy(
            locallyAllowed = { true },
            oneShot = ReplyExecutionPolicy {
                oneShotCalls++
                true
            },
        )

        assertTrue(policy.allowsExecution())
        assertTrue(policy.allowsExecution())
        assertTrue(policy.allowsExecution())
        assertEquals(0, oneShotCalls)
    }

    @Test
    fun localRevocationIsObservedOnTheNextExecution() {
        var locallyAllowed = true
        val policy = AuthorizedReplyExecutionPolicy(
            locallyAllowed = { locallyAllowed },
            oneShot = ReplyExecutionPolicy { false },
        )

        assertTrue(policy.allowsExecution())
        locallyAllowed = false
        assertFalse(policy.allowsExecution())
    }

    @Test
    fun oneShotCanAuthorizeOneExecutionWhenLocalPermissionIsOff() {
        var available = true
        val policy = AuthorizedReplyExecutionPolicy(
            locallyAllowed = { false },
            oneShot = ReplyExecutionPolicy {
                val allowed = available
                available = false
                allowed
            },
        )

        assertTrue(policy.allowsExecution())
        assertFalse(policy.allowsExecution())
    }
}
