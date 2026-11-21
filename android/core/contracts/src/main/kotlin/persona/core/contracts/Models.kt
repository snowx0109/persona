@file:OptIn(kotlinx.serialization.ExperimentalSerializationApi::class)

package persona.core.contracts

import persona.core.contracts.CoreText.Key
import persona.core.contracts.CoreText
import kotlinx.serialization.SerialName
import kotlinx.serialization.Serializable
import kotlinx.serialization.KeepGeneratedSerializer
import kotlinx.serialization.json.Json
import kotlinx.serialization.json.JsonElement
import kotlinx.serialization.json.JsonArray
import kotlinx.serialization.json.JsonObject
import kotlinx.serialization.json.JsonPrimitive
import java.time.Instant
import java.time.OffsetDateTime

// Wire types mirror docs/protocol/protocol.md and tolerate optional enhancements according to D5.

/** How long past its reset a usage-limit wait still counts (the hub's limitResumeGrace). */
internal const val USAGE_LIMIT_GRACE_MS = 10 * 60 * 1000L

/**
 * The time half of a usage-limit line (iOS `usageLimitWhen`): "Resets <time>", "Resumes <time>", or "Resumes after
 * the reset". The assistant's rows and card format it; the shared [Session.interrupted] flag does not.
 */
sealed class UsageLimitWhen {
    /** "Resets <time>": nothing continues the turn on its own. */
    data class Resets(val at: Instant) : UsageLimitWhen()
    /** "Resumes <time>": the Agent or the hub continues it. */
    data class Resumes(val at: Instant) : UsageLimitWhen()
    /** No reset time, and something continues it after the reset. */
    data object AfterReset : UsageLimitWhen()
}

val JsonCodec = Json {
    ignoreUnknownKeys = true
    explicitNulls = false
    coerceInputValues = true
    encodeDefaults = false
}

/**
 * RFC3339 from the hub; D5 treats dates before 2000, including Go zero time, as unset. A hub running in a local zone
 * sends offsets (`+08:00`), and [Instant.parse] before Android 14 accepts only `Z`, so everything goes through
 * [OffsetDateTime], which takes both: trying [Instant.parse] first threw on every offset, and sorting the inbox by time
 * on the main thread took most of every frame on Android 13 and older (#932).
 */
fun parseTime(s: String?): Instant? {
    if (s.isNullOrEmpty()) return null
    val instant = runCatching { OffsetDateTime.parse(s).toInstant() }.recoverCatching { Instant.parse(s) }.getOrNull()
    return instant?.takeUnless { it.isBefore(EarliestTime) }
}

private val EarliestTime: Instant = Instant.parse("2000-01-01T00:00:00Z")

@KeepGeneratedSerializer
@Serializable(with = MachineSerializer::class)
data class Machine(
    @SerialName("id") val id: String,
    @SerialName("name") val name: String = "",
    @SerialName("kind") val kind: String = "",
    @SerialName("icon") val icon: String? = null,
    @SerialName("ssh_host") val sshHost: String? = null,
    @SerialName("online") val online: Boolean = false,
    @SerialName("error") val error: String? = null,
    @SerialName("hostname") val hostname: String? = null,
    @SerialName("os") val os: String? = null,
    @SerialName("herdr_version") val herdrVersion: String? = null,
    @SerialName("claude_version") val claudeVersion: String? = null,
    @WireTime @SerialName("last_seen") val lastSeen: String? = null,
    val arch: String? = null,
    val home: String? = null,
    @SerialName("node_version") val nodeVersion: String? = null,
    val system: MachineSystem? = null,
    @SerialName("node_mismatch") val nodeMismatch: Boolean? = null,
    /** The terminal engine chosen for this machine (docs/integrations/engines.md); hubs send it always, herdr when never chosen. */
    @SerialName("engine") val engine: String = "herdr",
    /**
     * The terminal hosts this machine enables side by side (#2576 / #3302); empty when none are enabled; null from an older
     * hub that only sent [engine]. Resume in Terminal (#5565) reads the empty set as "no terminal engine".
     */
    @SerialName("engine_hosts") val engineHosts: List<String>? = null,
    /** What the node found there; empty from older hubs and nodes, and while the machine is offline. */
    @SerialName("engines") val engines: List<DetectedEngine> = emptyList(),
    /** The hub is installing its own personad build there ("Redeploy"). */
    @SerialName("deploying") val deploying: Boolean = false,
    /** Why the last "Redeploy" did not work. */
    @SerialName("deploy_error") val deployError: String? = null,
    /** Whether the hub would take a "Redeploy" here (docs/operations/node-update.md); false with [deployReason]; null from older hubs. */
    @SerialName("can_deploy") val canDeploy: Boolean? = null,
    @SerialName("deploy_reason") val deployReason: String? = null,
    /** A redeploy's stage: waiting_idle, transferring, restarting or confirming; null from older hubs. */
    @SerialName("deploy_phase") val deployPhase: String? = null,
    /** ego lite on a Mac while the hub's browser plugin is on (docs/plans/plan-ego-browser.md); absent otherwise. */
    @SerialName("browser") val browser: MachineBrowser? = null,
    /** How Cua Driver is on this Mac (docs/extensions/computer-use.md), while the cua extension is on; null elsewhere. */
    @SerialName("computer") val computer: MachineComputer? = null,
    /** `pending` while it waits for the user to say it is theirs (feature machine_trust); absent: confirmed. */
    @SerialName("trust") val trust: String? = null,
    /** What kind of trouble [error] is: node_service_down | link_offline | never_connected | node_outdated. */
    @SerialName("error_code") val errorCode: String? = null,
    /** The manager it belongs to (docs/assistant/managers.md「Ownership」); null from an older hub: the default manager. */
    @SerialName("manager") val manager: String? = null,
    /** SSH to Persona Link eligibility or progress; absent on older hubs and when there is nothing to show. */
    @WireStrict @SerialName("link_switch") val linkSwitch: MachineLinkSwitch? = null,
    /** The default for new sessions among [engineHosts]. Older hubs describe it with [engine]. */
    @SerialName("engine_default") val engineDefault: String? = null,
) {
    val displayName: String get() = name.ifEmpty { id }
    val pending: Boolean get() = trust == "pending"
}

/** `Machine.link_switch`: whether this computer can switch from SSH, or where that switch stands. */
@KeepGeneratedSerializer
@Serializable(with = MachineLinkSwitchSerializer::class)
data class MachineLinkSwitch(
    @SerialName("available") val available: Boolean,
    @SerialName("reason") val reason: String? = null,
    /** checking | installing | connecting | rolling_back | succeeded | failed. */
    @SerialName("phase") val phase: String? = null,
    /** A stable reason or failure code, never presented as raw text. */
    @SerialName("error") val error: String? = null,
    /** True means the hub could not prove the computer returned to SSH. */
    @WireStrict @SerialName("rollback_unconfirmed") val rollbackUnconfirmed: Boolean = false,
    @WireTime @SerialName("at") val at: String? = null,
)

/** `GET /api/v1/machines/{id}/browser` (and the machine's `browser`): ego lite there and how far its setup is. */
@Serializable
data class MachineBrowser(
    @SerialName("enabled") val enabled: Boolean = false,
    @SerialName("installed") val installed: Boolean = false,
    @SerialName("version") val version: String? = null,
    /** Its first-run setup is done; [imported], that setup took another browser's data. */
    @SerialName("set_up") val setUp: Boolean = false,
    @SerialName("imported") val imported: Boolean = false,
    /** Chromium's usage statistics: on | off | unknown. */
    @SerialName("analytics") val analytics: String? = null,
    @SerialName("ready") val ready: Boolean = false,
    @SerialName("note") val note: String? = null,
    @SerialName("screen_sharing") val screenSharing: Boolean = false,
    @SerialName("job") val job: MachineJob? = null,
)

/** Installing or opening something on the machine (the page's buttons): the running one, or the last. */
@Serializable
data class MachineJob(
    /** install | open | grant */
    @SerialName("kind") val kind: String = "",
    /** running | done | failed | cancelled */
    @SerialName("state") val state: String = "",
    @SerialName("step") val step: String? = null,
    @SerialName("error") val error: String? = null,
) {
    val running: Boolean get() = state == "running"
}

/**
 * The hub's `proto.Computer`: whether the user turned operating this Mac on, and what its node found when last asked
 * (only for an enabled one). [accessibility] and [screenRecording] are granted | missing | unknown; [desktop] is
 * active | locked | away; [note] says in words what is missing.
 */
@Serializable
data class MachineComputer(
    val enabled: Boolean = false,
    val installed: Boolean = false,
    val version: String? = null,
    val daemon: Boolean = false,
    val accessibility: String? = null,
    @SerialName("screen_recording") val screenRecording: String? = null,
    val desktop: String? = null,
    @SerialName("screen_sharing") val screenSharing: Boolean = false,
    val ready: Boolean = false,
    val note: String? = null,
)

/** One engine the node looked for on its machine (docs/protocol/protocol.md "Engines"); [name] and [note] are shown as given. */
@KeepGeneratedSerializer
@Serializable(with = DetectedEngineSerializer::class)
data class DetectedEngine(
    @SerialName("id") val id: String,
    @SerialName("name") val name: String = "",
    @SerialName("available") val available: Boolean = false,
    @SerialName("version") val version: String? = null,
    @SerialName("note") val note: String? = null,
)

/** Unified agent state computed by the hub (docs/protocol/agent-states.md). */
enum class AgentState(val wire: String, val priority: Int) {
    NeedsYou("needs_you", 0), Unread("unread", 1), Working("working", 2), Idle("idle", 3), Unknown("unknown", 4),
    Offline("offline", 5);

