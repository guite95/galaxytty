package com.galaxytty.helper.security

import java.security.MessageDigest
import java.util.Base64

internal object SessionAuthenticator {
    const val MODE = "hmac-sha256"
    private val clientIdPattern = Regex("^[A-Za-z0-9_-]{8,64}$")

    fun verify(
        secret: ByteArray,
        deviceId: String,
        clientId: String,
        challenge: String,
        encodedProof: String,
    ): Boolean {
        if (!clientIdPattern.matches(clientId)) return false
        val received = try {
            Base64.getUrlDecoder().decode(encodedProof)
        } catch (_: IllegalArgumentException) {
            return false
        }
        val expected = AuthProof.create(secret, deviceId, clientId, challenge)
        return MessageDigest.isEqual(expected, received)
    }
}
