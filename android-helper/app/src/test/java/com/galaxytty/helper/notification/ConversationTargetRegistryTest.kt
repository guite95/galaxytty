package com.galaxytty.helper.notification

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class ConversationTargetRegistryTest {
    @Test
    fun keepsOnlyBoundedNonSensitiveTargets() {
        val registry = ConversationTargetRegistry(maxTargets = 2)

        registry.register(1, " 01000000001 ")
        registry.register(2, "01000000002")
        registry.register(3, "01000000003")

        assertNull(registry.oneToOneAddress(1))
        assertEquals("01000000002", registry.oneToOneAddress(2))
        assertEquals("01000000003", registry.oneToOneAddress(3))
    }

    @Test
    fun ignoresInvalidTargets() {
        val registry = ConversationTargetRegistry()

        registry.register(0, "01000000000")
        registry.register(1, " ")

        assertNull(registry.oneToOneAddress(0))
        assertNull(registry.oneToOneAddress(1))
    }
}
