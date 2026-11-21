package persona.core.contracts

import persona.core.contracts.api.*
import kotlinx.serialization.encodeToString
import kotlinx.serialization.json.JsonNull
import kotlinx.serialization.json.JsonPrimitive
import kotlinx.serialization.json.jsonPrimitive
import org.junit.Assert.*
import org.junit.Test

/** Machine management, async starts and model configuration on the wire (docs/protocol/protocol.md, docs/assistant/model-config.md). */
class MachineManagementContractTest {
    @Test fun missingEngineFieldsMeanHerdrAndNothingDetected() {
        val m = JsonCodec.decodeFromString<Machine>("""{"id":"m1","online":true}""")
        assertEquals("herdr", (m.engine).zhAny())
        assertTrue(m.engines.isEmpty())
        assertFalse(m.deploying)
        assertNull(m.deployError)
        val n = JsonCodec.decodeFromString<Machine>("""{"id":"m1","engine":null,"engines":null,"deploying":null}""")
        assertEquals("herdr", (n.engine).zhAny())
        assertTrue(n.engines.isEmpty())
        assertFalse(n.deploying)
    }

    @Test fun herdrStatusDistinguishesStoppedRunningAndUnknownNodes() {
        val stopped = JsonCodec.decodeFromString<HerdrState>("""{"autostart":false,"running":false,"known":true}""")
        assertEquals(HerdrState(false, false, true), stopped)
        assertEquals(stopped, JsonCodec.decodeFromString<HerdrState>(JsonCodec.encodeToString(stopped)))
        assertFalse(JsonCodec.decodeFromString<HerdrState>("""{"autostart":true,"running":false,"known":false}""").known)
        assertTrue(JsonCodec.decodeFromString<HerdrState>("""{"autostart":false,"running":true,"known":true}""").running)
    }

    @Test fun keepsUnknownEnginesAndTheirFieldsAsGiven() {
        val m = JsonCodec.decodeFromString<Machine>(
            """{"id":"m1","engine":"orca","engines":[
              {"id":"herdr","name":"herdr","available":true,"version":"0.9.3","extra":1},
              {"id":"orca","name":"Orca","available":false,"note":"这个版本的 Orca 和 Persona 不兼容"},
              {"id":"future","name":"Future"},
              {"id":"bare"}
            ],"surprise":{"a":1}}""",
        )
        assertEquals("orca", (m.engine).zhAny())
        assertEquals(listOf("herdr", "orca", "future", "bare"), (m.engines.map { it.id }).zhAny())
        assertEquals("0.9.3", (m.engines[0].version).zhAny())
        assertEquals("这个版本的 Orca 和 Persona 不兼容", (m.engines[1].note).zhAny())
        assertFalse(m.engines[2].available)
        assertEquals("", (m.engines[3].name).zhAny())
    }

    @Test fun deployProgressAndItsErrorDecode() {
        val m = JsonCodec.decodeFromString<Machine>("""{"id":"m1","node_mismatch":true,"deploying":true,"deploy_error":"ssh: timeout"}""")
        assertTrue(m.deploying)
        assertEquals("ssh: timeout", (m.deployError).zhAny())
        assertEquals(true, m.nodeMismatch)
    }

    @Test fun linkSwitchIsOptionalAndKeepsItsAuthoritativeFields() {
        assertNull(JsonCodec.decodeFromString<Machine>("""{"id":"m1"}""").linkSwitch)
        assertNull(JsonCodec.decodeFromString<Machine>("""{"id":"m1","link_switch":null}""").linkSwitch)
        val machine = JsonCodec.decodeFromString<Machine>(
            """{"id":"m1","link_switch":{"available":false,"reason":"in_progress","phase":"failed",
              "error":"link_timeout","rollback_unconfirmed":true,"at":"2026-11-19T16:00:05+08:00","future":1}}""",
        )
        val state = requireNotNull(machine.linkSwitch)
        assertFalse(state.available)
        assertEquals("in_progress", state.reason)
        assertEquals("failed", state.phase)
        assertEquals("link_timeout", state.error)
        assertTrue(state.rollbackUnconfirmed)
        assertEquals(java.time.Instant.parse("2026-11-19T08:00:05Z"), parseTime(state.at))
        assertEquals(state, JsonCodec.decodeFromString<MachineLinkSwitch>(JsonCodec.encodeToString(state)))
        // A malformed safety flag cannot fall back to "rollback confirmed" in either a direct or nested state.
        for (json in listOf(
            """{"available":true,"phase":"failed","rollback_unconfirmed":"true"}""",
            """{"id":"m1","link_switch":{"available":true,"phase":"failed","rollback_unconfirmed":"true"}}""",
        )) {
            val decoded = runCatching {
                if ("link_switch" in json) JsonCodec.decodeFromString<Machine>(json)
                else JsonCodec.decodeFromString<MachineLinkSwitch>(json)
            }
            assertTrue(json, decoded.exceptionOrNull() is kotlinx.serialization.SerializationException)
        }
    }

    @Test fun startingAndStartErrorDecode() {
        val s = JsonCodec.decodeFromString<Session>("""{"machine_id":"mac","session_id":"s1","status":"idle","starting":true}""")
        assertTrue(s.starting)
        assertNull(s.startError)
        val f = JsonCodec.decodeFromString<Session>(
            """{"machine_id":"mac","session_id":"s1","status":"offline","start_error":"claude did not become ready"}""")
        assertEquals("claude did not become ready", (f.startError).zhAny())
        assertFalse(f.starting)
    }

