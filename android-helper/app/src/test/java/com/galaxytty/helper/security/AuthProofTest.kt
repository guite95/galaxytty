package com.galaxytty.helper.security

import org.junit.Assert.assertEquals
import org.junit.Test

class AuthProofTest {
    @Test
    fun matchesCrossPlatformProofVector() {
        val secret = ByteArray(20) { it.toByte() }
        val proof = AuthProof.create(secret, "device-1", "client-1", "challenge-1")

        assertEquals(
            "0cba84724a158c5f22774a19bc907252d9482980a7f0d77ac2f87a428fd228e4",
            proof.joinToString("") { byte -> "%02x".format(byte.toInt() and 0xff) },
        )
        val serverProof = AuthProof.createServer(secret, "device-1", "client-1", "challenge-1")
        assertEquals(
            "d0a7cda23a4f9203374e49ae9614da85d76ec19641dbc706df78dda42210853b",
            serverProof.joinToString("") { byte -> "%02x".format(byte.toInt() and 0xff) },
        )
    }

    @Test
    fun base32FormattingMatchesMacImplementation() {
        val secret = ByteArray(20) { it.toByte() }

        assertEquals("AAAQ-EAYE-AUDA-OCAJ-BIFQ-YDIO-B4IB-CEQT", Base32Code.format(secret))
    }
}
