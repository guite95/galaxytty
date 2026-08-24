package com.galaxytty.helper.tcp

import android.content.Context
import android.os.Build
import android.util.Log
import com.galaxytty.helper.message.LiveMessageQuery
import com.galaxytty.helper.message.MessageRepository
import com.galaxytty.helper.message.StoredMessage
import com.galaxytty.helper.message.toStoredMessage
import com.galaxytty.helper.notification.NotificationObservation
import com.galaxytty.helper.protocol.FrameCodec
import com.galaxytty.helper.protocol.ProtocolEnvelope
import com.galaxytty.helper.protocol.ProtocolException
import com.galaxytty.helper.protocol.ProtocolTypes
import com.galaxytty.helper.protocol.SecureChannel
import com.galaxytty.helper.protocol.SecureRole
import com.galaxytty.helper.security.AuthProof
import com.galaxytty.helper.security.PairingCredential
import com.galaxytty.helper.security.PairingIdentity
import com.galaxytty.helper.security.SessionAuthenticator
import com.galaxytty.helper.samsung.ReplyActionRegistry
import com.galaxytty.helper.samsung.ReplyDispatchStatus
import java.io.Closeable
import java.io.EOFException
import java.net.ServerSocket
import java.net.Socket
import java.net.SocketException
import java.net.SocketTimeoutException
import java.util.concurrent.Executors
import java.util.concurrent.RejectedExecutionException
import java.util.concurrent.atomic.AtomicLong
import java.security.SecureRandom
import java.util.Base64
import org.json.JSONArray
import org.json.JSONObject