    companion object {
        fun of(wire: String?, status: String?): AgentState =
            if (wire != null) entries.firstOrNull { it.wire == wire } ?: Unknown else when (status) {
                "blocked" -> NeedsYou
                "working" -> Working
                "done" -> Unread
                "idle" -> Idle
                "offline" -> Offline
                else -> Unknown
            }
    }
}

@KeepGeneratedSerializer
@Serializable(with = LoopInfoSerializer::class)
data class LoopInfo(
    @SerialName("kind") val kind: String = "",
    @SerialName("prompt") val prompt: String? = null,
    @SerialName("interval_seconds") val intervalSeconds: Int? = null,
    @WireTime @SerialName("next_at") val nextAt: String? = null,
    @SerialName("cron") val cron: String? = null,
    val reason: String? = null,
    val ticks: Int? = null,
)

/**
 * A goal the agent keeps working toward until it holds (docs/integrations/agent-goal.md): Claude Code's `/goal`, whose Stop hook
 * checks the condition each time it would stop, or a Codex thread goal.
 */
@KeepGeneratedSerializer
@Serializable(with = GoalInfoSerializer::class)
data class GoalInfo(
    val objective: String = "",
    /** active / paused / blocked / usage_limited / budget_limited / complete (Claude Code's are only active or complete). */
    val status: String = "",
    /** Claude Code's latest check of the condition: why it holds, or what is still missing. */
    val reason: String? = null,
    @SerialName("tokens_used") val tokensUsed: Long? = null,
    /** Codex: the tokens the goal may spend; absent or 0 when there is no budget. */
    @SerialName("token_budget") val tokenBudget: Long? = null,
    /** Time spent on it so far. */
    val seconds: Long? = null,
    @WireTime @SerialName("updated_at") val updatedAt: String? = null,
) {
    val isComplete: Boolean get() = status == "complete"
    /** Still being worked on; anything else has stopped (met, paused, out of budget…). */
    val isActive: Boolean get() = status == "active"
    /** The objective's first non-blank line, for the one-line bar. */
    val headline: String get() = objective.lineSequence().map(String::trim).firstOrNull(String::isNotEmpty) ?: objective.trim()
}

@KeepGeneratedSerializer
@Serializable(with = BackgroundTaskSerializer::class)
data class BackgroundTask(
    @SerialName("id") val id: String = "",
    @SerialName("kind") val kind: String = "",
    @SerialName("description") val description: String? = null,
    @SerialName("command") val command: String? = null,
    @SerialName("output_file") val outputFile: String? = null,
    @WireTime @SerialName("started_at") val startedAt: String? = null,
    /** When a Monitor stops by itself; absent when it does not say. */
    @WireTime @SerialName("expires_at") val expiresAt: String? = null,
    /** A workflow's run (`wf_…`): which of [Session.workflows] it is. */
    @SerialName("run_id") val runId: String? = null,
    /** Retained results have a terminal status, a stop time and, when observed, an exit code. */
    val status: String? = null,
    @WireTime @SerialName("ended_at") val endedAt: String? = null,
    @SerialName("exit_code") val exitCode: Int? = null,
) {
    /** A Workflow run: shown from [Session.workflows] where the hub sends them, never counted twice. */
    val isWorkflow: Boolean get() = kind == "workflow"
}

/**
 * What a session with a [Session.sourceKind] allows; a missing field is a refusal (docs/protocol/protocol.md "Codex sources
 * and capabilities"), except the optional ones, whose null keeps the app's own rule.
 */
@KeepGeneratedSerializer
@Serializable(with = SessionCapabilitiesSerializer::class)
data class SessionCapabilities(
    val read: Boolean = false,
    val send: Boolean = false,
    val interrupt: Boolean = false,
    val approve: Boolean = false,
    val terminal: Boolean = false,
    val attach: Boolean = false,
    /** Null on an older hub: the client keeps its agent-based rule instead of treating the field as a refusal. */
    val modes: Boolean? = null,
    /** Close the agent, terminal or not (a runtime's session, #6294); null from an older node: the app keeps its terminal rule. */
    val close: Boolean? = null,
    /** Rename it, terminal or not (a runtime's session, #6294); null from an older node: the app keeps its terminal rule. */
    val rename: Boolean? = null,
    /** Switch its model through `POST …/modes`; null from an older node: the app keeps its Claude-in-a-terminal rule. */
    val model: Boolean? = null,
    /** Rewind to before one of your messages; null from an older node: the app keeps its Claude-in-a-terminal rule. */
    val rewind: Boolean? = null,
    /** A rewind may put the files back too ("Code and conversation"); null from an older node: as its [rewind] rule says. */
    @SerialName("rewind_code") val rewindCode: Boolean? = null,
    /**
     * The permission modes it takes when its agent is not Claude Code (#8261, Paseo's Grok, OpenCode or Codex): `default`,
     * its own approval, and `bypassPermissions` where it has an Allow everything. With two the chat offers a switch
     * between them (`persona.core.domain.sort.listedModes`); null from an older node and wherever Claude Code's modes stand.
     */
    @SerialName("permission_modes") val permissionModes: List<String>? = null,
)

/**
 * A session's work state (docs/assistant/agent-work.md): working, blocked on a named precondition ([on]: user, agent, external,
 * transient), or done; [line] is what the lists show, [wakeAt] when the hub acts next on its own.
 */
@KeepGeneratedSerializer
@Serializable(with = WorkStateSerializer::class)
data class WorkState(
    val phase: String = "",
    @SerialName("just_finished") val justFinished: Boolean = false,
    val on: String? = null,
    val line: String? = null,
    val wait: String? = null,
    val then: String? = null,
    @WireTime @SerialName("wake_at") val wakeAt: String? = null,
    /** When a usage limit that blocked this turn resets, if the session's own time is absent. */
    @WireTime @SerialName("resets_at") val resetsAt: String? = null,
    val attempts: Int? = null,
    val max: Int? = null,
    @SerialName("interaction_id") val interactionId: String? = null,
    val agent: WorkAgent? = null,
    /** What the turn produced (a pull request, a commit, a document), as the hub read it from the final reply. */
    val artifacts: List<WorkArtifact>? = null,
    /** What the agent said of its own work as the turn ended (#3612); null without one and from an older hub. */
    val closeout: Closeout? = null,
)

/** One thing a turn produced: [kind] pr, commit, branch, doc, build or url (kept as sent), [ref] its link or name. */
@Serializable
data class WorkArtifact(val kind: String = "", val ref: String = "")

/**
 * An agent's closeout (docs/assistant/agent-work.md "Closeout", #3612): the record it ends a turn with. It is the agent's
 * own claim: [status] done, not_done or blocked (anything else is no closeout: [known]), [merged] and [deployed] "yes" or
 * "no" (null when unsaid or not applicable), and [checked], what the hub found when it looked itself. Session lists
 * leave out [did] and [verified].
 */
@Serializable
data class Closeout(
    val status: String = "",
    val waiting: String? = null,
    val did: List<String>? = null,
    val merged: String? = null,
    val deployed: String? = null,
    val verified: String? = null,
    val missing: String? = null,
    val checked: CloseoutChecked? = null,
) {
    /** A status this app knows; a closeout with any other is not shown. */
    val known: Boolean get() = status == "done" || status == "not_done" || status == "blocked"
}

/** What the hub found of a closeout's merged and deployed claims: "yes", "no", or null when it could not tell. */
@Serializable
data class CloseoutChecked(val merged: String? = null, val deployed: String? = null)

@KeepGeneratedSerializer
@Serializable(with = WorkAgentSerializer::class)
data class WorkAgent(
    @SerialName("machine_id") val machineId: String = "",
    @SerialName("session_id") val sessionId: String = "",
    val label: String? = null,
)

@KeepGeneratedSerializer
@Serializable(with = SessionTodoSerializer::class)
data class SessionTodo(
    val id: String = "",
    val title: String = "",
)

/**
 * What Paseo's bridge calls every agent but Claude Code and Codex (integrations/paseo/lib/map.js `appAgent`): its
 * engine's id, never the agent's name. Such a session names its provider by `agent_label` (Paseo's label for it,
 * "Grok") and `launcher` (the provider's id, "grok"); the app never names it after the engine (#8143).
 */
const val PaseoAgent = "paseo"

/**
 * The name of an agent the app does not know by its id ([agent]): what the hub calls it ([label], a runtime's own name,
 * "DSH"), else the id. Paseo's [PaseoAgent] is its engine, not the agent: there it is the provider's label, else its
 * launcher (the provider's id), and the engine's name only while neither is known (#8143).
 */
fun unknownAgentName(agent: String?, label: String?, launcher: String?): String {
    val said = label?.trim()?.takeIf(String::isNotEmpty)
    if (agent != PaseoAgent) return said ?: agent.orEmpty()
    return said?.takeUnless { it.equals(PaseoAgent, ignoreCase = true) } ?: launcher?.trim()?.takeIf(String::isNotEmpty) ?: said ?: agent
}

/** A permission mode change recorded by the session's manager (GET …/mode-changes, #10297). */
@Serializable
data class ModeChange(
    /** set, restart_done, restart_failed or restart_dropped. */
    val kind: String = "",
    val from: String? = null,
    val to: String? = null,
    /** For set: now, restart when it takes effect after this turn, or empty from an older node. */
    val applied: String? = null,
    /** Who asked: device, hub or extension. */
    @SerialName("actor_kind") val actorKind: String? = null,
    @SerialName("device_name") val deviceName: String? = null,
    /** Why a queued restart finished as it did; a code, not display text. */
    val reason: String? = null,
    @WireTime val at: String = "",
)

