package com.galaxytty.helper.service

object BridgeStartupPolicy {
    private val restartActions = setOf(
        "android.intent.action.BOOT_COMPLETED",
        "android.intent.action.MY_PACKAGE_REPLACED",
    )

    fun shouldRestart(enabledByUser: Boolean, action: String?): Boolean =
        enabledByUser && action in restartActions
}
