package persona.core.domain.engines

import persona.core.contracts.CoreText.Key
import persona.core.contracts.CoreText
import persona.core.contracts.DetectedEngine
import persona.core.contracts.Machine
import persona.core.contracts.api.NO_TERMINAL_ENGINE

/**
 * One terminal engine per machine, or none (docs/integrations/engines.md), as on iOS `EngineSelectionModel`. The options are what
 * the hub reports in `engines`; [known] only names the engines this build knows: for hubs that report nothing, and to decide
 * what may be chosen without detection (an offline machine). [NONE] is the empty host set: it is listed, and read back from
 * `engine_hosts`, only for a hub that reports host sets.
 */
object Engines {
    const val DEFAULT = "herdr"

    /** No terminal engine: not a hub engine id, saved as the empty host set and not in [known]. */
    const val NONE = NO_TERMINAL_ENGINE

    /** An engine this build knows: its id, a fallback name, and where its sessions live. */
    data class Known(val id: String, val name: String, val detail: CoreText)

    val known = listOf(
        Known("herdr", "herdr", CoreText.of(Key.EngineHerdr)),
        Known("herdr-core", "Herdr App", CoreText.of(Key.EngineHerdrApp)),
        Known("orca", "Orca", CoreText.of(Key.EngineOrca)),
        Known("tern", "Tern", CoreText.of(Key.EngineTern)),
        Known("rex", "Rex", CoreText.of(Key.EngineRex)),
        Known("paseo", "Paseo", CoreText.of(Key.EnginePaseo)),
        Known("cindy", "Cindy", CoreText.of(Key.EngineCindy)),
    )

    fun detail(id: String): CoreText? =
        if (id == NONE) CoreText.of(Key.EngineNoneDetail) else known.firstOrNull { it.id == id }?.detail

    /** The hub reports host sets and the machine enables none: `engine` still says herdr for it, so it cannot be read from there. */
    fun isNone(machine: Machine): Boolean = machine.engineHosts?.isEmpty() == true

    /** An older hub reports no host sets and would refuse the empty one: no such option there. */
    fun offersNone(machine: Machine): Boolean = machine.engineHosts != null

    /** The chosen engine: [NONE] for the empty set, else the machine's engine; herdr when the hub sent none. */
    fun current(machine: Machine): String = if (isNone(machine)) NONE else machine.engine.ifBlank { DEFAULT }

    /**
     * The engine a screen judges the machine by (herdr's stopped notice, the health features): [NONE] for the empty set, else the
     * reported `engine` as is. Unlike [current] a blank one stays blank, so a hub that reports no engine is read as before.
     */
    fun judged(machine: Machine): String = if (isNone(machine)) NONE else machine.engine

    /** The chosen engine's name: the hub's name for it, else the known one, else its id. */
    fun currentName(machine: Machine): String = name(current(machine), machine)

    /** [currentName] as words: [NONE] has no name the hub or this build could spell out, it is a localized one. */
    fun currentLabel(machine: Machine): CoreText = label(current(machine), machine)

    fun label(id: String, machine: Machine): CoreText = if (id == NONE) CoreText.of(Key.EngineNone) else CoreText.plain(name(id, machine))

    fun name(id: String, machine: Machine): String = machine.engines.firstOrNull { it.id == id }?.name?.takeIf { it.isNotBlank() }
        ?: if (id == NONE) Key.EngineNone.english else known.firstOrNull { it.id == id }?.name ?: id

    /** The machine pairing asks about: the hub's own (kind local), else the first one. */
    fun pairingMachine(machines: List<Machine>): Machine? = machines.firstOrNull { it.kind == "local" } ?: machines.firstOrNull()

    /** Pairing asks only when the hub reports engines: an older hub cannot save a choice and the step must not trap. */
    fun pairingStep(machines: List<Machine>): Machine? = pairingMachine(machines)?.takeIf { it.engines.isNotEmpty() }

    /**
     * What the picker starts on: the machine's current engine, herdr when never chosen. Never guessed from what happens
     * to be detected (2026-11-01, the user: an engine must be picked, herdr by default).
     */
    fun initialChoice(machine: Machine): String = current(machine)

    /**
     * The rows to list: the hub's `engines` as given; with none reported, the known engines (not detected). No terminal engine
     * comes last, and only from a hub that reports host sets.
     */
    fun options(machine: Machine): List<DetectedEngine> {
        val detected = machine.engines.ifEmpty { known.map { DetectedEngine(it.id, it.name, available = false) } }
        return if (offersNone(machine)) detected + DetectedEngine(NONE, Key.EngineNone.english, available = true) else detected
    }

    /**
     * Known engines can be set without detection (offline: applied on connect); an unknown one only when found there. No
     * terminal engine needs the hub's host sets, and nothing to detect.
     */
    fun canSelect(id: String, machine: Machine): Boolean =
        if (id == NONE) offersNone(machine) else known.any { it.id == id } || machine.engines.any { it.id == id && it.available }

    /** Whether an option says if it was found there: not for [NONE], it is there whenever the hub takes host sets. */
    fun showsDetection(id: String): Boolean = id != NONE

    /** The detection line under an option; missing detection is not reported as "Not detected". */
    fun detection(id: String, machine: Machine): CoreText {
        if (!machine.online) return CoreText.of(Key.EngineOffline)
        val found = machine.engines.firstOrNull { it.id == id } ?: return CoreText.of(Key.EngineUnknown)
        return when {
            !found.available -> CoreText.of(Key.EngineMissing)
            !found.version.isNullOrBlank() -> CoreText.of(Key.EngineFoundVersion, found.version.orEmpty())
            else -> CoreText.of(Key.EngineFound)
        }
    }

    /** The note under the options, for the option [selected] (the machine's own until something else is picked). */
    fun footer(machine: Machine, selected: String = current(machine)): CoreText = CoreText.of(
        when {
            selected == NONE -> if (machine.online) Key.EngineFooterNone else Key.EngineFooterNoneOffline
            machine.online -> Key.EngineFooter
            else -> Key.EngineFooterOffline
        },
    )

    /** What "Save" says once the hub took the choice. */
    fun saved(machine: Machine): CoreText = CoreText.of(if (machine.online) Key.EngineSwitched else Key.EngineSavedOffline)
}