@KeepGeneratedSerializer
@Serializable(with = SessionSerializer::class)
data class Session(
    @SerialName("machine_id") val machineId: String = "",
    @SerialName("session_id") val sessionId: String,
    @SerialName("title") val title: String = "",
    @SerialName("cwd") val cwd: String = "",
    @WireEnum("idle", "working", "blocked", "done", "offline", "interrupted", "unknown")
    @SerialName("status") val status: String = "unknown",
    @SerialName("tab_id") val tabId: String? = null,
    @SerialName("workspace_id") val workspaceId: String? = null,
    @SerialName("agent_name") val agentName: String? = null,
    @SerialName("started_by_persona") val startedByPersona: Boolean = false,
    val effort: String? = null,
    @SerialName("space_kind") val spaceKind: String? = null,
    @SerialName("space_index") val spaceIndex: Int? = null,
    @SerialName("slot_id") val slotId: String? = null,
    @WireTime @SerialName("state_since") val stateSince: String? = null,
    val subagents: List<Subagent>? = null,
    /** The ports the agent's processes serve over HTTP (docs/operations/ports.md "Discovering ports"): a dev server it started. */
    val ports: List<ListeningPort>? = null,
    @SerialName("pane_id") val paneId: String? = null,
    @SerialName("agent") val agent: String? = null,
    /** What the hub calls an agent this app does not know (a runtime's own name, "DSH"; docs/integrations/runtimes.md). */
    @SerialName("agent_label") val agentLabelHint: String? = null,
    /**
     * The client the agent runs in (#6390): "desktop" (Claude's or Codex's desktop app) or "cli" (its command line);
     * absent when the node cannot tell (an SDK, a runtime, an older node). The command line's mark carries </>.
     */
    @SerialName("agent_surface") val agentSurface: String? = null,
    /**
     * Where a session comes from when it is not an agent in a terminal engine: codex_app_server, codex_history,
     * identity_conflict, runtime (docs/protocol/protocol.md "Codex sources and capabilities"). Absent on older sources, which can do everything;
     * a session with a source can only do what [capabilities] explicitly allows (core:domain SessionSource).
     */
    @SerialName("source_kind") val sourceKind: String? = null,
    /** The source's own thread id (Codex): what `codex resume` takes, never the hub's opaque session id. */
    @SerialName("source_thread_id") val sourceThreadId: String? = null,
    @SerialName("source_instance_id") val sourceInstanceId: String? = null,
    @SerialName("parent_thread_id") val parentThreadId: String? = null,
    @SerialName("parent_session_id") val parentSessionId: String? = null,
    @SerialName("root_session_id") val rootSessionId: String? = null,
    @SerialName("is_subagent") val isSubagent: Boolean = false,
    @SerialName("asking_parent_session_id") val askingParentSessionId: String? = null,
    /** The listed ancestor whose chat owns this child (sessions_hierarchy). */
    @SerialName("folded_under") val foldedUnder: String? = null,
    /** Why a row that is no child folds: [FoldedAs.WORKER] (a tab's worker, #5180) or [FoldedAs.SIDE] (#7944). */
    @SerialName("folded_as") val foldedAs: String? = null,
    /**
     * The nearest side session a folded row belongs to when it folds through one and is not that side itself (a side's
     * own Codex children, #7944): its card and child card are that side's chat's, not the topmost parent's.
     */
    @SerialName("folded_side") val foldedSide: String? = null,
    val children: SessionChildren? = null,
    /**
     * The session on the same machine this one was started from while staying a conversation of its own (a Claude
     * desktop app's side chat or /btw question, #5935). Whether it folds is the hub's [foldedAs], never this.
     */
    @SerialName("parent_agent_id") val parentAgentId: String? = null,
    @WireTime @SerialName("started_at") val startedAt: String? = null,
    /** Why the source only lets the phone read this session. */
    @SerialName("read_only_reason") val readOnlyReason: String? = null,
    val capabilities: SessionCapabilities? = null,
    /** A Codex app-server thread the user already joined live (POST …/attach). */
    val attached: Boolean = false,
    @SerialName("launcher") val launcher: String? = null,
    @SerialName("model") val model: String? = null,
    @SerialName("permission_mode") val permissionMode: String? = null,
    /**
     * The agent's process can take Allow Everything while it runs (its start flags put the mode in its cycle, read off the
     * process by the node; #1379, #3957). False while unknown and from older hubs: then Allow Everything is a start's choice.
     */
    @SerialName("bypass_available") val bypassAvailable: Boolean = false,
    /**
     * Allow Everything was picked for this terminal Claude Code while it cannot reach it with shift+tab: the node restarts
     * the session in it once its turn is over (#6476, docs/integrations/claude-integration.md "Full access by a restart").
     * Until then the session still runs in [permissionMode]. False from older hubs.
     */
    @SerialName("bypass_pending") val bypassPending: Boolean = false,
    @SerialName("assistant") val assistant: Boolean = false,
    /** An assistant conversation put away (POST /api/v1/assistant/conversations/{id}/archive): kept, not listed. */
    @SerialName("archived") val archived: Boolean = false,
    /** User-defined labels; null on an older hub and where the hub omits an empty set. */
    @SerialName("tags") val tags: List<String>? = null,
    /** The manager it belongs to (docs/assistant/managers.md「Ownership」); null from an older hub: the default manager. */
    @SerialName("manager") val manager: String? = null,
    /**
     * The assistant space it runs in (docs/assistant/health.md "Which space"): "health" for a health chat; absent for the
     * assistant's own chats and from older hubs.
     */
    @SerialName("space") val space: String? = null,
    /**
     * Who opened an assistant conversation (docs/protocol/protocol.md "Origin of assistant chats", #3264): user, channel, inbox, task, spawned,
     * lark_agent, extension, or one this app doesn't know yet; null is the user's (older hubs never say).
     */
    @SerialName("origin") val origin: String? = null,
    /** What it was opened for: the inbox item, the task, the conversation, the channel, the extension. */
    @SerialName("origin_ref") val originRef: String? = null,
    /**
     * The extension whose token opened it (`POST /api/v1/assistant/conversations`, docs/protocol/protocol.md "Origin of
     * assistant chats"); absent for everyone else and from older hubs. Its mark draws that extension's icon (#10358).
     */
    @SerialName("opened_by") val openedBy: String? = null,
    @SerialName("space_id") val spaceId: String? = null,
    @SerialName("space_name") val spaceName: String? = null,
    @SerialName("slot_name") val slotName: String? = null,
    @SerialName("slot_index") val slotIndex: Int? = null,
    /**
     * The pane's place in its tab (#2320; docs/protocol/agent-states.md "Several panes in one tab"): 0 is the top-left pane, the one
     * the others were split from (the tab's initiator). Absent when the host gives no layout.
     */
    @SerialName("pane_index") val paneIndex: Int? = null,
    /**
     * The main checkout of the repository [cwd] belongs to, when its host knows it (#2840: Paseo keeps its worktrees
     * under its own folder, whose paths name no repository). Group by repository goes by it (core:domain repoOf); older hubs never send it.
     */
    @SerialName("repo_root") val repoRoot: String? = null,
    /** The top of the git working tree cwd is in (repoRoot for the main checkout) and its branch, from the node (#1805). */
    @SerialName("worktree") val worktree: String? = null,
    @SerialName("branch") val branch: String? = null,
    @WireEnum("needs_you", "unread", "working", "idle", "offline", "unknown")
    @SerialName("state") val state: String? = AgentState.of(null, status).wire,
    @WireTime @SerialName("seen_at") val seenAt: String? = null,
    @SerialName("queued") val queued: List<String>? = null,
    /**
     * A Persona conversation's last this many of [queued] came from other conversations or the system and arrive when
     * the conversation is free (docs/protocol/protocol.md, `queued_relayed`): they are shown, never changed. Absent: none.
     */
    @SerialName("queued_relayed") val queuedRelayed: Int? = null,
    @SerialName("background") val background: List<BackgroundTask>? = null,
    /** A short-lived list of shell and Monitor outcomes. Summary rows and older hubs can omit it. */
    @SerialName("background_ended") val backgroundEnded: List<BackgroundTask>? = null,
    /**
     * Its Workflow runs of the last day, newest first (docs/protocol/agent-states.md "Background workflows and Monitor"): a running one is
     * in [background] too, as kind `workflow`. Older hubs never send it (null).
     */
    val workflows: List<Workflow>? = null,
    @SerialName("loop") val loop: LoopInfo? = null,
    /** The goal it works toward, when one is set (docs/integrations/agent-goal.md); older hubs never send it. */
    val goal: GoalInfo? = null,
    /** The agent's counted task list, as reported by the node. */
    val plan: PlanInfo? = null,
    @WireTime @SerialName("updated_at") val updatedAt: String? = null,
    @SerialName("last_text") val lastText: String? = null,
    /** What its last reply asks the user, when its closing words put a question or a decision to them (#5096). */
    @SerialName("asks") val asks: String? = null,
    /**
     * It stopped on a question to the user nobody answered yet (docs/protocol/agent-states.md "Awaiting your reply", #5096): set by the hub
     * on a stopped session (Unread or Seen). Older hubs never send it.
     */
    @SerialName("awaits_reply") val awaitsReply: Boolean = false,
    /** Its latest step when that is a tool call no text came after ("running tests", "reading tasks.go"), and when it started. */
    @SerialName("activity") val activity: String? = null,
    @WireTime @SerialName("activity_at") val activityAt: String? = null,
    /** Its latest turn ended in a model API error (the hub sets it while it is stopped, #1516): its task is interrupted. */
    @SerialName("turn_error") val turnError: String? = null,
    /**
     * What the agent or the hub called [turnError] (docs/protocol/protocol.md): `usage_limit` is a subscription limit
     * until it resets, not a turn to continue as an interruption; `no_output` and `stopped` are the hub's own word on a
     * task Agent's turn with no output (#10187), [turnError] then being its sentence, not an API error. Older hubs never send it.
     */
    @SerialName("turn_error_kind") val turnErrorKind: String? = null,
    /** When a `usage_limit` resets (RFC3339). Absent when that time is not known. */
    @WireTime @SerialName("turn_error_resets_at") val turnErrorResetsAt: String? = null,
    /** The CLI goes on by itself once a usage limit resets. Absent or false: nothing of the CLI will. */
    @SerialName("turn_error_resumes") val turnErrorResumes: Boolean? = null,
    /**
     * What the hub recognized of [turnError] (docs/protocol/protocol.md): not_logged_in, auth_failed, model_route,
     * rate_limited or model_fallback_unavailable (the model Claude Code fell back to after a refusal can't be used, #2528:
     * "Continue" would only go to it again). Older hubs never send it.
     */
    @SerialName("turn_error_code") val turnErrorCode: String? = null,
    /** The refusal fallback still active in this session, as Claude Code recorded it (#2289, #2528). */
    @SerialName("model_fallback") val modelFallback: ModelRefusal? = null,
    /** "Back to the configured model" was asked and no newer reply has come yet (#2528). Not proof it runs. */
    @SerialName("model_restore") val modelRestore: ModelRestore? = null,
    /** Why it waits for the user, in its host's words (a botmux attention's reason); only while it needs you. */
    @SerialName("attention_reason") val attentionReason: String? = null,
    /**
     * Its host stopped the turn on an error it will not retry by itself (a botmux notice), until a message goes to it
     * (docs/protocol/protocol.md). The chat offers a retry over the composer; it is not a wait for the user.
     */
    @SerialName("failure_reason") val failureReason: String? = null,
    /** Where its host shows it (a botmux session's Feishu topic, docs/integrations/engines.md "botmux"); opened only as [externalLink]. */
    @SerialName("external_url") val externalUrl: String? = null,
    /** Where it stands in its work (docs/assistant/agent-work.md): set by the hub once it triaged a stopped turn. */
    val work: WorkState? = null,
    @SerialName("pending_interactions") val pendingInteractions: Int = 0,
    /** "Branch to new chat" works for it (a Claude Code conversation, node/fork.go); older nodes never say so. */
    @SerialName("can_fork") val canFork: Boolean = false,
    @SerialName("rev") val rev: Long? = null,
    @SerialName("suggestion") val suggestion: String? = null,
    /** The suggested message must be reviewed in the composer, never sent in one action; absent on older hubs. */
    @SerialName("suggestion_risky") val suggestionRisky: Boolean = false,
    /** Persona is still opening its tab and starting the agent (docs/protocol/protocol.md "New session"); sends wait for it. */
    @SerialName("starting") val starting: Boolean = false,
    /** Why starting it failed; the session is offline and sends get the same error. */
    @SerialName("start_error") val startError: String? = null,
    /** Codex is open in its pane and has no thread until its first message. */
    @SerialName("awaiting_first_message") val awaitingFirstMessage: Boolean = false,
    /**
     * The channels messages came into this conversation from, newest first (`wechat`, `feishu`; docs/extensions/channels.md
     * "Source marks"): the conversation list draws the latest's mark. Older hubs never send it.
     */
    @SerialName("channels") val channels: List<String> = emptyList(),
    val todo: SessionTodo? = null,
    /**
     * The last compaction the user asked for in an assistant conversation (POST …/compact, docs/assistant/assistant.md "Manual compaction"):
     * running, then done or failed. Kept until the next one; older hubs never send it.
     */
    val compaction: SessionCompaction? = null,
    /**
     * Works (or worked) for a quiet task (#1731): the main conversation opened it while "Notify main-chat tasks only in the main chat"
     * was on. No alerts or global badges for it; the main conversation tells the user. Older hubs never send it.
     */
    val quiet: Boolean = false,
    /**
     * The levels [effort] may be set to when they are not Claude Code's: a Codex thread the hub drives through its
     * app-server, where only the effort changes (node/codex_effort.go). Empty for every other session.
     */
    val efforts: List<String> = emptyList(),
    /**
     * Claude Code's ultracode as the node last saw it (docs/protocol/protocol.md "Claude Code ultracode"): null is unknown
     * (no evidence yet, an older node or hub, another agent), not off.
     */
    val ultracode: Boolean? = null,
    /** The node's reason the switch can't be used, was refused or still waits; null when there is none. */
    @SerialName("ultracode_note") val ultracodeNote: String? = null,
    /** A change of it waits for the agent's turn to end (or a draft / dialog in the terminal): a queue flag, not the value. */
    @SerialName("ultracode_pending") val ultracodePending: Boolean = false,
    /** It was on and a resume started it off (#582, docs/protocol/protocol.md "Claude Code ultracode"): the chat offers it back
     *  until it is on again or the notice is dismissed; the hub never turns it back on by itself. */
    @WireTime @SerialName("ultracode_lost_at") val ultracodeLostAt: String? = null,
    /** Present on bounded summary rows; absent on legacy complete rows. */
    val waiting: Boolean? = null,
    @SerialName("background_count") val backgroundCountHint: Int? = null,
    @SerialName("running_subagents") val runningSubagents: Int? = null,
    @SerialName("running_workflows") val runningWorkflowsCount: Int? = null,
    /** Independent shell/Monitor count, excluding workflows and subagents. */
    @SerialName("background_tasks_count") val backgroundTasksCount: Int? = null,
    /** Newest running workflow; bounded list metadata without its agent or phase arrays. */
    @SerialName("workflow_progress") val workflowProgress: Workflow? = null,
    @SerialName("background_working") val backgroundWorking: Boolean? = null,
    /**
     * Working only because of background work that only waits (`sleep`, a `--watch`, `tail -f`; the hub's
     * `background_waiting`, #6045): set only with `state` working. False from older hubs.
     */
    @SerialName("background_waiting") val backgroundWaiting: Boolean = false,
    /**
     * How Persona reaches a Codex in a terminal (docs/integrations/codex.md, "Sessions on the daemon"): `app_server` (joined on
     * its shared app-server) or `terminal` (its screen and session record); [codexRouteNote] says why it is the terminal.
     */
    @SerialName("codex_route") val codexRoute: String? = null,
    @SerialName("codex_route_note") val codexRouteNote: String? = null,
    /**
     * What the model saw in the agent's latest reply, and the model's window (docs/protocol/protocol.md, context usage): null
     * while the node can't tell (no reply yet, a window it does not know, an older node).
     */
    @SerialName("context_tokens") val contextTokens: Long? = null,
    @SerialName("context_window") val contextWindow: Long? = null,
    /** An assistant conversation set to run on the Agent SDK: `agentsdk` while it does; [runtimeFallback] why the node ran the CLI instead. */
    @SerialName("runtime") val runtime: String? = null,
    @SerialName("runtime_fallback") val runtimeFallback: String? = null,
    /**
     * What an assistant chat's next messages run on when it was switched to a model in an app (`PUT …/conversations/{id}/model`,
     * feature `assistant_model_pick`, docs/assistant/model-config.md "A conversation's own model"); null: it follows the
     * default. Summary rows carry it too.
     */
    @SerialName("model_pick") val modelPick: ModelPick? = null,
    /**
     * A running Claude Code Agent's switch to another launcher that the user's next message from the chat carries out
     * (feature `launcher_switch`, docs/protocol/protocol.md "Switching a running Agent to another provider"); null: none
     * waits. [launcher] stays the one it runs on until then. Summary rows leave it out.
     */
    @SerialName("pending_switch") val pendingSwitch: PendingSwitch? = null,
    /** Earlier stretches of this conversation, oldest first, after a handoff to the other engine. */
    @SerialName("engine_history") val engineHistory: List<EngineLeg> = emptyList(),
    /** The manager's current queue. A complete session row without the field has no held messages. */
    val undelivered: List<HeldAgentMessage>? = null,
) {
    val key: SessionKey get() = SessionKey(machineId, sessionId)
    /** A chat of the health space ([space]), which the assistant's own lists leave out. */
    val isHealth: Boolean get() = space == persona.core.contracts.api.AssistantSpace.HEALTH
    val agentState: AgentState get() = AgentState.of(state, status)
    /**
     * The list state of this session and its folded children; its own wire state stays intact. A side session's result
     * not looked at yet makes it unread (`sides_unread`, #7944: sides only; Codex children and workers do not), unless it
     * works itself.
     */
    val foldedState: AgentState get() = when {
        agentState == AgentState.NeedsYou || (children?.needsYou ?: 0) > 0 -> AgentState.NeedsYou
        (children?.working ?: 0) > 0 && agentState != AgentState.Unread &&
            !(agentState == AgentState.Idle && awaitsReply) -> AgentState.Working
        agentState != AgentState.Working && (children?.sidesUnread ?: 0) > 0 -> AgentState.Unread
        else -> agentState
    }
    /** Stopped on a model API error ([turnError]): cut off, not finished (#1516). */
    val interrupted: Boolean get() = !turnError.isNullOrBlank() && (agentState == AgentState.Unread || agentState == AgentState.Idle)

    /**
     * Stopped on a subscription usage limit whose reset is still ahead, or not known (#8237). A known reset more than
     * [USAGE_LIMIT_GRACE_MS] ago is over: the turn reads as interrupted again. [now] is epoch millis.
     */
    fun isUsageLimited(now: Long = System.currentTimeMillis()): Boolean {
        if (turnError.isNullOrBlank() || turnErrorKind != "usage_limit") return false
        if (agentState != AgentState.Unread && agentState != AgentState.Idle) return false
        val at = parseTime(turnErrorResetsAt ?: work?.resetsAt)?.toEpochMilli() ?: return true
        return now <= at + USAGE_LIMIT_GRACE_MS
    }

    /** The CLI said it goes on by itself, or the hub scheduled this limit's continuation. */
    fun usageLimitContinues(): Boolean {
        if (turnErrorResumes == true) return true
        val waiting = work ?: return false
        return waiting.phase == "blocked" && parseTime(waiting.resetsAt) != null && parseTime(waiting.wakeAt) != null
    }

    /**
     * Only the time half of a usage-limit line (iOS `usageLimitWhen`), for a row that already says "Usage Limit".
     * Null when this is not a usage-limit wait, or nothing says when. [now] is epoch millis: callers pass the
     * minute tick, so the line changes at the grace boundary.
     */
    fun usageLimitWhen(now: Long = System.currentTimeMillis()): UsageLimitWhen? {
        if (!isUsageLimited(now)) return null
        val goesOn = usageLimitContinues()
        val at = parseTime(turnErrorResetsAt ?: work?.resetsAt)
        return when {
            at == null && goesOn -> UsageLimitWhen.AfterReset
            at == null -> null
            goesOn -> UsageLimitWhen.Resumes(at)
            else -> UsageLimitWhen.Resets(at)
        }
    }
    val folderName: String get() = cwd.trimEnd('/').substringAfterLast('/')
    val displayTitle: String get() = if (title.isEmpty() || title == "Claude Code") folderName.ifEmpty { sessionId.take(8) } else title
    /** Codex in a terminal, or a thread from a Codex app-server / Codex history. */
    val isCodex: Boolean get() = agent == "codex" || sourceKind == "codex_app_server" || sourceKind == "codex_history"

    /** The launcher's catalog identity when a row omits an explicit launcher. */
    val effectiveLauncher: String get() = launcher?.takeIf(String::isNotEmpty) ?: when {
        spaceKind == "codex_app" -> "codex-app"
        isCodex -> "codex"
        else -> "claude"
    }

    /** Claude Code, or an older hub that does not name the agent: what Claude-only controls (modes, /model, /clear) need. */
    val isClaude: Boolean get() = !isCodex && (agent.isNullOrEmpty() || agent == "claude")
    /**
     * Who the user talks to: Claude, Codex, a compatible CLI, what the hub calls a runtime's agent ("DSH"), or a Paseo
     * provider by its own name ("Grok", [unknownAgentName]), never the engine's.
     */
    val agentLabel: String get() = when {
        isCodex -> "Codex"
        agent == "traex" -> "TRAEX"
        isClaude -> "Claude"
        else -> unknownAgentName(agent, agentLabelHint, launcher)
    }
    val isLive: Boolean get() = status != "offline"
    /** [failureReason] without the warning sign botmux starts it with (the app draws its own); null when there is none. */
    val failureText: String? get() = failureReason?.let(::failureNoticeText)?.takeIf(String::isNotEmpty)
    /** [externalUrl] when it is an https link (iOS `externalLink`): anything else is not offered to open. */
    val externalLink: String? get() = externalUrl?.trim()?.takeIf { url ->
        runCatching { java.net.URI(url) }.getOrNull()?.let { it.scheme.equals("https", ignoreCase = true) && !it.host.isNullOrEmpty() } == true
    }
    /**
     * What a list row says under the title instead of the last words (iOS `listPreview`): why it waits for you while it
     * does, else what stopped it while it is not working; null otherwise.
     */
    val reasonPreview: String? get() = when {
        agentState == AgentState.NeedsYou && !attentionReason.isNullOrBlank() -> attentionReason.trim()
        agentState != AgentState.Working -> failureText
        else -> null
    }
    /**
     * "Delete conversation" is offered: not running (offline, finished, seen or unknown), not still starting, not the assistant's (those
     * archive). The hub accepts and then hides exactly these (server/internal/hub/deleted_sessions.go `stillDeleted`).
     */
    val canDelete: Boolean get() = !assistant && !starting && status in setOf("offline", "done", "idle", "unknown", "interrupted")
    val updated: Instant? get() = parseTime(updatedAt)
    /**
     * Its workflows still running, newest first. On a summary row (`summary=1`, one with [waiting]) its [workflowProgress],
     * the newest running run without its agents and phases, stands for them (#5888), even when the row kept an older
     * complete row's [workflows] (`keepingDetails`): fresh summary counts win, as in `backgroundWork()`.
     */
    val runningWorkflows: List<Workflow> get() = if (waiting != null) listOfNotNull(workflowProgress?.takeIf { it.isRunning })
        else workflows.orEmpty().filter { it.isRunning }
    /**
     * How much runs in the background without counting a workflow twice: its other background tasks and its running
     * workflows. An older hub sends no [workflows]: every background entry counts, as before.
     */
    val backgroundCount: Int get() = backgroundCountHint ?: if (workflows == null) background.orEmpty().size
        else background.orEmpty().count { !it.isWorkflow } + runningWorkflows.size
    /** The background tasks that are not workflows, where the hub lists those apart ([workflows]). */
    val backgroundTasks: List<BackgroundTask> get() = if (workflows == null) background.orEmpty() else background.orEmpty().filter { !it.isWorkflow }
    /** An older hub says a workflow runs in [background] but sends no [workflows] to show. */
    val legacyWorkflowRunning: Boolean get() = workflows == null && background.orEmpty().any { it.isWorkflow }
}

