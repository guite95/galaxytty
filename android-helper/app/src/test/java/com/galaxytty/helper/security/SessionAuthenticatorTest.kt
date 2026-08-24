package com.galaxytty.helper.security

import java.util.Base64
import org.junit.Assert.assertFalse
import org.junit.Assert.assertTrue
import org.junit.Test

class SessionAuthenticatorTest {
    private val secret = ByteArray(20) { it.toByte() }

    @Test
    fun acceptsValidChallengeProof() {
        val proof = Base64.getUrlEncoder().withoutPadding().encodeToString(
            AuthProof.create(secret, "device-1", "client-123", "challenge-1"),
        )

        assertTrue(SessionAuthenticator.verify(secret, "device-1", "client-123", "challenge-1", proof))
    }

    @Test
    fun rejectsReplayAndMalformedInputs() {
        val proof = Base64.getUrlEncoder().withoutPadding().encodeToString(
            AuthProof.create(secret, "device-1", "client-123", "challenge-1"),
        )

        assertFalse(SessionAuthenticator.verify(secret, "device-1", "client-123", "challenge-2", proof))
        assertFalse(SessionAuthenticator.verify(secret, "device-1", "bad client", "challenge-1", proof))
        assertFalse(SessionAuthenticator.verify(secret, "device-1", "client-123", "challenge-1", "not-base64!"))
    }
}
