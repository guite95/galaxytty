package com.galaxytty.helper.service

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.app.Service
import android.content.Context
import android.content.Intent
import android.content.pm.ServiceInfo
import android.os.Build
import android.os.IBinder
import android.util.Log
import com.galaxytty.helper.MainActivity
import com.galaxytty.helper.R
import com.galaxytty.helper.tcp.BridgeRuntime

class BridgeForegroundService : Service() {
    override fun onCreate() {
        super.onCreate()
        running = true
        createNotificationChannel()
        startAsForeground(buildNotification("Starting local bridge…"))
        try {
            val port = BridgeRuntime.start(applicationContext)
            notificationManager().notify(NOTIFICATION_ID, buildNotification("Available on local Wi-Fi · port $port"))
            Log.i(TAG, "background bridge started port=$port")
        } catch (error: RuntimeException) {
            Log.e(TAG, "background bridge failed: ${error.javaClass.simpleName}")
            stopSelf()
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int = START_STICKY

    override fun onBind(intent: Intent?): IBinder? = null

    override fun onDestroy() {
        BridgeRuntime.stop()
        running = false
        Log.i(TAG, "background bridge stopped")
        super.onDestroy()
    }

    private fun startAsForeground(notification: Notification) {
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.UPSIDE_DOWN_CAKE) {
            startForeground(
                NOTIFICATION_ID,
                notification,
                ServiceInfo.FOREGROUND_SERVICE_TYPE_REMOTE_MESSAGING,
            )
        } else {
            startForeground(NOTIFICATION_ID, notification)
        }
    }

    private fun createNotificationChannel() {
        val channel = NotificationChannel(
            CHANNEL_ID,
            "GalaxyTTY background bridge",
            NotificationManager.IMPORTANCE_LOW,
        ).apply {
            description = "Keeps the local GalaxyTTY connection available"
            setShowBadge(false)
        }
        notificationManager().createNotificationChannel(channel)
    }

    private fun buildNotification(status: String): Notification {
        val activityIntent = Intent(this, MainActivity::class.java)
            .addFlags(Intent.FLAG_ACTIVITY_SINGLE_TOP)
        val pendingIntent = PendingIntent.getActivity(
            this,
            0,
            activityIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        return Notification.Builder(this, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_bridge)
            .setContentTitle("GalaxyTTY bridge is running")
            .setContentText(status)
            .setContentIntent(pendingIntent)
            .setCategory(Notification.CATEGORY_SERVICE)
            .setOngoing(true)
            .setOnlyAlertOnce(true)
            .setShowWhen(false)
            .build()
    }

    private fun notificationManager(): NotificationManager =
        getSystemService(NotificationManager::class.java)

    companion object {
        private const val CHANNEL_ID = "galaxytty_bridge"
        private const val NOTIFICATION_ID = 1001
        private const val TAG = "GalaxyTTY-TCP"

        @Volatile
        var running: Boolean = false
            private set

        fun start(context: Context, remember: Boolean = true) {
            if (remember) BridgePreferences.setEnabled(context, true)
            val intent = Intent(context, BridgeForegroundService::class.java)
            context.startForegroundService(intent)
        }

        fun stop(context: Context) {
            BridgePreferences.setEnabled(context, false)
            context.stopService(Intent(context, BridgeForegroundService::class.java))
        }
    }
}
