package com.galaxytty.helper.notification

class NotificationAnalyzer(
    private val samsungMessagesPackage: String = SAMSUNG_MESSAGES_PACKAGE,
) {
    fun analyze(snapshot: NotificationSnapshot): NotificationObservation? {
        if (snapshot.packageName != samsungMessagesPackage) return null

        val replyCandidates = snapshot.actions
            .filter { action -> action.remoteInputs.isNotEmpty() }
            .map { action ->
                ReplyCandidate(
                    actionIndex = action.index,
                    semanticAction = action.semanticAction,
                    contextual = action.contextual,
                    authenticationRequired = action.authenticationRequired,
                    remoteInputs = action.remoteInputs,
                )
            }

        return NotificationObservation(
            keyFingerprint = snapshot.keyFingerprint,
            postedAtMillis = snapshot.postedAtMillis,
            category = snapshot.category,
            extrasKeys = snapshot.extrasKeys
                .asSequence()
                .map(::safeMetadataKey)
                .distinct()
                .sorted()
                .take(MAX_EXTRAS_KEYS)
                .toList(),
            actionCount = snapshot.actions.size,
            replyCandidates = replyCandidates,
            message = snapshot.message,
        )
    }

    private fun safeMetadataKey(value: String): String =
        value.take(MAX_METADATA_KEY_LENGTH)

    companion object {
        const val SAMSUNG_MESSAGES_PACKAGE = "com.samsung.android.messaging"
        private const val MAX_EXTRAS_KEYS = 32
        private const val MAX_METADATA_KEY_LENGTH = 64
    }
}
