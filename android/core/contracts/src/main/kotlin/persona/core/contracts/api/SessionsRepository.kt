package persona.core.contracts.api

import persona.core.contracts.Machine
import persona.core.contracts.Session
import persona.core.contracts.SessionKey
import kotlinx.coroutines.flow.MutableStateFlow
import kotlinx.coroutines.flow.StateFlow

/**
 * [total] is the active filter's complete membership, not the loaded window length.
 * [counts] are state-chip totals with state omitted from [filter]. [machineCounts]
 * also omits machineId, for cross-machine chips. Neither is the active state's group count.
 */
data class SessionListStatus(
    val paged: Boolean = false,
    val loaded: Boolean = false,
    val loading: Boolean = false,
    /** The last request failed; an empty first page offers retry instead of a permanent spinner. */
    val failed: Boolean = false,
    val hasMore: Boolean = false,
    val hasPrevious: Boolean = false,
    val total: Int = 0,
    val counts: SessionListCounts = SessionListCounts(),
    val machineCounts: SessionListCounts = SessionListCounts(),
    val filter: SessionListFilter = SessionListFilter(),
)

private val legacySessionListStatus: StateFlow<SessionListStatus> = MutableStateFlow(SessionListStatus(loaded = true))
private val noAdoptedSessions: StateFlow<List<Session>> = MutableStateFlow(emptyList())
private val noStartedThreads: StateFlow<Map<SessionKey, Session>> = MutableStateFlow(emptyMap())

interface SessionsRepository {
    val machines: StateFlow<List<Machine>>
    val sessions: StateFlow<List<Session>>
    /** Start/send receipts for chat metadata, independent of the filtered list's membership and counts. */
    val adoptedSessions: StateFlow<List<Session>> get() = noAdoptedSessions
    /** Confirmed first-message handoffs, keyed by the exact waiting session that received the message. */
    val startedThreads: StateFlow<Map<SessionKey, Session>> get() = noStartedThreads
    val listStatus: StateFlow<SessionListStatus> get() = legacySessionListStatus
    /** Capabilities of the hub owning this machine; merged lists must not borrow the selected hub's. */
    fun supportsFeature(machineId: String, feature: String): Boolean = false
    suspend fun refresh()
    /** Refresh only the hub that owns this machine; merged repositories must not let another hub's success hide its failure. */
    suspend fun refreshMachine(machineId: String) = refresh()
    suspend fun setListFilter(filter: SessionListFilter) {}
    /** Hub-group scope, separate from the server's machine-local group filter; null includes all hubs. */
    suspend fun setListNamespaces(namespaces: Set<String?>?) {}
    suspend fun loadMore() {}
    suspend fun loadPrevious() {}
    /** Aggregate metadata for a visible group; never downloads its session rows. */
    suspend fun listCounts(filter: SessionListFilter): SessionListCounts = throw SessionListsUnsupported()

    /**
     * A session the hub just created (an async start answers with it while the agent is still starting): listed at
     * once, so its chat has metadata before the next `sessions` push. A session already listed is left as it is.
     */
    suspend fun adopt(session: Session) {}
    suspend fun adoptThread(previous: SessionKey, session: Session) { adopt(session) }

    /** A machine the hub just returned (PUT …/engine): shown at once, before the next `machines` push. */
    suspend fun applyMachine(machine: Machine) {}
}
