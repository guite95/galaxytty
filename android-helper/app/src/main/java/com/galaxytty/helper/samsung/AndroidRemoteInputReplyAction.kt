package com.galaxytty.helper.samsung

import android.app.PendingIntent
import android.app.RemoteInput
import android.content.Context
import android.content.Intent
import android.os.Build
import android.os.Bundle

class AndroidRemoteInputReplyAction(
    private val context: Context,
    private val pendingIntent: PendingIntent,
    private val remoteInputs: Array<RemoteInput>,
) : ReplyAction {
    override fun send(text: String) {
        val results = Bundle().apply {
            remoteInputs.forEach { remoteInput -> putCharSequence(remoteInput.resultKey, text) }
        }
        val intent = Intent()
        RemoteInput.addResultsToIntent(remoteInputs, intent, results)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.P) {
            RemoteInput.setResultsSource(intent, RemoteInput.SOURCE_FREE_FORM_INPUT)
        }
        pendingIntent.send(context, 0, intent)
    }
}
