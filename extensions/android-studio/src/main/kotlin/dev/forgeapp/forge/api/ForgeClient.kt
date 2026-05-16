package dev.forgeapp.forge.api

import com.google.gson.Gson
import com.google.gson.JsonSyntaxException
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.RequestBody.Companion.toRequestBody
import java.io.IOException
import java.util.concurrent.TimeUnit

data class ConnectRequest(val ide: String, val project_path: String)
data class ConnectResponse(val token: String, val context_id: String)
data class AskRequest(
    val context_id: String,
    val instruction: String,
    val selection: String? = null,
    val file_path: String? = null,
    val stream: Boolean = false
)
data class AskResponse(val response: String)
data class BuildError(
    val file: String,
    val message: String,
    val line: Int,
    val col: Int,
    val severity: String
)
data class ErrorsResponse(val errors: List<BuildError>)
data class HealthResponse(val status: String, val version: String, val ai_ready: Boolean = false)

class ForgeClient {

    companion object {
        val instance = ForgeClient()
    }

    private val gson = Gson()
    private val JSON = "application/json; charset=utf-8".toMediaType()

    private val client = OkHttpClient.Builder()
        .connectTimeout(5, TimeUnit.SECONDS)
        .readTimeout(60, TimeUnit.SECONDS)
        .writeTimeout(30, TimeUnit.SECONDS)
        .build()

    private val baseUrl = "http://localhost:7878"

    @Volatile private var token: String? = null
    @Volatile private var contextId: String? = null

    // -------------------------------------------------------------------------
    // Public API
    // -------------------------------------------------------------------------

    /**
     * POST /ide/v1/connect
     * Stores the returned token and context_id for subsequent calls.
     */
    fun connect(projectPath: String): Result<Unit> = runCatching {
        val body = gson.toJson(ConnectRequest(ide = "android-studio", project_path = projectPath))
            .toRequestBody(JSON)

        val request = Request.Builder()
            .url("$baseUrl/ide/v1/connect")
            .post(body)
            .build()

        val responseText = executeRequest(request)
        val parsed = gson.fromJson(responseText, ConnectResponse::class.java)
            ?: throw IOException("Empty connect response from daemon")

        token = parsed.token
        contextId = parsed.context_id
    }

    /**
     * POST /ide/v1/ask
     * Requires a prior successful [connect] call.
     */
    fun ask(
        instruction: String,
        selection: String? = null,
        filePath: String? = null
    ): Result<String> = runCatching {
        val ctx = contextId ?: throw IOException("Not connected to daemon. Call connect() first.")
        val tok = token   ?: throw IOException("Not connected to daemon. Call connect() first.")

        val payload = AskRequest(
            context_id  = ctx,
            instruction = instruction,
            selection   = selection,
            file_path   = filePath,
            stream      = false
        )
        val body = gson.toJson(payload).toRequestBody(JSON)

        val request = Request.Builder()
            .url("$baseUrl/ide/v1/ask")
            .addHeader("X-Forge-Token", tok)
            .post(body)
            .build()

        val responseText = executeRequest(request)
        val parsed = gson.fromJson(responseText, AskResponse::class.java)
            ?: throw IOException("Empty ask response from daemon")

        parsed.response
    }

    /**
     * GET /ide/v1/errors
     * Returns current build / lint errors from the daemon.
     */
    fun getErrors(): Result<List<BuildError>> = runCatching {
        val tok = token ?: throw IOException("Not connected to daemon. Call connect() first.")

        val request = Request.Builder()
            .url("$baseUrl/ide/v1/errors")
            .addHeader("X-Forge-Token", tok)
            .get()
            .build()

        val responseText = executeRequest(request)
        val parsed = gson.fromJson(responseText, ErrorsResponse::class.java)
            ?: throw IOException("Empty errors response from daemon")

        parsed.errors
    }

    /**
     * GET /health
     * Does not require authentication.
     */
    fun checkHealth(): Result<HealthResponse> = runCatching {
        val request = Request.Builder()
            .url("$baseUrl/health")
            .get()
            .build()

        val responseText = executeRequest(request)
        gson.fromJson(responseText, HealthResponse::class.java)
            ?: throw IOException("Empty health response from daemon")
    }

    fun isConnected(): Boolean = token != null

    fun disconnect() {
        token     = null
        contextId = null
    }

    // -------------------------------------------------------------------------
    // Internal helpers
    // -------------------------------------------------------------------------

    /**
     * Executes [request] and returns the response body as a String.
     * Throws [IOException] on connection failure or non-2xx HTTP status.
     */
    private fun executeRequest(request: Request): String {
        val response = try {
            client.newCall(request).execute()
        } catch (e: IOException) {
            throw IOException(
                "Cannot reach Forge daemon at $baseUrl. " +
                "Make sure it is running with `forge start`.",
                e
            )
        }

        return response.use { resp ->
            val bodyText = resp.body?.string() ?: ""
            if (!resp.isSuccessful) {
                val detail = try {
                    val errObj = gson.fromJson(bodyText, Map::class.java)
                    errObj?.get("error")?.toString() ?: bodyText
                } catch (_: JsonSyntaxException) {
                    bodyText
                }
                throw IOException("Daemon returned HTTP ${resp.code}: $detail")
            }
            bodyText
        }
    }
}
