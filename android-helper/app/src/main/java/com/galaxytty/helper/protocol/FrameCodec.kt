package com.galaxytty.helper.protocol

import java.io.DataInputStream
import java.io.DataOutputStream
import java.io.EOFException
import java.io.InputStream
import java.io.OutputStream

object FrameCodec {
    const val MAX_FRAME_SIZE = 1024 * 1024

    fun read(input: InputStream): String {
        val stream = DataInputStream(input)
        val length = try {
            stream.readInt()
        } catch (error: EOFException) {
            throw error
        }
        if (length <= 0) throw ProtocolException("empty frame")
        if (length > MAX_FRAME_SIZE) throw ProtocolException("frame too large: $length")
        val payload = ByteArray(length)
        stream.readFully(payload)
        return payload.toString(Charsets.UTF_8)
    }

    fun write(output: OutputStream, json: String) {
        val payload = json.toByteArray(Charsets.UTF_8)
        if (payload.isEmpty()) throw ProtocolException("empty frame")
        if (payload.size > MAX_FRAME_SIZE) throw ProtocolException("frame too large: ${payload.size}")
        DataOutputStream(output).apply {
            writeInt(payload.size)
            write(payload)
            flush()
        }
    }
}

class ProtocolException(message: String, cause: Throwable? = null) : Exception(message, cause)
