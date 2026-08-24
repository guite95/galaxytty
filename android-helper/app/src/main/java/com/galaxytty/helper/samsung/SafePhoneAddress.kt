package com.galaxytty.helper.samsung

object SafePhoneAddress {
    fun normalize(value: String): String? {
        val candidate = value.trim()
        if (candidate.isEmpty()) return null
        if (candidate.any { character ->
                character !in '0'..'9' && character !in FORMATTING_CHARACTERS
            }
        ) {
            return null
        }
        if (candidate.count { character -> character == '+' } > 1 ||
            ('+' in candidate && !candidate.startsWith('+'))
        ) {
            return null
        }
        val digits = candidate.filter { character -> character in '0'..'9' }
        if (digits.length < MIN_DIGITS) return null
        return if (candidate.startsWith('+')) "+$digits" else digits
    }

    private const val MIN_DIGITS = 3
    private val FORMATTING_CHARACTERS = setOf('+', '-', '(', ')', '.', ' ')
}
