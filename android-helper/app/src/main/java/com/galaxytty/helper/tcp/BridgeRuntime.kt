package com.galaxytty.helper.tcp

import android.content.Context
import android.content.pm.ApplicationInfo
import android.util.Log
import com.galaxytty.helper.message.CompositeMessageStore
import com.galaxytty.helper.message.CompositeConversationAddressResolver
import com.galaxytty.helper.message.LiveMessageStore
import com.galaxytty.helper.message.SmsRepository
import com.galaxytty.helper.notification.ConversationTargetRegistry
import com.galaxytty.helper.notification.NotificationObservation
import com.galaxytty.helper.service.RemoteReplyPreferences
import com.galaxytty.helper.samsung.AuthorizedReplyExecutionPolicy
import com.galaxytty.helper.samsung.AddressedComposeReplyFallback
import com.galaxytty.helper.samsung.ReplyAction
import com.galaxytty.helper.samsung.ReplyActionRegistry
import com.galaxytty.helper.samsung.ReplyExecutionPolicy
import com.galaxytty.helper.samsung.OneShotReplyExecutionPolicy
import com.galaxytty.helper.samsung.SamsungComposeNotificationPublisher
import java.io.File

object BridgeRuntime {
    private val messages = LiveMessageStore()
    private val notificationTargets = ConversationTargetRegistry()
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
        val appContext = context.applicationContext
        if (replyExecution == null) {
            val oneShot = OneShotReplyExecutionPolicy(
                markerFile = File(appContext.filesDir, OneShotReplyExecutionPolicy.MARKER_FILE_NAME),
                enabled = context.applicationInfo.flags and ApplicationInfo.FLAG_DEBUGGABLE != 0,
            )
            oneShotReplyExecution = oneShot
            replyExecution = AuthorizedReplyExecutionPolicy(
                locallyAllowed = { RemoteReplyPreferences.allowed(appContext) },
                oneShot = oneShot,
            )
        }
        val running = server ?: run {
            val smsHistory = SmsRepository(appContext)
            val combined = CompositeMessageStore(messages, smsHistory) { error ->
                Log.w(TAG, "SMS history unavailable: ${error.javaClass.simpleName}")
            }
            GalaxyTcpServer(
                context = context,
                messageStore = combined,
                replyActions = replies,
                replyFallback = AddressedComposeReplyFallback(
                    addresses = CompositeConversationAddressResolver(notificationTargets, smsHistory),
                    publisher = SamsungComposeNotificationPublisher(appContext),
                ),
                smsHistoryAvailable = smsHistory::available,
            ).also { server = it }
        }
        return running.start()
    }

    @Synchronized
    fun port(): Int? = server?.port()

    fun replyTestArmed(): Boolean = oneShotReplyExecution?.isArmed() == true

    fun replyActionCounts() = replies.counts()

    fun publish(observation: NotificationObservation) {
        observation.message?.let { message ->
            notificationTargets.register(message.threadId, message.replyAddress)
        }
        val contentIncluded = observation.message?.let(messages::add) == true
        synchronized(this) { server }?.publish(observation, contentIncluded)
    }

    fun updateReply(notificationKey: String, threadId: Long?, action: ReplyAction?) {
        if (threadId == null || action == null) {
            replies.markInactive(notificationKey)
            return
        }
        replies.register(notificationKey, threadId, action)
    }

    fun markReplyInactive(notificationKey: String): Boolean = replies.markInactive(notificationKey)

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
