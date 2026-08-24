package com.galaxytty.helper.security

internal object Base32Code {
    private const val ALPHABET = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"

    fun format(value: ByteArray): String {
        val encoded = encode(value)
        return encoded.chunked(4).joinToString("-")
    }

    private fun encode(value: ByteArray): String {
        val result = StringBuilder((value.size * 8 + 4) / 5)
        var buffer = 0
        var bits = 0
        value.forEach { byte ->
            buffer = (buffer shl 8) or (byte.toInt() and 0xff)
            bits += 8
            while (bits >= 5) {
                bits -= 5
                result.append(ALPHABET[(buffer shr bits) and 0x1f])
            }
        }
        if (bits > 0) result.append(ALPHABET[(buffer shl (5 - bits)) and 0x1f])
        return result.toString()
    }
}
