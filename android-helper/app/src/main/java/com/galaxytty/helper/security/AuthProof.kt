package com.galaxytty.helper.security

import java.nio.charset.StandardCharsets
import javax.crypto.Mac
import javax.crypto.SecretKey
import javax.crypto.spec.SecretKeySpec

internal object AuthProof {
    private const val ALGORITHM = "HmacSHA256"
    private const val TRANSCRIPT_DOMAIN = "galaxytty-auth-v1"
    private const val SERVER_TRANSCRIPT_DOMAIN = "galaxytty-auth-server-v1"

    fun create(secret: ByteArray, deviceId: String, clientId: String, challenge: String): ByteArray =
        create(SecretKeySpec(secret, ALGORITHM), TRANSCRIPT_DOMAIN, deviceId, clientId, challenge)

    fun create(key: SecretKey, deviceId: String, clientId: String, challenge: String): ByteArray =
        create(key, TRANSCRIPT_DOMAIN, deviceId, clientId, challenge)

    fun createServer(secret: ByteArray, deviceId: String, clientId: String, challenge: String): ByteArray =
        create(SecretKeySpec(secret, ALGORITHM), SERVER_TRANSCRIPT_DOMAIN, deviceId, clientId, challenge)

    private fun create(
        key: SecretKey,
        domain: String,
        deviceId: String,
        clientId: String,
        challenge: String,
    ): ByteArray = hmac(key, transcript(domain, deviceId, clientId, challenge))

    fun hmac(key: SecretKey, value: String): ByteArray {
        val mac = Mac.getInstance(ALGORITHM)
        mac.init(key)
        return mac.doFinal(value.toByteArray(StandardCharsets.UTF_8))
    }

    private fun transcript(domain: String, deviceId: String, clientId: String, challenge: String): String =
        listOf(domain, deviceId, clientId, challenge).joinToString("\u0000")
}