class GalaxyTcpServer(
    context: Context,
    private val messageStore: MessageRepository,
    private val replyActions: ReplyActionRegistry,
    private val smsHistoryAvailable: () -> Boolean = { false },
) : Closeable {
    private val appContext = context.applicationContext
    private val executor = Executors.newCachedThreadPool { runnable ->
        Thread(runnable, "GalaxyTTY-TCP").apply { isDaemon = true }
    }
    private val eventExecutor = Executors.newSingleThreadExecutor { runnable ->
        Thread(runnable, "GalaxyTTY-TCP-Events").apply { isDaemon = true }
    }
    private val advertiser = NsdAdvertiser(appContext)
    private val networkMonitor = WifiNetworkMonitor(appContext) { network ->
        val listeningPort = synchronized(this) { serverSocket?.localPort } ?: return@WifiNetworkMonitor
        if (network == null) {
            advertiser.stop()
        } else {
            advertiser.start(listeningPort, network)
        }
    }
    private val sequence = AtomicLong(0)
    private var serverSocket: ServerSocket? = null
    private var activeSession: ClientSession? = null

    @Synchronized
    fun start(): Int {
        serverSocket?.let { return it.localPort }
        val socket = ServerSocket(0)
        serverSocket = socket
        networkMonitor.start()
        executor.execute { acceptLoop(socket) }
        Log.i(TAG, "read-only PoC server started port=${socket.localPort}; waiting for local Wi-Fi NSD")
        return socket.localPort
    }

    @Synchronized
    fun port(): Int? = serverSocket?.localPort

    fun publish(observation: NotificationObservation, contentIncluded: Boolean) {
        val replyShapes = JSONArray().apply {
            observation.replyCandidates.forEach { candidate ->
                put(
                    JSONObject()
                        .put("actionIndex", candidate.actionIndex)
                        .put("remoteInputCount", candidate.remoteInputs.size)
                        .put("semanticAction", candidate.semanticAction)
                        .put("authenticationRequired", candidate.authenticationRequired),
                )
            }
        }
        val payload = JSONObject()
            .put("notificationKey", observation.keyFingerprint)
            .put("postedAt", observation.postedAtMillis)
            .put("category", observation.category ?: JSONObject.NULL)
            .put("actionCount", observation.actionCount)
            .put("replyCandidates", replyShapes)
            .put("contentIncluded", contentIncluded)
        if (contentIncluded) {
            observation.message?.let { message ->
                payload.put("message", messageJson(message.toStoredMessage()))
            }
        }
        val envelope = ProtocolEnvelope.create(
            type = ProtocolTypes.MESSAGE_RECEIVED,
            sequence = sequence.incrementAndGet(),
            payload = payload,
        )
        try {
            eventExecutor.execute {
                synchronized(this) { activeSession }?.send(envelope)
            }
        } catch (_: RejectedExecutionException) {
            // The foreground bridge stopped while this notification was being observed.
        }
    }

    private fun acceptLoop(socket: ServerSocket) {
        while (!socket.isClosed) {
            try {
                val client = socket.accept()
                executor.execute { serve(client) }
            } catch (error: SocketException) {
                if (!socket.isClosed) Log.w(TAG, "accept failed", error)
                return
            } catch (error: Exception) {
                Log.w(TAG, "accept failed", error)
            }
        }
    }

    private fun serve(socket: Socket) {
        val session = ClientSession(socket)
        try {
            if (!session.authenticate()) return
            synchronized(this) {
                activeSession?.close()
                activeSession = session
            }
            session.runAuthenticated()
        } catch (_: SocketTimeoutException) {
            Log.i(TAG, "client handshake timeout")
        } catch (_: SocketException) {
            Log.i(TAG, "client disconnected during handshake")
        } catch (error: ProtocolException) {
            Log.w(TAG, "invalid client handshake: ${error.message}")
        } catch (error: Exception) {
            Log.w(TAG, "client handshake failed: ${error.javaClass.simpleName}")
        } finally {
            synchronized(this) {
                if (activeSession === session) activeSession = null
            }
            session.close()
        }
    }

    @Synchronized
    override fun close() {
        activeSession?.close()
        activeSession = null
        serverSocket?.close()
        serverSocket = null
        networkMonitor.close()
        advertiser.stop()
        eventExecutor.shutdownNow()
        executor.shutdownNow()
        Log.i(TAG, "server stopped")
    }

    private inner class ClientSession(private val socket: Socket) : Closeable {
        private var secureChannel: SecureChannel? = null

        fun authenticate(): Boolean {
            socket.soTimeout = CLIENT_TIMEOUT_MILLIS
            val deviceId = PairingIdentity.deviceId(appContext)
            val challengeBytes = ByteArray(CHALLENGE_BYTES).also(SecureRandom()::nextBytes)
            val challenge = Base64.getUrlEncoder().withoutPadding().encodeToString(challengeBytes)
            sendPlain(
                ProtocolEnvelope.create(
                    type = ProtocolTypes.HELLO,
                    payload = JSONObject()
                        .put("deviceId", deviceId)
                        .put("deviceName", Build.MODEL)
                        .put(
                            "capabilities",
                            JSONArray(
                                listOf(
                                    "ping",
                                    "notification-content",
                                    "live-memory-history",
                                    if (smsHistoryAvailable()) {
                                        "sms-history"
                                    } else {
                                        "sms-history-permission-required"
                                    },
                                    "remote-input-observation",
                                    "remote-input-reply-gated",
                                ),
                            ),
                        )
                        .put("authenticated", false)
                        .put("readOnlyPoc", true)
                        .put(
                            "auth",
                            JSONObject()
                                .put("mode", SessionAuthenticator.MODE)
                                .put("challenge", challenge),
                        ),
                ),
            )
            val request = ProtocolEnvelope.decode(FrameCodec.read(socket.getInputStream()))
            if (request.type != ProtocolTypes.AUTH) {
                sendError(request.requestId, "AUTH_REQUIRED", "Authenticate before using the local protocol")
                return false
            }
            val clientId = request.payload.optString("clientId")
            val proof = request.payload.optString("proof")
            val secret = PairingCredential.secret()
            val valid = SessionAuthenticator.verify(
                secret,
                deviceId,
                clientId,
                challenge,
                proof,
            )
            if (!valid) {
                Thread.sleep(AUTH_FAILURE_DELAY_MILLIS)
                sendError(request.requestId, "AUTH_FAILED", "Pairing credential was not accepted")
                Log.w(TAG, "client authentication rejected")
                return false
            }
            sendPlain(
                ProtocolEnvelope.create(
                    type = ProtocolTypes.AUTH,
                    requestId = request.requestId,
                    payload = JSONObject()
                        .put("authenticated", true)
                        .put(
                            "serverProof",
                            Base64.getUrlEncoder().withoutPadding().encodeToString(
                                AuthProof.createServer(secret, deviceId, clientId, challenge),
                            ),
                        )
                        .put("secureMode", SecureChannel.MODE),
                ),
            )
            secureChannel = SecureChannel.create(secret, challengeBytes, SecureRole.SERVER)
            Log.i(TAG, "client authenticated; encrypted session active")
            return true
        }

        fun runAuthenticated() {
            try {
                while (!socket.isClosed) {
                    val outer = ProtocolEnvelope.decode(FrameCodec.read(socket.getInputStream()))
                    val request = secureChannel?.unwrap(outer)
                        ?: throw ProtocolException("encrypted session unavailable")
                    when (request.type) {
                        ProtocolTypes.PING -> send(
                            ProtocolEnvelope.create(
                                type = ProtocolTypes.PONG,
                                requestId = request.requestId,
                                payload = JSONObject().put("receivedAt", System.currentTimeMillis()),
                            ),
                        )
                        ProtocolTypes.GET_CONVERSATIONS -> send(
                            ProtocolEnvelope.create(
                                type = ProtocolTypes.CONVERSATIONS,
                                requestId = request.requestId,
                                payload = JSONObject().put(
                                    "items",
                                    JSONArray().apply {
                                        messageStore.conversations().forEach { conversation ->
                                            put(
                                                JSONObject()
                                                    .put("threadId", conversation.threadId)
                                                    .put("title", conversation.title)
                                                    .put(
                                                        "participants",
                                                        JSONArray().apply {
                                                            conversation.participants.forEach { participant ->
                                                                put(
                                                                    JSONObject()
                                                                        .put("id", participant.id)
                                                                        .put("displayName", participant.displayName)
                                                                        .put("phone", participant.phone),
                                                                )
                                                            }
                                                        },
                                                    )
                                                    .put("snippet", conversation.snippet)
                                                    .put("updatedAt", conversation.updatedAtMillis)
                                                    .put("unreadCount", conversation.unreadCount),
                                            )
                                        }
                                    },
                                ),
                            ),
                        )
                        ProtocolTypes.GET_MESSAGES -> {
                            val query = LiveMessageQuery(
                                threadId = request.payload.optLong("threadId", 0),
                                limit = request.payload.optInt("limit", LiveMessageQuery.DEFAULT_LIMIT)
                                    .takeIf { it > 0 }
                                    ?: LiveMessageQuery.DEFAULT_LIMIT,
                                beforeId = request.payload.optLong("beforeId", 0),
                                afterId = request.payload.optLong("afterId", 0),
                                latest = request.payload.optBoolean("latest", false),
                            )
                            send(
                                ProtocolEnvelope.create(
                                    type = ProtocolTypes.MESSAGES,
                                    requestId = request.requestId,
                                    payload = JSONObject().put(
                                        "items",
                                        JSONArray().apply {
                                            messageStore.messages(query).forEach { message -> put(messageJson(message)) }
                                        },
                                    ),
                                ),
                            )
                        }
                        ProtocolTypes.SEND_REPLY -> {
                            val threadId = request.payload.optLong("threadId", 0)
                            val result = replyActions.dispatch(
                                threadId = threadId,
                                text = request.payload.optString("text"),
                            )
                            val payload = when (result.status) {
                                ReplyDispatchStatus.ACCEPTED_UNVERIFIED -> JSONObject()
                                    .put("outcome", "accepted_unverified")
                                    .put("evidence", "remote_input_pending_intent_accepted")
                                    .put("threadId", threadId)
                                else -> JSONObject()
                                    .put("outcome", "failed")
                                    .put("error", result.error ?: "RemoteInput reply failed")
                            }
                            send(
                                ProtocolEnvelope.create(
                                    type = ProtocolTypes.SEND_RESULT,
                                    requestId = request.requestId,
                                    payload = payload,
                                ),
                            )
                        }
                        else -> sendError(
                            request.requestId,
                            "POC_READ_ONLY",
                            "Message sending is disabled in this read-only PoC",
                        )
                    }
                }
            } catch (_: SocketTimeoutException) {
                Log.i(TAG, "client heartbeat timeout")
            } catch (_: EOFException) {
                Log.i(TAG, "client disconnected")
            } catch (_: SocketException) {
                Log.i(TAG, "client disconnected")
            } catch (error: ProtocolException) {
                Log.w(TAG, "invalid client frame: ${error.message}")
            } catch (error: Exception) {
                Log.w(TAG, "client session failed", error)
            }
        }

        private fun sendError(requestId: String?, code: String, message: String) {
            send(
                ProtocolEnvelope.create(
                    type = ProtocolTypes.ERROR,
                    requestId = requestId,
                    payload = JSONObject()
                        .put("code", code)
                        .put("message", message),
                ),
            )
        }

        @Synchronized
        fun send(envelope: ProtocolEnvelope) {
            if (socket.isClosed) return
            try {
                val wireEnvelope = secureChannel?.wrap(envelope) ?: envelope
                FrameCodec.write(socket.getOutputStream(), wireEnvelope.encode())
            } catch (error: Exception) {
                Log.w(TAG, "client write failed", error)
                close()
            }
        }

        private fun sendPlain(envelope: ProtocolEnvelope) {
            FrameCodec.write(socket.getOutputStream(), envelope.encode())
        }

        override fun close() {
            try {
                socket.close()
            } catch (_: Exception) {
                // Already closed.
            }
        }
    }

    companion object {
        private const val TAG = "GalaxyTTY-TCP"
        private const val CLIENT_TIMEOUT_MILLIS = 45_000
        private const val CHALLENGE_BYTES = 32
        private const val AUTH_FAILURE_DELAY_MILLIS = 750L
    }

    private fun messageJson(message: StoredMessage): JSONObject =
        JSONObject()
            .put("id", message.id)
            .put("threadId", message.threadId)
            .put("address", message.address)
            .put("body", message.body)
            .put("postedAt", message.postedAtMillis)
            .put("direction", message.direction)
            .put("read", message.read)
            .put("messageType", message.messageType)
            .put("attachments", JSONArray())
}
