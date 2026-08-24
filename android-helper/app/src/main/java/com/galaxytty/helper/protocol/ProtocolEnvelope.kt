package com.galaxytty.helper.protocol

import org.json.JSONObject

data class ProtocolEnvelope(
    val version: Int,
    val type: String,
    val requestId: String?,
    val sequence: Long,
    val payload: JSONObject,
) {
    fun encode(): String = JSONObject()
        .put("version", version)
        .put("type", type)
        .apply {
            if (!requestId.isNullOrBlank()) put("requestId", requestId)
            if (sequence > 0) put("sequence", sequence)
        }
        .put("payload", payload)
        .toString()

    companion object {
        const val CURRENT_VERSION = 1

        fun decode(json: String): ProtocolEnvelope {
            val root = try {
                JSONObject(json)
            } catch (error: Exception) {
                throw ProtocolException("invalid JSON envelope", error)
            }
            val version = root.optInt("version", -1)
            if (version != CURRENT_VERSION) throw ProtocolException("unsupported protocol version: $version")
            val type = root.optString("type")
            if (type !in ProtocolTypes.known) throw ProtocolException("unknown protocol type")
            val payload = root.optJSONObject("payload") ?: throw ProtocolException("payload must be an object")
            return ProtocolEnvelope(
                version = version,
                type = type,
                requestId = root.optString("requestId").takeIf(String::isNotBlank),
                sequence = root.optLong("sequence", 0),
                payload = payload,
            )
        }

        fun create(
            type: String,
            requestId: String? = null,
            sequence: Long = 0,
            payload: JSONObject = JSONObject(),
        ): ProtocolEnvelope {
            if (type !in ProtocolTypes.known) throw ProtocolException("unknown protocol type")
            return ProtocolEnvelope(CURRENT_VERSION, type, requestId, sequence, payload)
        }
    }
}

object ProtocolTypes {
    const val HELLO = "HELLO"
    const val AUTH = "AUTH"
    const val SECURE = "SECURE"
    const val PING = "PING"
    const val PONG = "PONG"
    const val MESSAGE_RECEIVED = "MESSAGE_RECEIVED"
    const val GET_CONVERSATIONS = "GET_CONVERSATIONS"
    const val CONVERSATIONS = "CONVERSATIONS"
    const val GET_MESSAGES = "GET_MESSAGES"
    const val MESSAGES = "MESSAGES"
    const val SEND_MESSAGE = "SEND_MESSAGE"
    const val GET_REPLY_CAPABILITY = "GET_REPLY_CAPABILITY"
    const val REPLY_CAPABILITY = "REPLY_CAPABILITY"
    const val SEND_REPLY = "SEND_REPLY"
    const val SEND_RESULT = "SEND_RESULT"
    const val SYNC_REQUEST = "SYNC_REQUEST"
    const val SYNC_MESSAGE = "SYNC_MESSAGE"
    const val ERROR = "ERROR"

    val known = setOf(
        HELLO,
        AUTH,
        SECURE,
        PING,
        PONG,
        MESSAGE_RECEIVED,
        GET_CONVERSATIONS,
        CONVERSATIONS,
        GET_MESSAGES,
        MESSAGES,
        SEND_MESSAGE,
        GET_REPLY_CAPABILITY,
        REPLY_CAPABILITY,
        SEND_REPLY,
        SEND_RESULT,
        SYNC_REQUEST,
        SYNC_MESSAGE,
        ERROR,
    )
}
