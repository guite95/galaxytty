package com.galaxytty.helper.security

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import java.security.KeyStore
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey

object PairingCredential {
    private const val KEYSTORE = "AndroidKeyStore"
    private const val KEY_ALIAS = "galaxytty-pairing-hmac-v1"
    private const val CREDENTIAL_DOMAIN = "galaxytty-pairing-credential-v1"
    private const val CREDENTIAL_BYTES = 20

    @Synchronized
    fun secret(): ByteArray = AuthProof.hmac(getOrCreateKey(), CREDENTIAL_DOMAIN)
        .copyOf(CREDENTIAL_BYTES)

    fun displayCode(): String = Base32Code.format(secret())

    private fun getOrCreateKey(): SecretKey {
        val keyStore = KeyStore.getInstance(KEYSTORE).apply { load(null) }
        (keyStore.getKey(KEY_ALIAS, null) as? SecretKey)?.let { return it }
        val generator = KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_HMAC_SHA256, KEYSTORE)
        generator.init(
            KeyGenParameterSpec.Builder(KEY_ALIAS, KeyProperties.PURPOSE_SIGN)
                .setDigests(KeyProperties.DIGEST_SHA256)
                .build(),
        )
        return generator.generateKey()
    }
}
