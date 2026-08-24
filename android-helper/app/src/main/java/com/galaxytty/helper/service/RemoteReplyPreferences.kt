package com.galaxytty.helper.service

import android.content.Context

object RemoteReplyPreferences {
    private const val FILE_NAME = "remote_reply"
    private const val ALLOWED_KEY = "allowed_by_local_user"

    fun allowed(context: Context): Boolean =
        context.getSharedPreferences(FILE_NAME, Context.MODE_PRIVATE)
            .getBoolean(ALLOWED_KEY, false)

    fun setAllowed(context: Context, allowed: Boolean) {
        context.getSharedPreferences(FILE_NAME, Context.MODE_PRIVATE)
            .edit()
            .putBoolean(ALLOWED_KEY, allowed)
            .apply()
    }
}
