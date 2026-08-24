package com.galaxytty.helper

import android.Manifest
import android.app.Activity
import android.app.NotificationManager
import android.content.ComponentName
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Typeface
import android.os.Build
import android.os.Bundle
import android.provider.Settings
import android.view.ViewGroup
import android.widget.Button
import android.widget.LinearLayout
import android.widget.ScrollView
import android.widget.TextView
import com.galaxytty.helper.notification.GalaxyNotificationListenerService
import com.galaxytty.helper.notification.ObservationRepository
import com.galaxytty.helper.service.BridgeForegroundService
import com.galaxytty.helper.service.BridgePreferences
import com.galaxytty.helper.security.PairingCredential
import com.galaxytty.helper.tcp.BridgeRuntime
import java.text.DateFormat
import java.util.Date

class MainActivity : Activity() {
    private lateinit var statusView: TextView
    private var showPairingCode = false

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContentView(buildContent())
    }

    override fun onStart() {
        super.onStart()
        renderStatus()
    }

    override fun onRequestPermissionsResult(
        requestCode: Int,
        permissions: Array<out String>,
        grantResults: IntArray,
    ) {
        super.onRequestPermissionsResult(requestCode, permissions, grantResults)
        if (requestCode == BRIDGE_NOTIFICATION_PERMISSION_REQUEST &&
            grantResults.firstOrNull() == PackageManager.PERMISSION_GRANTED
        ) {
            BridgeForegroundService.start(applicationContext)
        }
        renderStatus()
    }

    private fun buildContent(): ScrollView {
        val density = resources.displayMetrics.density
        val padding = (24 * density).toInt()
        val gap = (12 * density).toInt()
        val content = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(padding, padding, padding, padding)
        }

        content.addView(TextView(this).apply {
            text = getString(R.string.app_name)
            textSize = 24f
            setTypeface(typeface, Typeface.BOLD)
        })
        content.addView(TextView(this).apply {
            text = getString(R.string.app_subtitle)
            textSize = 15f
            setPadding(0, gap / 2, 0, gap)
        })
        content.addView(Button(this).apply {
            text = getString(R.string.start_bridge)
            setOnClickListener { startBackgroundBridge() }
        })
        content.addView(Button(this).apply {
            text = getString(R.string.stop_bridge)
            setOnClickListener {
                BridgeForegroundService.stop(applicationContext)
                statusView.postDelayed(::renderStatus, STATUS_REFRESH_DELAY_MILLIS)
            }
        })
        content.addView(Button(this).apply {
            text = getString(R.string.open_notification_access)
            setOnClickListener {
                startActivity(Intent(Settings.ACTION_NOTIFICATION_LISTENER_SETTINGS))
            }
        })
        content.addView(Button(this).apply {
            text = getString(R.string.allow_sms_history)
            setOnClickListener {
                requestPermissions(arrayOf(Manifest.permission.READ_SMS), SMS_HISTORY_PERMISSION_REQUEST)
            }
        })
        content.addView(Button(this).apply {
            text = getString(R.string.refresh_observations)
            setOnClickListener { renderStatus() }
        })
        content.addView(Button(this).apply {
            text = getString(R.string.toggle_pairing_code)
            setOnClickListener {
                showPairingCode = !showPairingCode
                renderStatus()
            }
        })
        statusView = TextView(this).apply {
            typeface = Typeface.MONOSPACE
            textSize = 13f
            setTextIsSelectable(true)
            setPadding(0, gap, 0, 0)
        }
        content.addView(
            statusView,
            LinearLayout.LayoutParams(
                ViewGroup.LayoutParams.MATCH_PARENT,
                ViewGroup.LayoutParams.WRAP_CONTENT,
            ),
        )
        return ScrollView(this).apply { addView(content) }
    }

    private fun renderStatus() {
        val observations = ObservationRepository.snapshot()
        val access = if (hasNotificationAccess()) "granted" else "required"
        val bridge = if (BridgeForegroundService.running) "running" else "stopped"
        val restart = if (BridgePreferences.enabled(this)) "enabled" else "disabled"
        val bridgeNotification = if (hasBridgeNotificationPermission()) "granted" else "required"
        val smsHistory = if (hasSmsHistoryPermission()) "granted" else "required"
        statusView.text = buildString {
            appendLine("Notification access: $access")
            appendLine("Bridge notification permission: $bridgeNotification")
            appendLine("SMS history permission: $smsHistory")
            appendLine("Background bridge: $bridge")
            appendLine("Start after reboot/update: $restart")
            appendLine("TCP authentication: required")
            appendLine("TCP session encryption: AES-256-GCM")
            if (showPairingCode) {
                val pairingCode = try {
                    PairingCredential.displayCode()
                } catch (_: RuntimeException) {
                    "unavailable"
                }
                appendLine("Pairing code: $pairingCode")
                appendLine("Keep this code private; GalaxyTTY never logs it.")
            } else {
                appendLine("Pairing code: hidden")
            }
            appendLine("Samsung notifications observed: ${observations.size}")
            appendLine("Real reply execution: disabled by default")
            appendLine("Debug one-shot reply: ${if (BridgeRuntime.replyTestArmed()) "armed" else "not armed"}")
            appendLine("Local TCP port: ${BridgeRuntime.port() ?: "stopped"}")
            appendLine("TCP commands enabled: authenticated read sync and encrypted events")
            observations.takeLast(10).forEachIndexed { index, observation ->
                appendLine()
                appendLine("Observation ${index + 1}")
                appendLine("  key: ${observation.keyFingerprint}")
                appendLine("  time: ${DateFormat.getDateTimeInstance().format(Date(observation.postedAtMillis))}")
                appendLine("  category: ${observation.category ?: "none"}")
                appendLine("  extras keys: ${observation.extrasKeys.joinToString()}")
                appendLine("  actions: ${observation.actionCount}")
                appendLine("  RemoteInput reply candidates: ${observation.replyCandidates.size}")
                observation.replyCandidates.forEach { candidate ->
                    appendLine("    action ${candidate.actionIndex}: inputs=${candidate.remoteInputs.size}, semantic=${candidate.semanticAction}, auth=${candidate.authenticationRequired}")
                }
            }
        }
    }

    private fun startBackgroundBridge() {
        if (!hasBridgeNotificationPermission()) {
            requestPermissions(
                arrayOf(Manifest.permission.POST_NOTIFICATIONS),
                BRIDGE_NOTIFICATION_PERMISSION_REQUEST,
            )
            return
        }
        BridgeForegroundService.start(applicationContext)
        statusView.postDelayed(::renderStatus, STATUS_REFRESH_DELAY_MILLIS)
    }

    private fun hasBridgeNotificationPermission(): Boolean =
        Build.VERSION.SDK_INT < Build.VERSION_CODES.TIRAMISU ||
            checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) == PackageManager.PERMISSION_GRANTED

    private fun hasSmsHistoryPermission(): Boolean =
        checkSelfPermission(Manifest.permission.READ_SMS) == PackageManager.PERMISSION_GRANTED

    private fun hasNotificationAccess(): Boolean {
        val component = ComponentName(this, GalaxyNotificationListenerService::class.java)
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O_MR1) {
            return getSystemService(NotificationManager::class.java)
                .isNotificationListenerAccessGranted(component)
        }
        @Suppress("DEPRECATION")
        val enabled = Settings.Secure.getString(contentResolver, "enabled_notification_listeners").orEmpty()
        return enabled.split(':').any { value -> ComponentName.unflattenFromString(value) == component }
    }

    companion object {
        private const val BRIDGE_NOTIFICATION_PERMISSION_REQUEST = 100
        private const val SMS_HISTORY_PERMISSION_REQUEST = 101
        private const val STATUS_REFRESH_DELAY_MILLIS = 250L
    }
}
