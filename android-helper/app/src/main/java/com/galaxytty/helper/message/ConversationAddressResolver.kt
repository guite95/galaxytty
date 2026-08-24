package com.galaxytty.helper.message

fun interface ConversationAddressResolver {
    fun oneToOneAddress(threadId: Long): String?
}

class CompositeConversationAddressResolver(
    private vararg val resolvers: ConversationAddressResolver,
) : ConversationAddressResolver {
    override fun oneToOneAddress(threadId: Long): String? = resolvers
        .asSequence()
        .mapNotNull { resolver -> resolver.oneToOneAddress(threadId)?.trim() }
        .firstOrNull(String::isNotEmpty)
}
