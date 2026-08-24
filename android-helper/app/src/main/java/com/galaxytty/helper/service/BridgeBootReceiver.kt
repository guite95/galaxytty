package com.galaxytty.helper.service

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.util.Log

class BridgeBootReceiver : BroadcastReceiver() {
    override fun onReceive(context: Context, intent: Intent) {
        if (!BridgeStartupPolicy.shouldRestart(BridgePreferences.enabled(context), intent.action)) return
        try {
            BridgeForegroundService.start(context, remember = false)
        } catch (error: RuntimeException) {
            Log.w(TAG, "bridge restart rejected: ${error.javaClass.simpleName}")
        }
    }

    companion object {
        private const val TAG = "GalaxyTTY"
    }
}
