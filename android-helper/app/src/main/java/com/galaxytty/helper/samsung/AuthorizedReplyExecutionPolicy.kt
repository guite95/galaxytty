package com.galaxytty.helper.samsung

class AuthorizedReplyExecutionPolicy(
    private val locallyAllowed: () -> Boolean,
    private val oneShot: ReplyExecutionPolicy = DisabledReplyExecutionPolicy,
) : ReplyExecutionPolicy {
    override fun allowsExecution(): Boolean = locallyAllowed() || oneShot.allowsExecution()
}
