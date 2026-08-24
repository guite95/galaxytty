package com.galaxytty.helper.samsung

import org.junit.Assert.assertEquals
import org.junit.Assert.assertNull
import org.junit.Test

class SafePhoneAddressTest {
    @Test
    fun normalizesCommonPhoneFormatting() {
        assertEquals("01012345678", SafePhoneAddress.normalize("010-1234-5678"))
        assertEquals("+821012345678", SafePhoneAddress.normalize("+82 (10) 1234-5678"))
        assertEquals("114", SafePhoneAddress.normalize("114"))
    }

    @Test
    fun rejectsNamesUrisQueriesAndMalformedPlusSigns() {
        assertNull(SafePhoneAddress.normalize("person@example.com"))
        assertNull(SafePhoneAddress.normalize("01012345678?body=123"))
        assertNull(SafePhoneAddress.normalize("010+12345678"))
        assertNull(SafePhoneAddress.normalize("+82+1012345678"))
        assertNull(SafePhoneAddress.normalize("12"))
    }
}
