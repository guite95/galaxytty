package com.galaxytty.helper.protocol

import java.io.ByteArrayInputStream
import java.io.ByteArrayOutputStream
import java.io.DataOutputStream
import org.junit.Assert.assertEquals
import org.junit.Assert.assertThrows
import org.junit.Test

class FrameCodecTest {
    @Test
    fun roundTripPreservesUtf8() {
        val expected = """{"version":1,"type":"HELLO","payload":{"text":"한글 😀"}}"""
        val output = ByteArrayOutputStream()
        FrameCodec.write(output, expected)
        assertEquals(expected, FrameCodec.read(ByteArrayInputStream(output.toByteArray())))
    }

    @Test
    fun rejectsOversizedFrameBeforeAllocation() {
        val output = ByteArrayOutputStream()
        DataOutputStream(output).writeInt(FrameCodec.MAX_FRAME_SIZE + 1)
        assertThrows(ProtocolException::class.java) {
            FrameCodec.read(ByteArrayInputStream(output.toByteArray()))
        }
    }
}
