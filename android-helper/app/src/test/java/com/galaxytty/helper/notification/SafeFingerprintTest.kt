package com.galaxytty.helper.notification

import org.junit.Assert.assertEquals
import org.junit.Assert.assertFalse
import org.junit.Test

class SafeFingerprintTest {
    @Test
    fun fingerprintIsStableAndDoesNotRetainInput() {
        val sensitive = "synthetic-notification-key"
        val first = SafeFingerprint.of(sensitive)
        assertEquals(first, SafeFingerprint.of(sensitive))
        assertEquals(12, first.length)
        assertFalse(first.contains(sensitive))
    }
}
