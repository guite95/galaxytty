package com.galaxytty.helper.tcp

import android.content.Context
import android.net.Network
import android.net.nsd.NsdManager
import android.net.nsd.NsdServiceInfo
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.util.Log

class NsdAdvertiser(context: Context) {
    private val manager = context.getSystemService(NsdManager::class.java)
    private val retryHandler = Handler(Looper.getMainLooper())
    private var listener: NsdManager.RegistrationListener? = null
    private var advertisedNetwork: Network? = null
    private var advertisedPort: Int? = null
    private var desiredNetwork: Network? = null
    private var desiredPort: Int? = null
    private val retryRegistration = Runnable {
        synchronized(this) {
            val network = desiredNetwork
            val port = desiredPort
            if (listener == null && network != null && port != null) registerLocked(port, network)
        }
    }

    @Synchronized
    fun start(port: Int, network: Network) {
        if (listener != null && desiredPort == port && desiredNetwork == network) return
        retryHandler.removeCallbacks(retryRegistration)
        stopRegistrationLocked()
        desiredNetwork = network
        desiredPort = port
        registerLocked(port, network)
    }

    private fun registerLocked(port: Int, network: Network) {
        val registration = object : NsdManager.RegistrationListener {
            override fun onServiceRegistered(serviceInfo: NsdServiceInfo) {
                Log.i(TAG, "NSD registered name=${serviceInfo.serviceName} port=$port")
            }

            override fun onRegistrationFailed(serviceInfo: NsdServiceInfo, errorCode: Int) {
                Log.w(TAG, "NSD registration failed code=$errorCode")
                synchronized(this@NsdAdvertiser) {
                    if (listener === this) {
                        listener = null
                        advertisedNetwork = null
                        advertisedPort = null
                        scheduleRetryLocked()
                    }
                }
            }

            override fun onServiceUnregistered(serviceInfo: NsdServiceInfo) {
                Log.i(TAG, "NSD unregistered")
            }

            override fun onUnregistrationFailed(serviceInfo: NsdServiceInfo, errorCode: Int) {
                Log.w(TAG, "NSD unregistration failed code=$errorCode")
            }
        }
        listener = registration
        advertisedNetwork = network
        advertisedPort = port
        val service = NsdServiceInfo().apply {
            serviceName = "GalaxyTTY-${safeModelName()}"
            serviceType = SERVICE_TYPE
            setPort(port)
            if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.TIRAMISU) {
                setNetwork(network)
            }
        }
        try {
            manager.registerService(service, NsdManager.PROTOCOL_DNS_SD, registration)
        } catch (error: RuntimeException) {
            if (listener === registration) {
                listener = null
                advertisedNetwork = null
                advertisedPort = null
                Log.w(TAG, "NSD registration rejected: ${error.javaClass.simpleName}")
                scheduleRetryLocked()
            }
        }
    }

    @Synchronized
    fun stop() {
        desiredNetwork = null
        desiredPort = null
        retryHandler.removeCallbacks(retryRegistration)
        stopRegistrationLocked()
    }

    private fun stopRegistrationLocked() {
        val registration = listener ?: return
        listener = null
        advertisedNetwork = null
        advertisedPort = null
        try {
            manager.unregisterService(registration)
        } catch (error: IllegalArgumentException) {
            Log.w(TAG, "NSD was already unregistered")
        }
    }

    private fun scheduleRetryLocked() {
        if (desiredNetwork == null || desiredPort == null) return
        retryHandler.removeCallbacks(retryRegistration)
        retryHandler.postDelayed(retryRegistration, RETRY_DELAY_MILLIS)
    }

    private fun safeModelName(): String = Build.MODEL
        .replace(Regex("[^A-Za-z0-9_-]"), "-")
        .take(40)
        .ifBlank { "Galaxy" }

    companion object {
        const val SERVICE_TYPE = "_galaxytty._tcp."
        private const val TAG = "GalaxyTTY-TCP"
        private const val RETRY_DELAY_MILLIS = 2_000L
    }
}
