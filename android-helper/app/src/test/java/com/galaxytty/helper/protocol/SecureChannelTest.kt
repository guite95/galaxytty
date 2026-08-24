package com.galaxytty.helper.protocol

import java.nio.charset.StandardCharsets
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class SecureChannelTest {
    @Test
    fun sessionMaterialMatchesGoVector() {
        val material = SecureChannel.deriveSessionMaterial(secret(), challenge())

        assertEquals("63ddb312c9aab6b965b5d8e87b238bb8d5165ba70f21e7d3ac2d6e896638daf4", material.clientToServerKey.hex())
        assertEquals("10910caf0c2e2dc1dd2eb5e763da0b2789c3318b482a2ef6d6bdda07e272d8b1", material.serverToClientKey.hex())
        assertEquals("56c98f09", material.clientToServerNonce.hex())
        assertEquals("aa4fe1bd", material.serverToClientNonce.hex())
    }

    @Test
    fun encryptsBothDirectionsAndRejectsReplay() {
        val client = SecureChannel.create(secret(), challenge(), SecureRole.CLIENT)
        val server = SecureChannel.create(secret(), challenge(), SecureRole.SERVER)
        val ping = "한글 😀".toByteArray(StandardCharsets.UTF_8)

        val wrapped = client.encrypt(ping)
        val received = server.decrypt(wrapped.counter, wrapped.ciphertext)
        assertEquals("한글 😀", String(received, StandardCharsets.UTF_8))
        assertThrows(ProtocolException::class.java) { server.decrypt(wrapped.counter, wrapped.ciphertext) }

        val pong = server.encrypt("pong".toByteArray(StandardCharsets.UTF_8))
        assertEquals("pong", String(client.decrypt(pong.counter, pong.ciphertext), StandardCharsets.UTF_8))
    }

    @Test
    fun rejectsCiphertextTampering() {
        val client = SecureChannel.create(secret(), challenge(), SecureRole.CLIENT)
        val server = SecureChannel.create(secret(), challenge(), SecureRole.SERVER)
        val wrapped = client.encrypt("ping".toByteArray(StandardCharsets.UTF_8))
        val tampered = wrapped.ciphertext.copyOf().apply { this[0] = (this[0].toInt() xor 1).toByte() }

        assertThrows(ProtocolException::class.java) { server.decrypt(wrapped.counter, tampered) }
    }

    private fun secret(): ByteArray = ByteArray(20) { it.toByte() }

    private fun challenge(): ByteArray = ByteArray(32) { (0xa0 + it).toByte() }

    private fun ByteArray.hex(): String = joinToString("") { byte -> "%02x".format(byte.toInt() and 0xff) }
}
