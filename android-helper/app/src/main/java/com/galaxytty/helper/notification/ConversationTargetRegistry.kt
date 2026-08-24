package com.galaxytty.helper.notification

import com.galaxytty.helper.message.ConversationAddressResolver

class ConversationTargetRegistry(
    private val maxTargets: Int = 100,
) : ConversationAddressResolver {
    private val targets = LinkedHashMap<Long, String>()

    init {
        require(maxTargets > 0) { "maxTargets must be positive" }
    }

    @Synchronized
    fun register(threadId: Long, address: String?) {
        val normalized = address?.trim().orEmpty()
        if (threadId <= 0 || normalized.isEmpty()) return
        targets.remove(threadId)
        targets[threadId] = normalized
        while (targets.size > maxTargets) {
            targets.remove(targets.entries.first().key)
        }
    }

    @Synchronized
    override fun oneToOneAddress(threadId: Long): String? = targets[threadId]
}