    @Test fun asyncStartsSendNameHintsAndOmitWhatIsUnset() {
        val request = AgentStartRequest("mac", "/code", "", async = true, nameHint = "看下首页")
        val body = JsonCodec.encodeToString(request)
        assertEquals("""{"machine_id":"mac","cwd":"/code","launcher":"","async":true,"name_hint":"看下首页"}""", (body).zhAny())
    }

    @Test fun claudeSettingsKnowTheirDefaultAndEffectiveValue() {
        val list = JsonCodec.decodeFromString<List<ClaudeSetting>>(
            """[{"key":"a","title":"总是深度思考","type":"bool","default":false,"value":null},
               {"key":"b","type":"enum","options":["low","high"],"default":"low","value":"high"},
               {"key":"c","type":"string","default":"中文"},
               {"key":"d","type":"string"}]""",
        )
        assertTrue(list[0].isDefault)
        assertFalse(list[0].boolValue)
        assertEquals("high", (list[1].stringValue).zhAny())
        assertFalse(list[1].isDefault)
        assertEquals("中文", (list[2].placeholder).zhAny())
        assertEquals("中文", (list[2].stringValue).zhAny())
        assertEquals("未设置", (list[3].placeholder).zhAny())
    }

    private val purpose = ModelPurpose(
        id = "lark.agent", name = "飞书 /agent", group = "对话", kind = "launcher", revision = 4,
        override = ModelBinding("strong", "main", "high"), effective = ModelBinding("strong", "main", "high"),
        fields = listOf("provider", "model", "effort"),
    )

    @Test fun emptyFieldsRestoreInheritanceAndUnsupportedFieldsStayAbsent() {
        val patch = ModelConfigurationDraft(provider = " other ").patch(purpose)
        assertEquals("4", (patch.getValue("revision").jsonPrimitive.content).zhAny())
        assertEquals("other", (patch.getValue("provider").jsonPrimitive.content).zhAny())
        assertEquals(JsonNull, patch["model"])
        assertEquals(JsonNull, patch["effort"])
        val modelOnly = ModelConfigurationDraft(model = "next").patch(purpose.copy(fields = listOf("model")))
        assertEquals(setOf("revision", "model"), modelOnly.keys)
        // Without a field list a purpose takes provider and model.
        assertEquals(setOf("revision", "provider", "model"), ModelConfigurationDraft().patch(purpose.copy(fields = null)).keys)
    }

    @Test fun restoreClearsRuntimeWhereThePurposeTakesIt() {
        val withRuntime = purpose.copy(fields = listOf("provider", "model", "effort", "runtime"))
        val restore = ModelConfigurationDraft.Restore.patch(withRuntime)
        assertEquals(setOf("revision", "provider", "model", "effort", "runtime"), restore.keys)
        assertEquals(JsonNull, restore["runtime"])
        // Without runtime in its fields nothing about it is sent; an ordinary edit leaves the runtime as it is.
        assertEquals(setOf("revision", "provider", "model", "effort"), ModelConfigurationDraft.Restore.patch(purpose).keys)
        assertFalse("runtime" in ModelConfigurationDraft(provider = "other").patch(withRuntime))
    }

    @Test fun choosingAnotherProviderClearsModelAndEffort() {
        val draft = ModelConfigurationDraft.from(purpose)
        assertEquals(ModelConfigurationDraft("strong", "main", "high"), draft)
        assertEquals(ModelConfigurationDraft(provider = "other"), draft.choosingProvider("other"))
        assertEquals(draft, draft.choosingProvider("strong"))
        assertEquals(ModelConfigurationDraft(), ModelConfigurationDraft.from(purpose.copy(override = ModelBinding())))
    }

    @Test fun candidatesUseHubCapabilitiesAndKeepReferencedHiddenProviders() {
        val providers = listOf(
            ModelProvider("strong", "常用", hidden = true), ModelProvider("other", "其他"),
            ModelProvider("hidden", "隐藏", hidden = true), ModelProvider("api", "API", kind = "api"),
        )
        assertEquals(listOf("strong", "other"), (purpose.providersFrom(providers).map { it.id }).zhAny())
        assertEquals(listOf("api"), (purpose.copy(providerIds = listOf("api")).providersFrom(providers).map { it.id }).zhAny())
        assertTrue(purpose.copy(providerIds = emptyList()).providersFrom(providers).isEmpty())
        assertEquals(listOf("常用", "main", "思考 high"), (purpose.summaryParts(providers)).zhAny())
        assertEquals(listOf("strong", "main", "思考 high"), (purpose.summaryParts()).zhAny())
        assertTrue(purpose.copy(effective = ModelBinding()).summaryParts(providers).isEmpty())
    }

    @Test fun catalogDecodesFuturePurposesAndKeepsTheRest() {
        val catalog = JsonCodec.decodeFromString<ModelCatalog>(
            """{"purposes":[{"id":"plugin.voice","kind":"plugin","effective":{"provider":"plugin.voice","model":"voice-next"},
               "override":{},"revision":2,"fields":["model"],"future":true}],
               "providers":[{"id":"launcher.claude","name":"claude","has_key":false,"kind":"launcher"}]}""",
        )
        val voice = catalog.purposes.single()
        assertEquals("plugin", (voice.kind).zhAny())
        assertEquals("voice-next", (voice.effective.model).zhAny())
        assertEquals(2L, voice.revision)
        assertTrue(voice.supports("model"))
        assertFalse(voice.supports("provider"))
        assertEquals("claude", (catalog.providers.single().displayName).zhAny())
        val detail = JsonCodec.decodeFromString<ModelPurposeDetail>("""{"purpose":{"id":"agent.new","effective":{}}}""")
        assertEquals("agent.new", (detail.purpose.displayName).zhAny())
        assertTrue(detail.providers.isEmpty())
    }
}
