package persona.core.domain.sort

import persona.core.contracts.CoreText
import persona.core.contracts.CoreText.Key
import persona.core.contracts.Session

/**
 * By App (#3756 / #9199, iOS `AppGroups`): a section per desktop app an agent runs in, then one per terminal engine,
 * then Other. Running and offline alike, latest first ([recentAgentOrder]).
 */
data class AppGroup(
    val id: String,
    /** The desktop app's display name ("Claude Code", "Codex", "TRAE"); null for an engine section and Other. */
    val kindName: String? = null,
    /** The terminal engine id (`herdr`, `herdr-core`, …); null for a desktop app and Other. */
    val engine: String? = null,
    val sessions: List<AgentSession> = emptyList(),
    val projects: List<AppProject> = emptyList(),
) {
    /** Projects have headings unless every agent is in Other alone (iOS `showsProjects`). */
    val showsProjects: Boolean get() = projects.isNotEmpty() && !(projects.size == 1 && projects[0].isOther)

    val title: CoreText get() = when {
        engine != null -> CoreText.plain(AppGroups.engineName(engine))
        kindName == null -> CoreText.of(Key.RepoOther)
        kindName == "Claude Code" -> CoreText.of(Key.SourceClaudeDesktop)
        kindName == "Codex" -> CoreText.of(Key.AgentsCodexApp)
        else -> CoreText.of(Key.AgentsDesktopApp, kindName)
    }
}

/** A project inside a By App section (iOS `AppProject`). */
data class AppProject(
    val id: String,
    val name: String,
    val machineId: String? = null,
    val sessions: List<AgentSession> = emptyList(),
    /** Codex App sidebar order from `space_index` (≥ 1); null when the app does not number it. */
    val place: Int? = null,
) {
    val isOther: Boolean get() = name.isEmpty()
    val title: CoreText get() = if (isOther) CoreText.of(Key.RepoOther) else CoreText.plain(name)
}

object AppGroups {
    const val OTHER_ID = "app/other"
    /** Desktop apps first in this order; any other after them by id. */
    private val known = listOf("Claude Code", "Codex")
    /** Terminal engines with their own section, after the desktop apps (iOS `AppGroups.engines`). */
    val engines = listOf("herdr", "herdr-core", "orca", "tern", "rex")

    fun engineName(id: String): String = when (id) {
        "herdr" -> "herdr"
        "herdr-core" -> "Herdr App"
        "orca" -> "Orca"
        "tern" -> "Tern"
        "rex" -> "Rex"
        else -> id
    }

    fun engineOf(session: Session): String? =
        session.spaceKind?.takeIf { it in engines }

    /**
     * Which desktop app a session belongs to (iOS `AgentKind`): Codex by agent or source, TRAE by id, Claude when unnamed
     * or `claude`, else the agent's label.
     */
    fun kindName(session: Session): String = when {
        session.isCodex -> "Codex"
        session.agent?.lowercase() in TRAE -> "TRAE"
        session.isClaude -> "Claude Code"
        else -> session.agentLabel
    }

    fun build(sessions: List<AgentSession>): List<AppGroup> {
        val desktop = linkedMapOf<String, MutableList<AgentSession>>()
        val byEngine = linkedMapOf<String, MutableList<AgentSession>>()
        val other = mutableListOf<AgentSession>()
        val kinds = mutableMapOf<String, String>()
        for (row in sessions) {
            val s = row.session
            if (s.agentSurface != "desktop") {
                val engine = engineOf(s)
                if (engine != null) byEngine.getOrPut("app/engine/$engine") { mutableListOf() }.add(row)
                else other.add(row)
                continue
            }
            val name = kindName(s)
            val id = "app/desktop/" + name.lowercase()
            kinds[id] = name
            desktop.getOrPut(id) { mutableListOf() }.add(row)
        }
        fun rank(id: String) = known.indexOf(kinds[id] ?: "").takeIf { it >= 0 } ?: known.size
        val groups = desktop.keys.sortedWith(compareBy(::rank).thenBy { it }).map { id ->
            AppGroup(id, kindName = kinds[id], sessions = desktop.getValue(id))
        }.toMutableList()
        for (engine in engines) {
            val id = "app/engine/$engine"
            byEngine[id]?.let { groups += AppGroup(id, engine = engine, sessions = it) }
        }
        if (other.isNotEmpty()) groups += AppGroup(OTHER_ID, sessions = other)
        return groups.map { group ->
            val ordered = recentAgentOrder(group.sessions)
            group.copy(
                sessions = ordered,
                projects = if (group.id == OTHER_ID) emptyList() else projectsOf(group.copy(sessions = ordered)),
            )
        }
    }