/** A host's failure notice without its leading warning signs (U+26A0, U+FE0F) and spaces, as iOS `Session.failureText`. */
fun failureNoticeText(reason: String): String = reason.trim().trimStart('\u26A0', '\uFE0F').trim()

/** All descendants folded under a listed session, counted by their own states. */
@Serializable
data class SessionChildren(
    val total: Int = 0,
    val working: Int = 0,
    @SerialName("needs_you") val needsYou: Int = 0,
    val workers: Int = 0,
    /**
     * The side sessions among them (`folded_as: "side"`, sessions_side_fold, #7944), by their own states, also counted in
     * the fields above. The app reads them only for a row whose sides it does not hold (core:domain `foldRows`).
     */
    val sides: Int = 0,
    @SerialName("sides_working") val sidesWorking: Int = 0,
    @SerialName("sides_needs_you") val sidesNeedsYou: Int = 0,
    @SerialName("sides_unread") val sidesUnread: Int = 0,
)

/** The hub's `folded_as` values. */
object FoldedAs {
    const val WORKER = "worker"
    /** A side session (a Claude desktop app's side chat or /btw question), folded under its topmost listed parent (#7944). */
    const val SIDE = "side"
}

@KeepGeneratedSerializer
@Serializable(with = SessionCompactionSerializer::class)
data class SessionCompaction(
    @WireEnum("running", "done", "failed", "unknown") val state: String = "unknown",
    val error: String? = null,
    @WireTime @SerialName("started_at") val startedAt: String? = null,
    @WireTime @SerialName("finished_at") val finishedAt: String? = null,
) {
    val running: Boolean get() = state == "running"
    val done: Boolean get() = state == "done"
    val failed: Boolean get() = state == "failed"
}

