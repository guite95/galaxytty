package com.galaxytty.helper.samsung

import java.io.File

class OneShotReplyExecutionPolicy(
    private val markerFile: File,
    private val enabled: Boolean,
    private val nowMillis: () -> Long = System::currentTimeMillis,
    private val validityMillis: Long = DEFAULT_VALIDITY_MILLIS,
) : ReplyExecutionPolicy {
    @Synchronized
    override fun allowsExecution(): Boolean {
        if (!markerFile.isFile) return false
        val armedAt = markerFile.lastModified()
        val consumed = markerFile.delete()
        if (!consumed || !enabled) return false
        val age = nowMillis() - armedAt
        return age in 0..validityMillis
    }

    @Synchronized
    fun isArmed(): Boolean {
        if (!enabled || !markerFile.isFile) return false
        val age = nowMillis() - markerFile.lastModified()
        if (age in 0..validityMillis) return true
        markerFile.delete()
        return false
    }

    @Synchronized
    fun clear() {
        markerFile.delete()
    }

    companion object {
        const val MARKER_FILE_NAME = "galaxytty-reply-once"
        const val DEFAULT_VALIDITY_MILLIS = 60_000L
    }
}