    fun projectsOf(group: AppGroup): List<AppProject> {
        val byKey = linkedMapOf<String, AppProject>()
        val order = mutableListOf<String>()
        val other = mutableListOf<AgentSession>()
        for (row in group.sessions) {
            val project = projectOf(row.session)
            if (project == null) {
                other += row
                continue
            }
            val (key, name) = project
            if (key !in byKey) {
                byKey[key] = AppProject(
                    id = group.id + "/project/" + key + "#",
                    name = name,
                    machineId = row.session.machineId,
                    sessions = emptyList(),
                    place = placeOf(row.session),
                )
                order += key
            }
            val current = byKey.getValue(key)
            byKey[key] = current.copy(
                sessions = current.sessions + row,
                place = current.place ?: placeOf(row.session),
            )
        }
        val out = inAppOrder(order.map { byKey.getValue(it) }).toMutableList()
        if (other.isNotEmpty()) {
            out += AppProject(id = group.id + "/project/other#", name = "", sessions = other)
        }
        return out
    }

    /** Codex-numbered projects first by place; the rest keep arrival order (iOS `inAppOrder`). */
    fun inAppOrder(projects: List<AppProject>): List<AppProject> =
        projects.withIndex().sortedWith { a, b ->
            when {
                a.value.place != null && b.value.place != null && a.value.place != b.value.place ->
                    a.value.place!!.compareTo(b.value.place!!)
                a.value.place != null && b.value.place == null -> -1
                a.value.place == null && b.value.place != null -> 1
                else -> a.index.compareTo(b.index)
            }
        }.map { it.value }

    fun placeOf(session: Session): Int? =
        session.spaceIndex?.takeIf { session.spaceKind == "codex_app" && it > 0 }

    /** Project key and display name; null when the session has none (home, Codex chats folder, …). */
    fun projectOf(session: Session): Pair<String, String>? {
        if (session.spaceKind == "codex_app") {
            val space = session.spaceId?.takeIf { it.isNotEmpty() } ?: return null
            if (space.startsWith("codex-chats:") || space == "codex-app:chats") return null
            val name = session.spaceName?.trim()?.takeIf { it.isNotEmpty() } ?: return null
            if (space.startsWith("codex-project:")) {
                val root = space.removePrefix("codex-project:")
                if (isHome(root) || isCodexChatFolder(root)) return null
            }
            return machinePath(session) + "/" + space to name
        }
        val place = gitPlaceOf(session.repoRoot, session.worktree) ?: repoOf(session.cwd, session.repoRoot)
        var path = place.repo
        var name = place.name
        if (isHome(path) && !isHome(session.cwd)) {
            path = session.cwd
            name = ""
        }
        while (path.length > 1 && (path.endsWith("/") || path.endsWith("\\"))) path = path.dropLast(1)
        if (path.isEmpty() || isHome(path)) return null
        if (name.isEmpty()) name = path.replace('\\', '/').substringAfterLast('/')
        return if (name.isEmpty()) null else machinePath(session) + "/" + path to name
    }

    fun isCodexChatFolder(path: String): Boolean {
        val parts = path.split('/', '\\').filter { it.isNotEmpty() }
        if (parts.size < 4) return false
        val n = parts.size
        val date = parts[n - 2]
        return parts[n - 4] == "Documents" && parts[n - 3] == "Codex" && date.length == 10 &&
            date.withIndex().all { (i, c) -> if (i == 4 || i == 7) c == '-' else c.isDigit() }
    }

    fun isHome(path: String): Boolean {
        val parts = path.split('/', '\\').filter { it.isNotEmpty() }
        if (parts.isEmpty() || path == "~") return true
        if (parts.size == 1) return parts[0] == "root" || parts[0] == "Users" || parts[0] == "home" || parts[0].endsWith(":")
        if (parts.size == 2) return parts[0] == "Users" || parts[0] == "home" || (parts[0].endsWith(":") && parts[1] == "Users")
        return parts.size == 3 && parts[0].endsWith(":") && parts[1] == "Users"
    }

    // Android's Session only carries machineId; iOS uses machineRef.path (hub-qualified). No hub id on the wire type yet.
    private fun machinePath(session: Session): String = session.machineId

    private val TRAE = setOf("trae", "trae-cli", "traecli")
}

/**
 * A flat activity timeline (iOS `AgentListOrder.recent`): `updated_at` descending, then session id; missing times last.
 * Reading a reply does not move a row.
 */
fun recentAgentOrder(rows: List<AgentSession>): List<AgentSession> =
    rows.sortedWith(
        compareByDescending<AgentSession> { it.updatedAt ?: java.time.Instant.MIN }
            .thenBy { it.id },
    )
