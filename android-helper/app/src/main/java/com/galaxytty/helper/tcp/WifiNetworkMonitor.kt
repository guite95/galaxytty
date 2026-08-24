package com.galaxytty.helper.tcp

import android.content.Context
import android.net.ConnectivityManager
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.util.Log
import java.io.Closeable

internal class WifiNetworkMonitor(
    context: Context,
    private val onNetworkChanged: (Network?) -> Unit,
) : Closeable {
    private val connectivityManager = context.getSystemService(ConnectivityManager::class.java)
    private val tracker = AvailableNetworkTracker<Network>()
    private var registered = false
    private val callback = object : ConnectivityManager.NetworkCallback() {
        override fun onAvailable(network: Network) {
            val transition = tracker.available(network) ?: return
            Log.i(TAG, "local Wi-Fi available; refreshing NSD")
            onNetworkChanged(transition.current)
        }

        override fun onLost(network: Network) {
            val transition = tracker.lost(network) ?: return
            Log.i(TAG, "local Wi-Fi changed; refreshing NSD")
            onNetworkChanged(transition.current)
        }
    }

    @Synchronized
    fun start() {
        if (registered) return
        val request = NetworkRequest.Builder()
            .addTransportType(NetworkCapabilities.TRANSPORT_WIFI)
            .build()
        connectivityManager.registerNetworkCallback(request, callback)
        registered = true
    }

    @Synchronized
    override fun close() {
        if (registered) {
            registered = false
            try {
                connectivityManager.unregisterNetworkCallback(callback)
            } catch (_: IllegalArgumentException) {
                // The process or framework already released this callback.
            }
        }
        if (tracker.clear() != null) onNetworkChanged(null)
    }

    companion object {
        private const val TAG = "GalaxyTTY-TCP"
    }
}