object SessionCompactionSerializer : WireSerializer<SessionCompaction>(SessionCompaction.generatedSerializer())

data class SessionKey(val machineId: String, val sessionId: String) {
    val path: String get() = "$machineId/$sessionId"
}

@KeepGeneratedSerializer
@Serializable(with = ImagePayloadSerializer::class)
data class ImagePayload(
    @SerialName("mime") val mime: String = "image/png",
    @SerialName("data") val data: String = "",
    @SerialName("lazy") val lazy: Boolean? = null,
    @SerialName("width") val width: Int? = null,
    @SerialName("height") val height: Int? = null,
    val size: Int? = null,
)

/**
 * What one model request used (`proto.TokenUsage`, docs/protocol/protocol.md "Token usage on items"). Counts are tokens;
 * [cacheWrite1h] is the part of [cacheWrite] kept for an hour. [id] (`agent:<id>`) marks one subagent's report, counted
 * once per model and speed. Older nodes omit the list.
 */
@Serializable
data class TokenUsage(
    val id: String? = null,
    val model: String? = null,
    @SerialName("in") val input: Long = 0,
    @SerialName("out") val output: Long = 0,
    @SerialName("cache_read") val cacheRead: Long = 0,
    @SerialName("cache_write") val cacheWrite: Long = 0,
    @SerialName("cache_write_1h") val cacheWrite1h: Long = 0,
    val fast: Boolean = false,
) {
    /** A subagent's or workflow agent's report. Entries with the same id, model and speed are one report. */
    val report: Boolean get() = !id.isNullOrBlank()
}

/**
 * The built-in catalog's reference price in USD per one million tokens (`proto.ModelPrice`). A missing kind is unknown,
 * not free. Input and output are present whenever the catalog has a price.
 */
@Serializable
data class ModelPrice(
    val input: Double? = null,
    val output: Double? = null,
    @SerialName("cache_read") val cacheRead: Double? = null,
    @SerialName("cache_write") val cacheWrite: Double? = null,
    @SerialName("cache_write_1h") val cacheWrite1h: Double? = null,
)

