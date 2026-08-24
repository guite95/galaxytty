package com.galaxytty.helper.protocol

import java.nio.ByteBuffer
import java.nio.charset.StandardCharsets
import java.util.Base64
import javax.crypto.Cipher
import javax.crypto.Mac
import javax.crypto.spec.GCMParameterSpec
import javax.crypto.spec.SecretKeySpec
import org.json.JSONObject

internal enum class SecureRole {
    CLIENT,
    SERVER,
}

internal data class SessionMaterial(
    val clientToServerKey: ByteArray,
    val serverToClientKey: ByteArray,
    val clientToServerNonce: ByteArray,
    val serverToClientNonce: ByteArray,
)

internal data class EncryptedPayload(
    val counter: Long,
    val ciphertext: ByteArray,
)

internal class SecureChannel private constructor(
    private val sendKey: ByteArray,
    private val receiveKey: ByteArray,
    private val sendPrefix: ByteArray,
    private val receivePrefix: ByteArray,
    private val sendDirection: String,
    private val receiveDirection: String,
) {
    private var sendCounter = 0L
    private var receiveCounter = 0L

    fun wrap(envelope: ProtocolEnvelope): ProtocolEnvelope {
        val encrypted = encrypt(envelope.encode().toByteArray(StandardCharsets.UTF_8))
        return ProtocolEnvelope.create(
            type = ProtocolTypes.SECURE,
            payload = JSONObject()
                .put("counter", encrypted.counter)
                .put("ciphertext", Base64.getUrlEncoder().withoutPadding().encodeToString(encrypted.ciphertext)),
        )
    }

    @Synchronized
    internal fun encrypt(plaintext: ByteArray): EncryptedPayload {
        if (sendCounter == Long.MAX_VALUE) throw ProtocolException("encrypted frame counter exhausted")
        sendCounter += 1
        if (plaintext.size > MAX_INNER_FRAME_BYTES) throw ProtocolException("encrypted frame too large")
        val ciphertext = crypt(
            mode = Cipher.ENCRYPT_MODE,
            key = sendKey,
            nonce = nonce(sendPrefix, sendCounter),
            additionalData = additionalData(sendDirection, sendCounter),
            value = plaintext,
        )
        return EncryptedPayload(sendCounter, ciphertext)
    }

    fun unwrap(outer: ProtocolEnvelope): ProtocolEnvelope {
        if (outer.type != ProtocolTypes.SECURE) throw ProtocolException("encrypted frame required")
        val counter = outer.payload.optLong("counter", 0)
        val ciphertext = try {
            Base64.getUrlDecoder().decode(outer.payload.optString("ciphertext"))
        } catch (_: IllegalArgumentException) {
            throw ProtocolException("encrypted frame authentication failed")
        }
        val plaintext = decrypt(counter, ciphertext)
        val envelope = try {
            ProtocolEnvelope.decode(String(plaintext, StandardCharsets.UTF_8))
        } catch (_: Exception) {
            throw ProtocolException("encrypted frame authentication failed")
        }
        if (envelope.type == ProtocolTypes.SECURE) throw ProtocolException("nested encrypted frame rejected")
        return envelope
    }

    @Synchronized
    internal fun decrypt(counter: Long, ciphertext: ByteArray): ByteArray {
        if (counter <= 0 || receiveCounter == Long.MAX_VALUE || counter != receiveCounter + 1) {
            throw ProtocolException("invalid encrypted frame counter")
        }
        val plaintext = try {
            crypt(
                mode = Cipher.DECRYPT_MODE,
                key = receiveKey,
                nonce = nonce(receivePrefix, counter),
                additionalData = additionalData(receiveDirection, counter),
                value = ciphertext,
            )
        } catch (_: Exception) {
            throw ProtocolException("encrypted frame authentication failed")
        }
        receiveCounter = counter
        return plaintext
    }

    companion object {
        const val MODE = "aes-256-gcm-hkdf-sha256"
        private const val DOMAIN = "galaxytty-secure-v1"
        private const val KEY_BYTES = 32
        private const val NONCE_PREFIX_BYTES = 4
        private const val CHALLENGE_BYTES = 32
        private const val GCM_TAG_BITS = 128
        private const val MAX_INNER_FRAME_BYTES = 1024 * 1024 * 3 / 4 - 1024

        fun create(secret: ByteArray, challenge: ByteArray, role: SecureRole): SecureChannel {
            val material = deriveSessionMaterial(secret, challenge)
            return when (role) {
                SecureRole.CLIENT -> SecureChannel(
                    material.clientToServerKey,
                    material.serverToClientKey,
                    material.clientToServerNonce,
                    material.serverToClientNonce,
                    "client-to-server",
                    "server-to-client",
                )
                SecureRole.SERVER -> SecureChannel(
                    material.serverToClientKey,
                    material.clientToServerKey,
                    material.serverToClientNonce,
                    material.clientToServerNonce,
                    "server-to-client",
                    "client-to-server",
                )
            }
        }

        internal fun deriveSessionMaterial(secret: ByteArray, challenge: ByteArray): SessionMaterial {
            require(secret.size >= 16) { "secure channel credential is too short" }
            require(challenge.size == CHALLENGE_BYTES) { "secure channel challenge must be 32 bytes" }
            val prk = hmac(challenge, secret)
            return SessionMaterial(
                clientToServerKey = hkdfExpand(prk, "$DOMAIN/client-to-server/key", KEY_BYTES),
                serverToClientKey = hkdfExpand(prk, "$DOMAIN/server-to-client/key", KEY_BYTES),
                clientToServerNonce = hkdfExpand(prk, "$DOMAIN/client-to-server/nonce", NONCE_PREFIX_BYTES),
                serverToClientNonce = hkdfExpand(prk, "$DOMAIN/server-to-client/nonce", NONCE_PREFIX_BYTES),
            )
        }

        private fun hkdfExpand(prk: ByteArray, info: String, length: Int): ByteArray {
            val result = ArrayList<Byte>(length)
            var previous = ByteArray(0)
            var counter = 1
            while (result.size < length) {
                val input = previous + info.toByteArray(StandardCharsets.UTF_8) + byteArrayOf(counter.toByte())
                previous = hmac(prk, input)
                previous.forEach { result.add(it) }
                counter += 1
            }
            return result.take(length).toByteArray()
        }

        private fun hmac(key: ByteArray, value: ByteArray): ByteArray {
            val mac = Mac.getInstance("HmacSHA256")
            mac.init(SecretKeySpec(key, "HmacSHA256"))
            return mac.doFinal(value)
        }

        private fun crypt(
            mode: Int,
            key: ByteArray,
            nonce: ByteArray,
            additionalData: ByteArray,
            value: ByteArray,
        ): ByteArray {
            val cipher = Cipher.getInstance("AES/GCM/NoPadding")
            cipher.init(mode, SecretKeySpec(key, "AES"), GCMParameterSpec(GCM_TAG_BITS, nonce))
            cipher.updateAAD(additionalData)
            return cipher.doFinal(value)
        }

        private fun nonce(prefix: ByteArray, counter: Long): ByteArray =
            ByteBuffer.allocate(NONCE_PREFIX_BYTES + Long.SIZE_BYTES)
                .put(prefix)
                .putLong(counter)
                .array()

        private fun additionalData(direction: String, counter: Long): ByteArray {
            val prefix = "$DOMAIN\u0000$direction\u0000".toByteArray(StandardCharsets.UTF_8)
            return ByteBuffer.allocate(prefix.size + Long.SIZE_BYTES)
                .put(prefix)
                .putLong(counter)
                .array()
        }
    }
}
