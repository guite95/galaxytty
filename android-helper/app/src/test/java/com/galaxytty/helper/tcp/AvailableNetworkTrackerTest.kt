package com.galaxytty.helper.tcp

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class AvailableNetworkTrackerTest {
    @Test
    fun selectsNewlyAvailableNetworkAndIgnoresDuplicateCallback() {
        val tracker = AvailableNetworkTracker<String>()

        assertEquals(NetworkTransition(null, "wifi-a"), tracker.available("wifi-a"))
        assertNull(tracker.available("wifi-a"))
        assertEquals(NetworkTransition("wifi-a", "wifi-b"), tracker.available("wifi-b"))
    }

    @Test
    fun losingActiveNetworkFallsBackToAnotherAvailableNetwork() {
        val tracker = AvailableNetworkTracker<String>()
        tracker.available("wifi-a")
        tracker.available("wifi-b")

        assertNull(tracker.lost("wifi-a"))
        assertEquals(NetworkTransition("wifi-b", null), tracker.lost("wifi-b"))
    }

    @Test
    fun fallsBackWhenOlderNetworkIsStillAvailable() {
        val tracker = AvailableNetworkTracker<String>()
        tracker.available("wifi-a")
        tracker.available("wifi-b")

        assertEquals(NetworkTransition("wifi-b", "wifi-a"), tracker.lost("wifi-b"))
    }

    @Test
    fun clearDropsCurrentNetworkOnlyOnce() {
        val tracker = AvailableNetworkTracker<String>()
        tracker.available("wifi-a")

        assertEquals(NetworkTransition("wifi-a", null), tracker.clear())
        assertNull(tracker.clear())
    }
}