@KeepGeneratedSerializer
@Serializable(with = ItemSerializer::class)
data class Item(
    @SerialName("id") val id: String,
    @SerialName("seq") val seq: Int = 0,
    @WireTime @SerialName("ts") val ts: String? = null,
    @SerialName("role") val role: String = "",
    @WireEnum("text", "thinking", "tool_use", "tool_result", "notice", "recap", "image", "fold", "turn_duration", "unknown")
    @SerialName("kind") val kind: String = "unknown",
    @SerialName("text") val text: String? = null,
    @SerialName("tool_use_id") val toolUseId: String? = null,
    @SerialName("tool_name") val toolName: String? = null,
    @SerialName("tool_input") val toolInput: JsonElement? = null,
    @SerialName("is_error") val isError: Boolean? = null,
    @SerialName("full_size") val fullSize: Int? = null,
    @SerialName("image") val image: ImagePayload? = null,
    @SerialName("queued") val queued: Boolean? = null,
    @SerialName("from") val from: String? = null,
    @SerialName("wake") val wake: Boolean? = null,
    val task: TaskNote? = null,
    val fold: Fold? = null,
    /** A user-visible action hidden in this work range; repeated full items retain it as on iOS. */
    @SerialName("supervision_user") val supervisionUser: Boolean? = null,
    @SerialName("turn_duration") val turnDuration: TurnDuration? = null,
    /** The client message id of the message sent from a phone that this item came from (#968): its text, images,
     *  queued copy, or the notice it became. Older hubs never send it. */
    @SerialName("client_msg_id") val clientMsgId: String? = null,
    /** On a notice: the context was compacted here (Claude Code's compact_boundary); the chat draws it as a divider. */
    val compact: CompactBoundary? = null,
    /** A message Persona typed in for the model (a task's notice, a scheduled run…), not by the user; the hub marks it
     *  (docs/protocol/protocol.md "Messages injected by Persona"), older hubs do not (core/domain `injectedOf`). */
    val injected: Injected? = null,
    /** Optional provenance for model input that was not typed by the user. Older hubs omit it. */
    val source: MessageSource? = null,
    /** On a notice: a Claude Code model_refusal_* record (#2289, #2528), its public fields only. */
    @SerialName("model_refusal") val modelRefusal: ModelRefusal? = null,
    /**
     * The model that wrote this assistant item as the transcript records it (Claude Code's `message.model`, a Codex turn's
     * model); null where it does not say (user items, older hubs). The chat marks where the replies' model changes by it.
     */
    val model: String? = null,
    /** What the model requests this item stands for used (#5426). Older hubs omit it. */
    val usage: List<TokenUsage> = emptyList(),
    val narration: Boolean? = null,
    @SerialName("codex_heartbeat_quiet") val codexHeartbeatQuiet: Boolean? = null,
)

/**
 * A Claude Code model_refusal_* record as the hub gives it (server proto.ModelRefusal, #2528): its known fields with
 * values as recorded (unknown ones included) and [text], the refusal's own words. Never the record's line: older hubs
 * also sent it as `raw`, which is not decoded, so it is never shown or kept.
 *
 * The recovery fields are present only when the node projects its own bounded recovery onto this shape (#7824): a
 * runtime-written record never sets them, so they decode to their defaults and older records render unchanged.
 * [recoveryReset] says when the configured model is available again; [recoveryExhausted] marks an exhausted chain
 * (then [fallbackModel] is empty); [recoveryReturning] that the configured model answered a probe and the next request
 * runs on it again; [recoveryTried] lists every model tried, in order, joined by arrows.
 */
@KeepGeneratedSerializer
@Serializable(with = ModelRefusalSerializer::class)
data class ModelRefusal(
    val subtype: String = "",
    @SerialName("original_model") val originalModel: String? = null,
    @SerialName("fallback_model") val fallbackModel: String? = null,
    val category: String? = null,
    val scope: String? = null,
    val explanation: String? = null,
    val text: String? = null,
    @WireTime @SerialName("recovery_reset") val recoveryReset: String? = null,
    @SerialName("recovery_exhausted") val recoveryExhausted: Boolean = false,
    @SerialName("recovery_returning") val recoveryReturning: Boolean = false,
    @SerialName("recovery_tried") val recoveryTried: String? = null,
)

object ModelRefusalSerializer : WireSerializer<ModelRefusal>(ModelRefusal.generatedSerializer())

/** What an assistant chat was switched to in an app (server proto.ModelPick): a launcher and, when picked, its model and effort. */
@KeepGeneratedSerializer
@Serializable(with = ModelPickSerializer::class)
data class ModelPick(
    val launcher: String = "",
    val model: String? = null,
    val effort: String? = null,
)

object ModelPickSerializer : WireSerializer<ModelPick>(ModelPick.generatedSerializer())

/**
 * What POST …/queue answers (docs/protocol/protocol.md, `spec/fixtures/wire/queue.json`): the messages that were queued before
 * the edit, in order ([queue]) and, in a Persona conversation, each one's photos for the composer ([images]: entry i is
 * message i's, null for a message without photos; empty when no message had any).
 */
@KeepGeneratedSerializer
@Serializable(with = QueueEditSerializer::class)
data class QueueEdit(
    val queue: List<String> = emptyList(),
    val images: List<List<ImagePayload>?> = emptyList(),
) {
    /** The photos of the message at [index]; none when it had none. */
    fun imagesOf(index: Int): List<ImagePayload> = images.getOrNull(index).orEmpty()
}

object QueueEditSerializer : WireSerializer<QueueEdit>(QueueEdit.generatedSerializer())

/**
 * A launcher switch waiting for the user's next message (server proto.PendingSwitch): the launcher, the model and effort
 * picked (null: the launcher's default model, the effort it runs with now), the engine when the switch moves the
 * conversation to the other one ([agent], `claude` / `codex`), and when it was picked.
 */
@KeepGeneratedSerializer
@Serializable(with = PendingSwitchSerializer::class)
data class PendingSwitch(
    val launcher: String = "",
    val model: String? = null,
    val effort: String? = null,
    val agent: String? = null,
    @WireTime val at: String? = null,
)

object PendingSwitchSerializer : WireSerializer<PendingSwitch>(PendingSwitch.generatedSerializer())

/** The session and cutoff of one earlier stretch of an Agent conversation. */
@KeepGeneratedSerializer
@Serializable(with = EngineLegSerializer::class)
data class EngineLeg(
    @SerialName("session_id") val sessionId: String = "",
    val agent: String = "",
    @WireTime val until: String? = null,
)

object EngineLegSerializer : WireSerializer<EngineLeg>(EngineLeg.generatedSerializer())

/** A pending "back to the configured model" (server proto.ModelRestore): `next_start`, the next start asks for [to]. */
@KeepGeneratedSerializer
@Serializable(with = ModelRestoreSerializer::class)
data class ModelRestore(
    val state: String = "",
    val to: String? = null,
    @WireTime val at: String? = null,
)

object ModelRestoreSerializer : WireSerializer<ModelRestore>(ModelRestore.generatedSerializer())

/** Source titles are business data; clients localize the fallback for each kind. */
@KeepGeneratedSerializer
@Serializable(with = MessageSourceSerializer::class)
data class MessageSource(
    val kind: String = "",
    @SerialName("conversation_id") val conversationId: String? = null,
    val title: String? = null,
)

object MessageSourceSerializer : WireSerializer<MessageSource>(MessageSource.generatedSerializer())

/** What a message Persona wrote for the model is: [line] is what the chat shows instead of it. */
@KeepGeneratedSerializer
@Serializable(with = InjectedSerializer::class)
data class Injected(
    /** task_notice | followup | system */
    val kind: String = "",
    /** task_notice: done | needs | interrupted | failed; followup: once | daily | interval | wake; system: lost_tasks */
    val state: String? = null,
    val title: String? = null,
    val machine: String? = null,
    @SerialName("todo_id") val todoId: String? = null,
    val line: String = "",
    /** followup once | daily | interval: what the task asks. */
    val detail: String? = null,
    /** system/supervise: info | triage | user — "user" must stay visible in the chat (web `supervisionVisible`); older hubs omit it. */
    val level: String? = null,
)

object InjectedSerializer : WireSerializer<Injected>(Injected.generatedSerializer())

/** How a compaction came about: [trigger] `manual` (/compact) or `auto`, and the context's size before it. */
@KeepGeneratedSerializer
@Serializable(with = CompactBoundarySerializer::class)
data class CompactBoundary(
    val trigger: String? = null,
    @SerialName("pre_tokens") val preTokens: Long? = null,
)

object CompactBoundarySerializer : WireSerializer<CompactBoundary>(CompactBoundary.generatedSerializer())

@KeepGeneratedSerializer
@Serializable(with = TurnDurationSerializer::class)
data class TurnDuration(
    @SerialName("duration_ms") val durationMs: Long? = null,
    val source: String = "",
    @SerialName("turn_id") val turnId: String? = null,
    @SerialName("parent_uuid") val parentUuid: String? = null,
    @SerialName("user_item_id") val userItemId: String? = null,
)

object TurnDurationSerializer : WireSerializer<TurnDuration>(TurnDuration.generatedSerializer())

@KeepGeneratedSerializer
@Serializable(with = QuestionOptionSerializer::class)
data class QuestionOption(
    @SerialName("label") val label: String = "",
    @SerialName("description") val description: String? = null,
    /** A manager-local answer (`defer`); absent from older hubs and from choices that message the Agent. */
    @SerialName("action") val action: String? = null,
)

@KeepGeneratedSerializer
@Serializable(with = QuestionSerializer::class)
data class Question(
    @SerialName("question") val question: String = "",
    @SerialName("header") val header: String? = null,
    @SerialName("multi_select") val multiSelect: Boolean = false,
    @SerialName("options") val options: List<QuestionOption> = emptyList(),
)

@KeepGeneratedSerializer
@Serializable(with = DialogOptionSerializer::class)
data class DialogOption(@SerialName("key") val key: String = "", @SerialName("label") val label: String = "", @SerialName("detail") val detail: String? = null, @SerialName("kind") val kind: String? = null, val keys: List<String>? = null,
    /** Ticked at the computer (a multi-select box) when the dialog was read. */
    @SerialName("checked") val checked: Boolean = false)

/** A terminal dialog the hub parsed (server/internal/screen). One answer shape for both: a single-choice dialog
 *  answers with [Resolution.option]; a multi-select one ([multi]) ticks options and sends them all in
 *  [Resolution.options], its free text in [Resolution.text]. */
