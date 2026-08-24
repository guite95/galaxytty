package com.galaxytty.helper.tcp

import android.content.Context
import android.util.Log
import com.galaxytty.helper.message.CompositeMessageStore
import com.galaxytty.helper.message.LiveMessageStore
import com.galaxytty.helper.message.SmsRepository
import com.galaxytty.helper.notification.NotificationObservation
import com.galaxytty.helper.samsung.ReplyAction
import com.galaxytty.helper.samsung.ReplyActionRegistry

object BridgeRuntime {
    private val messages = LiveMessageStore()
    private val replies = ReplyActionRegistry()
    private var server: GalaxyTcpServer? = null

    @Synchronized
    fun start(context: Context): Int {
        val running = server ?: run {
            val smsHistory = SmsRepository(context.applicationContext)
            val combined = CompositeMessageStore(messages, smsHistory) { error ->
                Log.w(TAG, "SMS history unavailable: ${error.javaClass.simpleName}")
            }
            GalaxyTcpServer(
                context = context,
                messageStore = combined,
                replyActions = replies,
                smsHistoryAvailable = smsHistory::available,
            ).also { server = it }
        }
        return running.start()
    }

    @Synchronized
    fun port(): Int? = server?.port()

    fun publish(observation: NotificationObservation) {
        val contentIncluded = observation.message?.let(messages::add) == true
        synchronized(this) { server }?.publish(observation, contentIncluded)
    }

    fun updateReply(notificationKey: String, threadId: Long?, action: ReplyAction?) {
        if (threadId == null || action == null) {
            replies.unregister(notificationKey)
            return
        }
        replies.register(notificationKey, threadId, action)
    }

    fun unregisterReply(notificationKey: String) {
        replies.unregister(notificationKey)
    }

    @Synchronized
    fun stop() {
        server?.close()
        server = null
    }

    private const val TAG = "GalaxyTTY"
}
