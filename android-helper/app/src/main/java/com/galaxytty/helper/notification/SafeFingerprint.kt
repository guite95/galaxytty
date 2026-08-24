package com.galaxytty.helper.notification

import java.security.MessageDigest

object SafeFingerprint {
    fun of(value: String): String {
        val digest = MessageDigest.getInstance("SHA-256")
            .digest(value.toByteArray(Charsets.UTF_8))
        return digest.take(6).joinToString(separator = "") { byte -> "%02x".format(byte) }
    }
}
