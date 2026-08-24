package com.galaxytty.helper.service

import android.content.Context

object BridgePreferences {
    private const val FILE_NAME = "bridge"
    private const val ENABLED_KEY = "enabled_by_user"

    fun enabled(context: Context): Boolean =
        context.getSharedPreferences(FILE_NAME, Context.MODE_PRIVATE)
            .getBoolean(ENABLED_KEY, false)

    fun setEnabled(context: Context, enabled: Boolean) {
        context.getSharedPreferences(FILE_NAME, Context.MODE_PRIVATE)
            .edit()
            .putBoolean(ENABLED_KEY, enabled)
            .apply()
    }
}
