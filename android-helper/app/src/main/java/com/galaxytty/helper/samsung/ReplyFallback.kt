package com.galaxytty.helper.samsung

import com.galaxytty.helper.message.ConversationAddressResolver

fun interface ComposeRequestPublisher {
    fun publish(address: String, text: String): Boolean
}

fun interface ReplyFallback {
    fun prepare(threadId: Long, text: String): ReplyDispatchResult
}

object DisabledReplyFallback : ReplyFallback {
    override fun prepare(threadId: Long, text: String): ReplyDispatchResult = ReplyDispatchResult(
        status = ReplyDispatchStatus.ACTION_UNAVAILABLE,
        error = "No active RemoteInput reply action or safe compose target is available for this conversation",
    )
}

class ReplyDispatcher(
    private val actions: ReplyActionRegistry,
    private val fallback: ReplyFallback = DisabledReplyFallback,
) {
    fun dispatch(threadId: Long, text: String): ReplyDispatchResult {
        val directResult = actions.dispatch(threadId, text)
        return if (directResult.status == ReplyDispatchStatus.ACTION_UNAVAILABLE) {
            fallback.prepare(threadId, text)
        } else {
            directResult
        }
    }
}

class AddressedComposeReplyFallback(
    private val addresses: ConversationAddressResolver,
    private val publisher: ComposeRequestPublisher,
) : ReplyFallback {
    override fun prepare(threadId: Long, text: String): ReplyDispatchResult {
        val address = try {
            addresses.oneToOneAddress(threadId)
        } catch (error: RuntimeException) {
            return ReplyDispatchResult(
                status = ReplyDispatchStatus.FAILED,
                error = "Conversation target lookup failed (${error.javaClass.simpleName})",
            )
        } ?: return DisabledReplyFallback.prepare(threadId, text)
        return try {
            if (publisher.publish(address, text)) {
                ReplyDispatchResult(
                    status = ReplyDispatchStatus.USER_ACTION_REQUIRED,
                    evidence = "samsung_compose_notification_posted",
                )
            } else {
                ReplyDispatchResult(
                    status = ReplyDispatchStatus.FAILED,
                    error = "Samsung Messages compose handoff is unavailable on this Galaxy",
                )
            }
        } catch (error: RuntimeException) {
            ReplyDispatchResult(
                status = ReplyDispatchStatus.FAILED,
                error = "Samsung Messages compose handoff failed (${error.javaClass.simpleName})",
            )
        }
    }
}