@KeepGeneratedSerializer
@Serializable(with = ScreenDialogSerializer::class)
data class ScreenDialog(
    @SerialName("header") val header: String? = null,
    @SerialName("question") val question: String = "",
    @SerialName("detail") val detail: String? = null,
    @SerialName("options") val options: List<DialogOption> = emptyList(),
    @SerialName("rev") val rev: String = "",
    /** Options are checkboxes, sent together by the submit row, instead of one answer on tap. */
    @SerialName("multi") val multi: Boolean = false,
    /** The submit row's own label ("Submit", or "Next" when another question follows). */
    @SerialName("submit") val submit: String? = null,
    /** Claude Code's command prompt, read off the terminal (#5265); absent on older hubs and other dialogs. */
    @SerialName("command") val command: String? = null,
    @SerialName("description") val description: String? = null,
    @SerialName("warning") val warning: String? = null,
    @SerialName("risk") val risk: String? = null,
    @SerialName("deadline") val deadline: String? = null,
    @SerialName("deadline_action") val deadlineAction: String? = null,
    @WireTime @SerialName("deadline_at") val deadlineAt: String? = null,
)

/** Something a permission card asks to fill in before approving (`proto.Field`, a password); see [Interaction.fields]. */
@KeepGeneratedSerializer
@Serializable(with = InteractionFieldSerializer::class)
data class InteractionField(
    val key: String = "",
    val label: String = "",
    val placeholder: String? = null,
    val secret: Boolean = false,
    val optional: Boolean = false,
)

/** The context the assistant filled in when it left a question to the user (`proto.Delegate`). */
@KeepGeneratedSerializer
@Serializable(with = DelegateSerializer::class)
data class Delegate(
    @SerialName("what") val what: String? = null,
    @SerialName("why_now") val whyNow: String? = null,
    /** An option's exact label; several questions are joined with a full-width semicolon. */
    @SerialName("recommend") val recommend: String? = null,
    @SerialName("cost") val cost: String? = null,
    @SerialName("reason") val reason: String? = null,
    @WireTime @SerialName("deadline") val deadline: String? = null,
)

@KeepGeneratedSerializer
@Serializable(with = InteractionSerializer::class)
data class Interaction(
    @SerialName("id") val id: String,
    @SerialName("machine_id") val machineId: String = "",
    @SerialName("session_id") val sessionId: String = "",
    @SerialName("session_title") val sessionTitle: String? = null,
    @SerialName("folded_under") val foldedUnder: String? = null,
    @SerialName("folded_as") val foldedAs: String? = null,
    /** The side session whose chat this card belongs to when it comes from a row folded through one (#7944). */
    @SerialName("folded_side") val foldedSide: String? = null,
    /** The task its agent works for (docs/assistant/unified-entry.md), named by the hub. */
    @SerialName("task_title") val taskTitle: String? = null,
    @SerialName("cwd") val cwd: String? = null,
    @WireEnum("question", "plan", "permission", "screen", "elicitation", "unknown")
    @SerialName("kind") val kind: String = "unknown",
    @WireTime @SerialName("expires_at") val expiresAt: String? = null,
    @SerialName("tool_use_id") val toolUseId: String? = null,
    val resumable: Boolean? = null,
    @WireTime @SerialName("created_at") val createdAt: String? = null,
    @SerialName("tool_name") val toolName: String? = null,
    @SerialName("questions") val questions: List<Question>? = null,
    /** The runtime can discard this question without answering or interrupting the task. */
    @SerialName("can_cancel") val canCancel: Boolean? = null,
    val reversible: Boolean? = null,
    @SerialName("plan") val plan: String? = null,
    @SerialName("plan_then") val planThen: String? = null,
    @SerialName("tool_input") val toolInput: JsonElement? = null,
    @SerialName("summary") val summary: String? = null,
    @SerialName("screen") val screen: String? = null,
    @SerialName("dialog") val dialog: ScreenDialog? = null,
    @SerialName("secret") val secret: Boolean? = null,
    @SerialName("agent_id") val agentId: String? = null,
    @SerialName("agent_type") val agentType: String? = null,
    @SerialName("mcp_server_name") val mcpServerName: String? = null,
    @SerialName("mcp_config_source") val mcpConfigSource: String? = null,
    val elicitation: Elicitation? = null,
    /** Who asks when it is not Claude Code: "codex", "traex", or an agent runtime's agent ("dsh"; docs/integrations/runtimes.md). */
    val agent: String? = null,
    /** What the hub calls an agent this app does not know ("DSH"). */
    @SerialName("agent_label") val agentLabelHint: String? = null,
    /** What "always allow" allows here; a card from another agent offers it only with this (as on iOS). */
    @SerialName("always_label") val alwaysLabel: String? = null,
    /** The persisted scope granted by this card; absent on older hubs and terminal-only sessions. */
    @SerialName("always_keys") val alwaysKeys: List<String>? = null,
    /**
     * Whether the manager keeps an "always allow" for this card (#3590): false on its own cards, which keep nothing, so
     * they offer only Allow and Deny; true where it does, another agent's card included. Absent (an older hub, an
     * Agents session's card): the card's own rule (`offersAlways` in core:domain).
     */
    @SerialName("always_honored") val alwaysHonored: Boolean? = null,
    /** Why the agent asks for this permission, in its own words (Codex). */
    val reason: String? = null,
    /** Filled in before approving (adding a mail account's password); the hub refuses allow without the required ones. */
    val fields: List<InteractionField>? = null,
    /** Owning manager, including page permissions; older hubs omit it for the default manager. */
    @SerialName("manager") val manager: String? = null,
    /** The manager's name for the tool, including what this call does where it adds that ("Report to supervisor · Done"). */
    @SerialName("tool_title") val toolTitle: String? = null,
    /** The tool alone, without what this call does, for the scope of "Always allow". */
    @SerialName("tool_name_title") val toolNameTitle: String? = null,
    /** The rule a question a supervisor passed up suggests (#4929); the card comes again without it once it was answered. */
    val suggestion: RuleSuggestion? = null,
    /** A prepared approval whose current deadline is authoritative while the hub keeps it pending; absent on older hubs. */
    @SerialName("can_clarify") val canClarify: Boolean = false,
    /** A server-owned exact proposal; its complete original payload, not an Always rule, is the approval authority. */
    @SerialName("approval_batch_id") val approvalBatchId: String? = null,
    @SerialName("approval_item_id") val approvalItemId: String? = null,
    @SerialName("owner_proposal") val ownerProposal: Boolean? = null,
    @SerialName("no_expiry") val noExpiry: Boolean? = null,
    @SerialName("owner_approval_phrases") val ownerApprovalPhrases: List<String>? = null,
    /** The assistant is still deciding whether it can answer this question; nothing was pushed yet. */
    @SerialName("delegating") val delegating: Boolean = false,
    /** What the assistant filled in when it handed the question to the user. */
    @SerialName("delegate") val delegate: Delegate? = null,
) {
    val sessionKey: SessionKey get() = SessionKey(machineId, sessionId)
    /** The one exact reply this card advertises. Anything else stays an ordinary card action. */
    val ownerApprovalPhrase: String?
        get() = ownerApprovalPhrases?.firstOrNull()?.takeIf {
            kind == "permission" && ownerProposal == true && !approvalBatchId.isNullOrBlank() &&
                machineId.isNotBlank() && sessionId.isNotBlank() && it.isNotBlank()
        }
    /** Only questions are delegated; a direct answer while this is true still wins. */
    val isDelegating: Boolean get() = kind == "question" && delegating
    /** A supported following mode; absent, plan and unknown values keep the legacy approval choices (#3784). */
    val recordedPlanThen: String?
        get() = when (planThen) {
            "default", "acceptEdits", "auto", "bypassPermissions" -> planThen
            else -> null
        }
    /** The page that answers it; resolution still uses this interaction's own id and session. */
    val homeKey: SessionKey get() = SessionKey(machineId, foldedUnder?.takeIf(String::isNotBlank) ?: sessionId)

    /** Asked by an agent other than Claude Code (Codex, a compatible CLI, a runtime's): the card is that agent's, never Claude's. */
    val fromOtherAgent: Boolean get() = !agent.isNullOrEmpty() && agent != "claude"

    /**
     * The agent's name on a card not from Claude Code ("Codex", "a compatible CLI", "DSH", a Paseo provider's "Grok");
     * null for Claude's, and for one that names only Paseo, its engine (a node before #8142 labels every card of Paseo's
     * other providers so): that card reads as one whose agent can't be named (#8143). [fromOtherAgent] says whose card it is.
     */
    val agentDisplayName: String?
        get() = when (agent) {
            "codex" -> "Codex"
            "traex" -> "TRAEX"
            null, "", "claude" -> null
            PaseoAgent -> agentLabelHint?.trim()?.takeUnless { it.isEmpty() || it.equals(PaseoAgent, ignoreCase = true) }
            else -> agentLabelHint?.takeIf(String::isNotBlank) ?: agent
        }

    /**
     * The assistant's password card (`secret_run`, #1137, #1581): commands run on one of the user's computers with a
     * password the user types into the card. Only the card's own form in the conversation or the Agents tab takes the
     * password (Run); every quick entry offers only Deny.
     */
    val isSecretRun: Boolean get() = kind == "permission" && toolName == "secret_run"

    /** The manager's one-run secret hand-off (#3962). Only its full card may approve it. */
    val isSecretHandoff: Boolean get() = kind == "permission" && toolName == "secret_handoff"

    /** The allowlisted, provider-described items and run context; never a reference or a secret value. */
    val secretHandoff: SecretHandoffRequest?
        get() {
            if (!isSecretHandoff) return null
            val input = toolInput as? JsonObject
            fun text(key: String, source: JsonObject? = input) =
                (source?.get(key) as? JsonPrimitive)?.takeIf { it.isString }?.content?.trim().orEmpty()
            // Never hide a malformed variable and leave the remaining secrets approvable: the owner must see every
            // variable covered by this approval, so any unreadable entry invalidates the whole card.
            val rawVariables = input?.get("variables") as? JsonArray ?: return null
            val variables = rawVariables.map { entry ->
                val variable = entry as? JsonObject ?: return null
                val name = text("name", variable)
                if (name.isEmpty()) return null
                SecretHandoffVariable(name, text("provider_name", variable).ifEmpty { text("provider", variable) },
                    text("title", variable))
            }
            return SecretHandoffRequest(variables, text("agent"), text("machine_name"), text("cwd"), text("prompt"))
        }

    /** A password card's computer, purpose and commands (the commands exactly as sent); null for any other card. */
    val secretRun: SecretRunRequest?
        get() {
            if (!isSecretRun) return null
            val input = toolInput as? JsonObject
            fun text(key: String) = (input?.get(key) as? JsonPrimitive)?.takeIf { it.isString }?.content?.trim().orEmpty()
            val commands = (input?.get("commands") as? JsonArray).orEmpty()
                .mapNotNull { command -> (command as? JsonPrimitive)?.takeIf { it.isString }?.content?.takeIf(String::isNotBlank) }
            return SecretRunRequest(text("machine_name").ifEmpty { text("machine") }, text("purpose"), commands)
        }

    /** One line for a notification or a list row. */
    val headline: CoreText
        get() = when (kind) {
            // Another agent's card that can't name it says only that it asks (DESIGN_SYSTEM.md 5.6), never "Claude".
            "question" -> questions?.firstOrNull()?.question?.let(CoreText::plain)
                ?: agentDisplayName?.let { CoreText.of(Key.HeadlineQuestion, it) }
                ?: if (fromOtherAgent) CoreText.of(Key.AskQuestion) else CoreText.of(Key.HeadlineQuestion, "Claude")
            "elicitation" -> CoreText.of(Key.HeadlineElicitation)
            "plan" -> CoreText.of(Key.FleetPlanWaiting)
            // A hand-off's unknown input is never a headline; a password card names its computer.
            "permission" -> if (isSecretHandoff) CoreText.of(Key.HeadlineSecretHandoff) else
                secretRun?.let { CoreText.of(Key.HeadlineSecretRun, it.machine) } ?: run {
                // The manager's title is for people; the raw toolName still decides the card's type and icon.
                val tool: Any = toolTitle?.takeIf(String::isNotBlank) ?: toolDisplayName(toolName) ?: CoreText.of(Key.ToolGeneric)
                val detail = summary?.takeIf { it.isNotBlank() }
                val agent = agentDisplayName
                when {
                    agent != null && detail != null -> CoreText.of(Key.HeadlineAgentPermissionDetail, agent, tool, detail)
                    agent != null -> CoreText.of(Key.HeadlineAgentPermission, agent, tool)
                    detail != null -> CoreText.of(Key.HeadlinePermissionDetail, tool, detail)
                    else -> CoreText.of(Key.HeadlinePermission, tool)
                }
            }
            "screen" -> dialog?.question?.takeIf { it.isNotBlank() }?.let(CoreText::plain) ?: CoreText.of(Key.HeadlineScreen)
            else -> CoreText.of(Key.HeadlineNeedsYou)
        }
}

