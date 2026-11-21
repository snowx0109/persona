package persona.core.hub

import persona.core.contracts.JsonCodec
import persona.core.contracts.Machine
import persona.core.contracts.MachineLinkSwitch
import persona.core.contracts.api.AgentLauncher
import persona.core.contracts.api.ClaudeSetting
import persona.core.contracts.api.ClaudeDirsState
import persona.core.contracts.api.LauncherQuotaAnswer
import persona.core.contracts.api.HerdrState
import persona.core.contracts.api.MachineManagement
import persona.core.contracts.api.MachineMove
import persona.core.contracts.api.NO_TERMINAL_ENGINE
import persona.core.contracts.MachineBrowser
import persona.core.contracts.api.DoctorReport
import persona.core.contracts.api.UsageAccount
import persona.core.contracts.api.UsageWindows
import kotlinx.serialization.json.*
import okhttp3.HttpUrl
import okhttp3.MediaType.Companion.toMediaType
import okhttp3.RequestBody
import okhttp3.RequestBody.Companion.toRequestBody
import okhttp3.Response

/**
 * The machine page's and settings' calls on one machine. [request] is the pairing's connection: it waits for the
 * route, adds the token and turns refusals into [HubFailure] (the hub's own words in its message).
 */
class HttpMachineManagement internal constructor(
    private val request: suspend (String, String, RequestBody?, Long?) -> Response,
) : MachineManagement {
    constructor(connection: OkHttpHubConnection) : this({ path, method, body, timeout ->
        connection.request(path, method, body, readTimeoutSeconds = timeout)
    })

    override suspend fun launchers(machineId: String): List<AgentLauncher> =
        (call(api("machines", machineId, "launchers"), "GET")["launchers"] as? JsonArray).orEmpty()
            .map { JsonCodec.decodeFromJsonElement<AgentLauncher>(it) }

    override suspend fun launcherQuota(machineId: String, name: String, refresh: Boolean): LauncherQuotaAnswer {
        require(name.isNotBlank())
        val path = HttpUrl.Builder().scheme("https").host("example.com").addPathSegments("api/v1/launchers").addPathSegment(name)
            .addPathSegment("quota").addQueryParameter("machine", machineId)
            .apply { if (refresh) addQueryParameter("refresh", "1") }.build().let { "${it.encodedPath}?${it.encodedQuery}" }
        // The hub gives the extension and the machine that has the key 45 s (a ~/.zshrc launcher is probed first).
        return JsonCodec.decodeFromJsonElement(call(path, "GET", timeout = 50))
    }

    override suspend fun reconnect(machineId: String) { call(api("machines", machineId, "reconnect"), "POST", buildJsonObject { }) }

    override suspend fun startLinkSwitch(machineId: String): MachineLinkSwitch {
        val answer = call(api("machines", machineId, "link-switch"), "POST", buildJsonObject { })
        val state = answer["link_switch"] as? JsonObject
            ?: throw HubFailure(HubError.Protocol, "hub answered the Link switch without link_switch", retryable = false)
        if ((state["available"] as? JsonPrimitive)?.booleanOrNull == null) {
            throw HubFailure(HubError.Protocol, "hub answered the Link switch without available", retryable = false)
        }
        return try { JsonCodec.decodeFromJsonElement(state) }
        catch (error: kotlinx.serialization.SerializationException) { throw HubFailure(HubError.Protocol, cause = error) }
    }

    override suspend fun deploy(machineId: String) { call(api("machines", machineId, "deploy"), "POST", buildJsonObject { }) }

    override suspend fun rename(machineId: String, name: String): String =
        (call(api("machines", machineId, "name"), "PUT", buildJsonObject { put("name", name) })["name"] as? JsonPrimitive)?.contentOrNull
            ?: throw HubFailure(HubError.Protocol, "hub answered the rename without a name", retryable = false)

    override suspend fun herdr(machineId: String): HerdrState =
        JsonCodec.decodeFromJsonElement(call(api("machines", machineId, "herdr"), "GET"))

    override suspend fun startHerdr(machineId: String): HerdrState =
        JsonCodec.decodeFromJsonElement(call(api("machines", machineId, "herdr", "start"), "POST", buildJsonObject { }, timeout = 50))

    override suspend fun setEngine(machineId: String, engine: String): Machine {
        require(engine.isNotBlank())
        // The empty host set says no terminal engine, which a single engine cannot (docs/protocol/protocol.md "Host sets").
        val body = if (engine == NO_TERMINAL_ENGINE) buildJsonObject { put("hosts", JsonArray(emptyList())) } else buildJsonObject { put("engine", engine) }
        val machine = JsonCodec.decodeFromJsonElement<Machine>(call(api("machines", machineId, "engine"), "PUT", body))
        // An answer for another machine is not this machine's state.
        if (machine.id != machineId) throw HubFailure(HubError.Protocol, "hub answered for another machine", retryable = false)
        return machine
    }

    override suspend fun botmux(machineId: String): Boolean? = boolSwitch(machineId, "botmux")

    override suspend fun setBotmux(machineId: String, on: Boolean): Boolean = setBoolSwitch(machineId, "botmux", on)

    override suspend fun claudeLiveReplies(machineId: String): Boolean? = boolSwitch(machineId, "claude-live-replies")

    override suspend fun setClaudeLiveReplies(machineId: String, on: Boolean): Boolean =
        setBoolSwitch(machineId, "claude-live-replies", on)

    private suspend fun boolSwitch(machineId: String, route: String): Boolean? = try {
        boolOn(call(api("machines", machineId, route), "GET"), route)
    } catch (failure: HubFailure) {
        // An older hub has no such route: the page hides the switch rather than showing an error.
        if (failure.status == 404) null else throw failure
    }

    private suspend fun setBoolSwitch(machineId: String, route: String, on: Boolean): Boolean =
        boolOn(call(api("machines", machineId, route), "PUT", buildJsonObject { put("on", on) }), route)

    private fun boolOn(body: JsonObject, what: String): Boolean = (body["on"] as? JsonPrimitive)?.booleanOrNull
        ?: throw HubFailure(HubError.Protocol, "hub answered $what without on", retryable = false)

    override suspend fun claudeSettings(machineId: String): List<ClaudeSetting> =
        settings(call(api("machines", machineId, "claude-settings"), "GET"))

    override suspend fun setClaudeSetting(machineId: String, key: String, value: JsonElement): List<ClaudeSetting> =
        setSetting(machineId, "claude-settings", key, value)

    override suspend fun codexSettings(machineId: String): List<ClaudeSetting> =
        settings(call(api("machines", machineId, "codex-settings"), "GET"))

    override suspend fun setCodexSetting(machineId: String, key: String, value: JsonElement): List<ClaudeSetting> =
        setSetting(machineId, "codex-settings", key, value)

    override suspend fun agySettings(machineId: String): List<ClaudeSetting> =
        settings(call(api("machines", machineId, "agy-settings"), "GET"))

    override suspend fun setAgySetting(machineId: String, key: String, value: JsonElement): List<ClaudeSetting> =
        setSetting(machineId, "agy-settings", key, value)

    private suspend fun setSetting(machineId: String, page: String, key: String, value: JsonElement): List<ClaudeSetting> {
        require(key.isNotBlank())
        return settings(call(api("machines", machineId, page), "PUT", buildJsonObject {
            put("key", key)
            // JSON null (not an absent field) takes the option back to the agent's default.
            put("value", value)
        }))
    }

    override suspend fun claudeDirs(machineId: String): ClaudeDirsState =
        JsonCodec.decodeFromJsonElement(call(api("machines", machineId, "claude-dirs"), "GET"))

    override suspend fun setBlockedClaudeDirs(machineId: String, blocked: List<String>): ClaudeDirsState =
        JsonCodec.decodeFromJsonElement(call(api("machines", machineId, "claude-dirs", "blocked"), "PUT", buildJsonObject {
            put("blocked", JsonArray(blocked.map(::JsonPrimitive)))
        }))

    // The hub gives the node 45 s to check and 60 s to rewrite and check again (server/internal/hub/node_doctor.go).
    override suspend fun doctor(machineId: String): DoctorReport =
        JsonCodec.decodeFromJsonElement(call(api("machines", machineId, "doctor"), "GET", timeout = 50))

    override suspend fun fixDoctor(machineId: String): DoctorReport =
        JsonCodec.decodeFromJsonElement(call(api("machines", machineId, "doctor", "fix"), "POST", buildJsonObject { }, timeout = 65))

    override suspend fun browser(machineId: String): MachineBrowser =
        JsonCodec.decodeFromJsonElement(call(api("machines", machineId, "browser"), "GET"))

    override suspend fun setBrowser(machineId: String, enabled: Boolean): MachineBrowser =
        JsonCodec.decodeFromJsonElement(call(api("machines", machineId, "browser"), "PUT", buildJsonObject { put("enabled", enabled) }))

    override suspend fun browserJob(machineId: String, action: String): MachineBrowser {
        require(action in setOf("install", "cancel", "open"))
        return JsonCodec.decodeFromJsonElement(call(api("machines", machineId, "browser", action), "POST", buildJsonObject { }))
    }

    override suspend fun usageWindows(): List<UsageAccount> =
        JsonCodec.decodeFromJsonElement<UsageWindows>(call(api("usage", "windows"), "GET")).accounts

    private fun settings(body: JsonObject): List<ClaudeSetting> =
        (body["settings"] as? JsonArray).orEmpty().map { JsonCodec.decodeFromJsonElement<ClaudeSetting>(it) }

    override suspend fun moveToManager(machineId: String, manager: String): MachineMove {
        require(machineId.isNotBlank() && manager.isNotBlank())
        val answer = call(api("managers", manager, "machines"), "POST", buildJsonObject { put("machine_id", machineId) })
        return try { JsonCodec.decodeFromJsonElement(MachineMove.serializer(), answer) }
        catch (error: kotlinx.serialization.SerializationException) { throw HubFailure(HubError.Protocol, cause = error) }
    }

    private suspend fun call(path: String, method: String, body: JsonObject? = null, timeout: Long? = null): JsonObject =
        request(path, method, body?.toString()?.toRequestBody("application/json; charset=utf-8".toMediaType()), timeout).use {
            if (!it.isSuccessful) throw HubFailure.from(it)
            val text = it.body?.string().orEmpty()
            if (text.isBlank()) buildJsonObject { } else JsonCodec.parseToJsonElement(text).jsonObject
        }

    private fun api(vararg segments: String) = HttpUrl.Builder().scheme("https").host("example.com")
        .addPathSegment("api").addPathSegment("v1").apply { segments.forEach(::addPathSegment) }.build().encodedPath
}
