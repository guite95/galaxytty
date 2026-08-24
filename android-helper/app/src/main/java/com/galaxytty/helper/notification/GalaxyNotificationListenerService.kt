package com.galaxytty.helper.notification

import android.service.notification.NotificationListenerService
import android.service.notification.StatusBarNotification
import android.util.Log
import com.galaxytty.helper.tcp.BridgeRuntime

class GalaxyNotificationListenerService : NotificationListenerService() {
    private val adapter by lazy { SamsungMessagesAdapter(applicationContext) }

    override fun onListenerConnected() {
        super.onListenerConnected()
        Log.i(TAG, "listener connected")
        activeNotifications
            ?.asSequence()
            ?.filter { notification -> notification.packageName == NotificationAnalyzer.SAMSUNG_MESSAGES_PACKAGE }
            ?.forEach(::observe)
    }

    override fun onListenerDisconnected() {
        Log.w(TAG, "listener disconnected")
        super.onListenerDisconnected()
    }

    override fun onNotificationPosted(statusBarNotification: StatusBarNotification) {
        observe(statusBarNotification)
    }

    override fun onNotificationRemoved(statusBarNotification: StatusBarNotification) {
        if (statusBarNotification.packageName != NotificationAnalyzer.SAMSUNG_MESSAGES_PACKAGE) return
        BridgeRuntime.unregisterReply(statusBarNotification.key)
        Log.i(TAG, "notification removed key=${SafeFingerprint.of(statusBarNotification.key)}")
    }

    private fun observe(statusBarNotification: StatusBarNotification) {
        val observation = adapter.observe(statusBarNotification) ?: return
        val message = observation.message
        val replyAction = message?.let { adapter.replyAction(statusBarNotification) }
        BridgeRuntime.updateReply(statusBarNotification.key, message?.threadId, replyAction)
        ObservationRepository.add(observation)
        BridgeRuntime.publish(observation)
        Log.i(
            NOTIFICATION_TAG,
            "${observation.safeSummary()} replyActionRegistered=${replyAction != null}",
        )
    }

    companion object {
        private const val TAG = "GalaxyTTY"
        private const val NOTIFICATION_TAG = "GalaxyTTY-Notification"
    }
}