/** One variable a hand-off offers, and the provider-described item its value comes from. */
data class SecretHandoffVariable(val name: String, val provider: String, val title: String)

/** The complete, non-secret context the owner approves for one Agent run. */
data class SecretHandoffRequest(
    val variables: List<SecretHandoffVariable>,
    val agent: String,
    val machine: String,
    val cwd: String,
    val prompt: String,
)

/** What a password card ([Interaction.isSecretRun], #1137) would run: the computer's name, why, and the commands. */
data class SecretRunRequest(val machine: String, val purpose: String, val commands: List<String>) {
    companion object {
        /** The card's one field, declared by the hub: Run sends the typed password under this key in [Resolution.values]. */
        const val PASSWORD = "password"
    }
}

@Serializable
data class Resolution(
    @SerialName("answers") val answers: Map<String, String>? = null,
    @SerialName("approve") val approve: Boolean? = null,
    @SerialName("mode") val mode: String? = null,
    @SerialName("feedback") val feedback: String? = null,
    @SerialName("allow") val allow: Boolean? = null,
    @SerialName("always") val always: Boolean? = null,
    @SerialName("message") val message: String? = null,
    @SerialName("keys") val keys: List<String>? = null,
    @SerialName("text") val text: String? = null,
    @SerialName("dismiss") val dismiss: Boolean? = null,
    @SerialName("cancel") val cancel: Boolean? = null,
    @SerialName("pre_keys") val preKeys: List<String>? = null,
    @SerialName("option") val option: String? = null,
    /** A multi-select dialog's checked option keys. */
    @SerialName("options") val options: List<String>? = null,
    /** A permission card's field values; only a password card's Run sends one ([SecretRunRequest.PASSWORD]). */
    @SerialName("values") val values: Map<String, String>? = null,
    /** Who answered when it was not the user in the app (`assistant`); only in a resolution the hub hands back. */
    @SerialName("answered_by") val answeredBy: String? = null,
    /** Why the assistant answered so, beside [answeredBy]; only in a resolution the hub hands back. */
    @SerialName("reason") val reason: String? = null,
    val action: String? = null,
    val content: kotlinx.serialization.json.JsonObject? = null,
    /** Runtime-only exact proposal reply; deliberately never serialized into an ordinary card resolution. */
    @kotlinx.serialization.Transient val ownerApprovalPhrase: String? = null,
)

/** A row of GET /api/v1/interactions/history (iOS `InteractionRecord`): a session's card, how it ended, its answer and when. */
@KeepGeneratedSerializer
@Serializable(with = InteractionRecordSerializer::class)
data class InteractionRecord(
    val interaction: Interaction,
    /** pending, resolved, expired, cancelled or dismissed. */
    val status: String = "",
    val resolution: Resolution? = null,
    @WireTime @SerialName("resolved_at") val resolvedAt: String? = null,
)

object InteractionRecordSerializer : WireSerializer<InteractionRecord>(InteractionRecord.generatedSerializer())

/**
 * A question the assistant answered for the user (iOS `DelegatedAnswer`, docs/assistant/assistant.md "Assistant answers
 * questions first"): the card, what it answered by question text and why, and when (null: unsaid).
 */
data class DelegatedAnswer(val interaction: Interaction, val answers: Map<String, String>, val reason: String, val at: Instant?) {
    val id: String get() = interaction.id

    companion object {
        /** `answered_by` on a question the assistant answered. */
        const val ASSISTANT = "assistant"
    }
}

@Serializable
data class Launcher(@SerialName("name") val name: String, @SerialName("base_url") val baseUrl: String? = null, @SerialName("model") val model: String? = null)

object MachineSerializer : WireSerializer<Machine>(Machine.generatedSerializer())
object MachineLinkSwitchSerializer : WireSerializer<MachineLinkSwitch>(MachineLinkSwitch.generatedSerializer())
object ListeningPortSerializer : WireSerializer<ListeningPort>(ListeningPort.generatedSerializer())

/** A port one of a session's processes listens on and answers HTTP at (`proto.Port`). */
@KeepGeneratedSerializer
@Serializable(with = ListeningPortSerializer::class)
data class ListeningPort(
    val port: Int,
    val addr: String? = null,
    val pid: Int? = null,
    val process: String? = null,
    /** The `<title>` of its `/`, when it answered in time. */
    val title: String? = null,
)
object DetectedEngineSerializer : WireSerializer<DetectedEngine>(DetectedEngine.generatedSerializer())
object LoopInfoSerializer : WireSerializer<LoopInfo>(LoopInfo.generatedSerializer())
object GoalInfoSerializer : WireSerializer<GoalInfo>(GoalInfo.generatedSerializer())
object BackgroundTaskSerializer : WireSerializer<BackgroundTask>(BackgroundTask.generatedSerializer())
object SessionCapabilitiesSerializer : WireSerializer<SessionCapabilities>(SessionCapabilities.generatedSerializer())
object SessionTodoSerializer : WireSerializer<SessionTodo>(SessionTodo.generatedSerializer())
object WorkflowSerializer : WireSerializer<Workflow>(Workflow.generatedSerializer())
object WorkflowAgentSerializer : WireSerializer<WorkflowAgent>(WorkflowAgent.generatedSerializer())
object WorkflowPhaseStatsSerializer : WireSerializer<WorkflowPhaseStats>(WorkflowPhaseStats.generatedSerializer())
object WorkflowToolSerializer : WireSerializer<WorkflowTool>(WorkflowTool.generatedSerializer())
object SessionSerializer : WireSerializer<Session>(Session.generatedSerializer())
object WorkStateSerializer : WireSerializer<WorkState>(WorkState.generatedSerializer())
object WorkAgentSerializer : WireSerializer<WorkAgent>(WorkAgent.generatedSerializer())
object ImagePayloadSerializer : WireSerializer<ImagePayload>(ImagePayload.generatedSerializer())
object ItemSerializer : WireSerializer<Item>(Item.generatedSerializer())
object QuestionOptionSerializer : WireSerializer<QuestionOption>(QuestionOption.generatedSerializer())
object QuestionSerializer : WireSerializer<Question>(Question.generatedSerializer())
object DialogOptionSerializer : WireSerializer<DialogOption>(DialogOption.generatedSerializer())
object ScreenDialogSerializer : WireSerializer<ScreenDialog>(ScreenDialog.generatedSerializer())
object DelegateSerializer : WireSerializer<Delegate>(Delegate.generatedSerializer())
object InteractionSerializer : WireSerializer<Interaction>(Interaction.generatedSerializer())
object InteractionFieldSerializer : WireSerializer<InteractionField>(InteractionField.generatedSerializer())
