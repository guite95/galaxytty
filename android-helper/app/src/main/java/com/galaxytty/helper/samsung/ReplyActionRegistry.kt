package com.galaxytty.helper.samsung

fun interface ReplyAction {
    fun send(text: String)
}

fun interface ReplyExecutionPolicy {
    fun allowsExecution(): Boolean
}

object DisabledReplyExecutionPolicy : ReplyExecutionPolicy {
    override fun allowsExecution(): Boolean = false
}

enum class ReplyDispatchStatus {
    DISABLED,
    INVALID_REQUEST,
    ACTION_UNAVAILABLE,
    ACCEPTED_UNVERIFIED,
    FAILED,
}

data class ReplyDispatchResult(
    val status: ReplyDispatchStatus,
    val error: String? = null,
)

class ReplyActionRegistry(
    private val policy: ReplyExecutionPolicy = DisabledReplyExecutionPolicy,
    private val maxActions: Int = 100,
) {
    private data class RegisteredReply(
        val notificationKey: String,
        val action: ReplyAction,
    )

    private val actions = LinkedHashMap<Long, RegisteredReply>()

    @Synchronized
    fun register(notificationKey: String, threadId: Long, action: ReplyAction) {
        if (notificationKey.isBlank() || threadId <= 0) return
        actions.remove(threadId)
        actions[threadId] = RegisteredReply(notificationKey, action)
        while (actions.size > maxActions) {
            actions.remove(actions.entries.first().key)
        }
    }

    @Synchronized
    fun unregister(notificationKey: String) {
        val matching = actions
            .filterValues { registered -> registered.notificationKey == notificationKey }
            .keys
        matching.forEach(actions::remove)
    }

    @Synchronized
    fun available(threadId: Long): Boolean = threadId > 0 && actions.containsKey(threadId)

    @Synchronized
    fun dispatch(threadId: Long, text: String): ReplyDispatchResult {
        if (threadId <= 0 || text.isBlank() || text.length > MAX_TEXT_LENGTH) {
            return ReplyDispatchResult(ReplyDispatchStatus.INVALID_REQUEST, "A valid thread and message text are required")
        }
        if (!policy.allowsExecution()) {
            return ReplyDispatchResult(
                ReplyDispatchStatus.DISABLED,
                "RemoteInput execution is disabled until an explicit real-send authorization",
            )
        }
        val registered = actions[threadId]
            ?: return ReplyDispatchResult(
                ReplyDispatchStatus.ACTION_UNAVAILABLE,
                "No active RemoteInput reply action is available for this conversation",
            )
        return try {
            registered.action.send(text)
            ReplyDispatchResult(ReplyDispatchStatus.ACCEPTED_UNVERIFIED)
        } catch (error: Exception) {
            ReplyDispatchResult(
                ReplyDispatchStatus.FAILED,
                "Samsung Messages rejected the RemoteInput action (${error.javaClass.simpleName})",
            )
        }
    }

    companion object {
        private const val MAX_TEXT_LENGTH = 4_000
    }
}
