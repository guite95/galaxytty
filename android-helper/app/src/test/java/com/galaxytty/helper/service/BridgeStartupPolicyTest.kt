package com.galaxytty.helper.service

import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class BridgeStartupPolicyTest {
    @Test
    fun restartsOnlyWhenUserEnabledBridge() {
        assertFalse(BridgeStartupPolicy.shouldRestart(false, "android.intent.action.BOOT_COMPLETED"))
        assertTrue(BridgeStartupPolicy.shouldRestart(true, "android.intent.action.BOOT_COMPLETED"))
        assertTrue(BridgeStartupPolicy.shouldRestart(true, "android.intent.action.MY_PACKAGE_REPLACED"))
    }

    @Test
    fun ignoresUnrelatedAndMissingActions() {
        assertFalse(BridgeStartupPolicy.shouldRestart(true, "android.net.conn.CONNECTIVITY_CHANGE"))
        assertFalse(BridgeStartupPolicy.shouldRestart(true, null))
    }
}
