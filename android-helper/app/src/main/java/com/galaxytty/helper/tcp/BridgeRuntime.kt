package com.galaxytty.helper.tcp

import android.content.Context
import android.content.pm.ApplicationInfo
import android.util.Log
import com.galaxytty.helper.message.CompositeMessageStore
import com.galaxytty.helper.message.LiveMessageStore
import com.galaxytty.helper.message.SmsRepository
import com.galaxytty.helper.notification.NotificationObservation
import com.galaxytty.helper.service.RemoteReplyPreferences
import com.galaxytty.helper.samsung.AuthorizedReplyExecutionPolicy
import com.galaxytty.helper.samsung.ReplyAction
import com.galaxytty.helper.samsung.ReplyActionRegistry
import com.galaxytty.helper.samsung.ReplyExecutionPolicy
import com.galaxytty.helper.samsung.OneShotReplyExecutionPolicy
import java.io.File

object BridgeRuntime {
    private val messages = LiveMessageStore()
    @Volatile
    private var oneShotReplyExecution: OneShotReplyExecutionPolicy? = null
    @Volatile
    private var replyExecution: ReplyExecutionPolicy? = null
    private val replies = ReplyActionRegistry(
        policy = ReplyExecutionPolicy { replyExecution?.allowsExecution() == true },
    )
    private var server: GalaxyTcpServer? = null

    @Synchronized
    fun start(context: Context): Int {
        if (replyExecution == null) {
            val appContext = context.applicationContext
            val oneShot = OneShotReplyExecutionPolicy(
                markerFile = File(context.applicationContext.filesDir, OneShotReplyExecutionPolicy.MARKER_FILE_NAME),
                enabled = context.applicationInfo.flags and ApplicationInfo.FLAG_DEBUGGABLE != 0,
            )
            oneShotReplyExecution = oneShot
            replyExecution = AuthorizedReplyExecutionPolicy(
                locallyAllowed = { RemoteReplyPreferences.allowed(appContext) },
                oneShot = oneShot,
            )
        }
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

    fun replyTestArmed(): Boolean = oneShotReplyExecution?.isArmed() == true

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
        oneShotReplyExecution?.clear()
        oneShotReplyExecution = null
        replyExecution = null
    }

    private const val TAG = "GalaxyTTY"
}
