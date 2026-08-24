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
    USER_ACTION_REQUIRED,
    FAILED,
}

data class ReplyDispatchResult(
    val status: ReplyDispatchStatus,
    val error: String? = null,
    val evidence: String? = null,
)

data class ReplyActionCounts(
    val active: Int,
    val cached: Int,
)

class ReplyActionRegistry(
    private val policy: ReplyExecutionPolicy = DisabledReplyExecutionPolicy,
    private val maxActions: Int = 100,
    private val retainedActionTtlMillis: Long = DEFAULT_RETAINED_ACTION_TTL_MILLIS,
    private val nowMillis: () -> Long = { System.nanoTime() / NANOS_PER_MILLISECOND },
) {
    private data class RegisteredReply(
        val notificationKey: String,
        val action: ReplyAction,
        val inactiveSinceMillis: Long? = null,
    )

    private val actions = LinkedHashMap<Long, RegisteredReply>()

    init {
        require(maxActions > 0) { "maxActions must be positive" }
        require(retainedActionTtlMillis > 0) { "retainedActionTtlMillis must be positive" }
    }

    @Synchronized
    fun register(notificationKey: String, threadId: Long, action: ReplyAction) {
        if (notificationKey.isBlank() || threadId <= 0) return
        pruneExpired()
        actions.entries.removeAll { (_, registered) -> registered.notificationKey == notificationKey }
        actions.remove(threadId)
        actions[threadId] = RegisteredReply(notificationKey, action)
        while (actions.size > maxActions) {
            actions.remove(actions.entries.first().key)
        }
    }

    @Synchronized
    fun markInactive(notificationKey: String): Boolean {
        if (notificationKey.isBlank()) return false
        pruneExpired()
        val inactiveSinceMillis = nowMillis()
        var retained = false
        actions.replaceAll { _, registered ->
            if (registered.notificationKey == notificationKey) {
                retained = true
                registered.copy(inactiveSinceMillis = registered.inactiveSinceMillis ?: inactiveSinceMillis)
            } else {
                registered
            }
        }
        return retained
    }

    @Synchronized
    fun unregister(notificationKey: String) {
        val matching = actions
            .filterValues { registered -> registered.notificationKey == notificationKey }
            .keys
        matching.forEach(actions::remove)
    }

    @Synchronized
    fun available(threadId: Long): Boolean {
        pruneExpired()
        return threadId > 0 && actions[threadId]
            ?.let { registered -> registered.inactiveSinceMillis == null } == true
    }

    @Synchronized
    fun cachedAvailable(threadId: Long): Boolean {
        pruneExpired()
        return threadId > 0 && actions[threadId]?.inactiveSinceMillis != null
    }

    @Synchronized
    fun counts(): ReplyActionCounts {
        pruneExpired()
        return ReplyActionCounts(
            active = actions.count { (_, registered) -> registered.inactiveSinceMillis == null },
            cached = actions.count { (_, registered) -> registered.inactiveSinceMillis != null },
        )
    }

    @Synchronized
    fun dispatch(threadId: Long, text: String): ReplyDispatchResult {
        if (threadId <= 0 || text.isBlank() || text.length > MAX_TEXT_LENGTH) {
            return ReplyDispatchResult(ReplyDispatchStatus.INVALID_REQUEST, "A valid thread and message text are required")
        }
        if (!policy.allowsExecution()) {
            return ReplyDispatchResult(
                ReplyDispatchStatus.DISABLED,
                "Replies are blocked on the Galaxy; enable replies locally in GalaxyTTY Helper",
            )
        }
        pruneExpired()
        val registered = actions[threadId]
            ?: return ReplyDispatchResult(
                ReplyDispatchStatus.ACTION_UNAVAILABLE,
                "No active or retained RemoteInput reply action is available for this conversation",
            )
        return try {
            registered.action.send(text)
            ReplyDispatchResult(
                status = ReplyDispatchStatus.ACCEPTED_UNVERIFIED,
                evidence = if (registered.inactiveSinceMillis == null) {
                    "remote_input_pending_intent_accepted"
                } else {
                    "retained_remote_input_pending_intent_accepted"
                },
            )
        } catch (error: Exception) {
            actions.remove(threadId)
            ReplyDispatchResult(
                ReplyDispatchStatus.FAILED,
                "Samsung Messages rejected the RemoteInput action (${error.javaClass.simpleName})",
            )
        }
    }

    private fun pruneExpired() {
        val now = nowMillis()
        val expired = actions
            .filterValues { registered ->
                registered.inactiveSinceMillis?.let { inactiveSince ->
                    now - inactiveSince >= retainedActionTtlMillis
                } == true
            }
            .keys
        expired.forEach(actions::remove)
    }

    companion object {
        private const val MAX_TEXT_LENGTH = 4_000
        private const val NANOS_PER_MILLISECOND = 1_000_000L
        const val DEFAULT_RETAINED_ACTION_TTL_MILLIS = 24L * 60L * 60L * 1_000L
    }
}
