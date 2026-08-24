package com.galaxytty.helper.samsung

import android.Manifest
import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.net.Uri
import android.os.Build
import com.galaxytty.helper.R
import java.util.ArrayDeque
import java.util.concurrent.atomic.AtomicInteger

class SamsungComposeNotificationPublisher(
    context: Context,
) : ComposeRequestPublisher {
    private val appContext = context.applicationContext
    private val notificationManager = appContext.getSystemService(NotificationManager::class.java)
    private val notificationIds = ArrayDeque<Int>()

    override fun publish(address: String, text: String): Boolean {
        val normalizedAddress = SafePhoneAddress.normalize(address) ?: return false
        if (text.isBlank()) return false
        if (!notificationsAllowed()) return false

        val composeIntent = Intent(
            Intent.ACTION_SENDTO,
            Uri.fromParts(SMSTO_SCHEME, normalizedAddress, null),
        ).apply {
            setPackage(SAMSUNG_MESSAGES_PACKAGE)
            putExtra(SMS_BODY_EXTRA, text)
            addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)
        }
        if (composeIntent.resolveActivity(appContext.packageManager) == null) return false

        createChannel()
        val notificationId = nextNotificationId()
        val contentIntent = PendingIntent.getActivity(
            appContext,
            notificationId,
            composeIntent,
            PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_IMMUTABLE,
        )
        val notification = Notification.Builder(appContext, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_bridge)
            .setContentTitle(appContext.getString(R.string.compose_handoff_title))
            .setContentText(appContext.getString(R.string.compose_handoff_text))
            .setContentIntent(contentIntent)
            .setCategory(Notification.CATEGORY_MESSAGE)
            .setAutoCancel(true)
            .setLocalOnly(true)
            .setOnlyAlertOnce(true)
            .build()
        notificationManager.notify(notificationId, notification)
        retain(notificationId)
        return true
    }

    private fun notificationsAllowed(): Boolean {
        if (!notificationManager.areNotificationsEnabled()) return false
        return Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
            appContext.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) ==
            PackageManager.PERMISSION_GRANTED
    }

    private fun createChannel() {
        notificationManager.createNotificationChannel(
            NotificationChannel(
                CHANNEL_ID,
                appContext.getString(R.string.compose_handoff_channel),
                NotificationManager.IMPORTANCE_DEFAULT,
            ).apply {
                description = appContext.getString(R.string.compose_handoff_channel_description)
                setShowBadge(false)
            },
        )
    }

    @Synchronized
    private fun retain(notificationId: Int) {
        notificationIds.addLast(notificationId)
        while (notificationIds.size > MAX_RETAINED_NOTIFICATIONS) {
            notificationManager.cancel(notificationIds.removeFirst())
        }
    }

    private fun nextNotificationId(): Int = NOTIFICATION_IDS.updateAndGet { previous ->
        if (previous >= LAST_NOTIFICATION_ID) FIRST_NOTIFICATION_ID else previous + 1
    }

    companion object {
        private const val SAMSUNG_MESSAGES_PACKAGE = "com.samsung.android.messaging"
        private const val SMSTO_SCHEME = "smsto"
        private const val SMS_BODY_EXTRA = "sms_body"
        private const val CHANNEL_ID = "galaxytty_compose_handoff"
        private const val FIRST_NOTIFICATION_ID = 2_000
        private const val LAST_NOTIFICATION_ID = 9_999
        private const val MAX_RETAINED_NOTIFICATIONS = 20
        private val NOTIFICATION_IDS = AtomicInteger(FIRST_NOTIFICATION_ID - 1)
    }
}
