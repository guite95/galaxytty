package com.galaxytty.helper.tcp

internal data class NetworkTransition<T>(
    val previous: T?,
    val current: T?,
)

/**
 * Tracks every matching network so a lost active network can fall back to a
 * still-available one without waiting for Android to emit another callback.
 */
internal class AvailableNetworkTracker<T> {
    private val available = LinkedHashSet<T>()
    private var active: T? = null

    @Synchronized
    fun available(network: T): NetworkTransition<T>? {
        if (network == active) return null
        available.remove(network)
        available.add(network)
        val previous = active
        active = network
        return NetworkTransition(previous, network)
    }

    @Synchronized
    fun lost(network: T): NetworkTransition<T>? {
        available.remove(network)
        if (network != active) return null
        val previous = active
        active = available.lastOrNull()
        return NetworkTransition(previous, active)
    }

    @Synchronized
    fun clear(): NetworkTransition<T>? {
        available.clear()
        val previous = active ?: return null
        active = null
        return NetworkTransition(previous, null)
    }
}
