package com.galaxytty.helper.security

import android.content.Context
import java.security.SecureRandom

object PairingIdentity {
    private const val FILE_NAME = "pairing_identity"
    private const val DEVICE_ID_KEY = "device_id"

    @Synchronized
    fun deviceId(context: Context): String {
        val preferences = context.getSharedPreferences(FILE_NAME, Context.MODE_PRIVATE)
        preferences.getString(DEVICE_ID_KEY, null)?.takeIf { it.length == 32 }?.let { return it }
        val random = ByteArray(16).also(SecureRandom()::nextBytes)
        val identifier = random.joinToString("") { byte -> "%02x".format(byte.toInt() and 0xff) }
        if (!preferences.edit().putString(DEVICE_ID_KEY, identifier).commit()) {
            throw IllegalStateException("could not persist pairing identity")
        }
        return identifier
    }
}
