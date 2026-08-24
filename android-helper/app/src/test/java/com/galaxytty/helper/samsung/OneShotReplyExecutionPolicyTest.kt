package com.galaxytty.helper.samsung

import java.io.File
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Rule
import org.junit.Test
import org.junit.rules.TemporaryFolder

class OneShotReplyExecutionPolicyTest {
    @get:Rule
    val temporaryFolder = TemporaryFolder()

    @Test
    fun freshMarkerAllowsExactlyOneExecution() {
        val now = 10_000L
        val marker = armedMarker(now - 1_000)
        val policy = OneShotReplyExecutionPolicy(marker, enabled = true, nowMillis = { now })

        assertTrue(policy.isArmed())
        assertTrue(policy.allowsExecution())
        assertFalse(policy.allowsExecution())
        assertFalse(marker.exists())
    }

    @Test
    fun expiredMarkerIsRejectedAndRemoved() {
        val now = 100_000L
        val marker = armedMarker(now - OneShotReplyExecutionPolicy.DEFAULT_VALIDITY_MILLIS - 1)
        val policy = OneShotReplyExecutionPolicy(marker, enabled = true, nowMillis = { now })

        assertFalse(policy.isArmed())
        assertFalse(marker.exists())
        assertFalse(policy.allowsExecution())
    }

    @Test
    fun disabledBuildConsumesMarkerWithoutExecuting() {
        val marker = armedMarker(1_000L)
        val policy = OneShotReplyExecutionPolicy(marker, enabled = false, nowMillis = { 1_000L })

        assertFalse(policy.allowsExecution())
        assertFalse(marker.exists())
    }

    private fun armedMarker(lastModified: Long): File =
        temporaryFolder.newFile(OneShotReplyExecutionPolicy.MARKER_FILE_NAME).also { marker ->
            check(marker.setLastModified(lastModified))
        }
}
