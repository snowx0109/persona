import SwiftUI

/// By App (#3756): the agents as someone working in the desktop apps knows them, by the app they run in. A section per
/// agent's desktop app (Claude's, the Codex App, then any other by name), then one per terminal engine an agent runs in
/// (herdr, Herdr App, Orca, Tern: the agents Persona and its assistant start there, #6567), then Other for every agent
/// outside both (the command line in another terminal, a Lark group's topic, a runtime, or a node that does not say).
/// Running and offline agents alike, the latest first, as the apps' sidebars list them (`AgentListOrder.recent`); an app
/// with no agent has no section. Which app is the node's `agent_surface`, as the agent's mark draws it (`AgentKind`);
/// which engine, the host the node placed it in (`space_kind`).
nonisolated struct AppGroup: Identifiable, Sendable {
    /// "app/desktop/<agent>", "app/engine/<engine>" or "app/other": the section's id and its fold key (`dev.folded`).
    var id: String
    /// The app: the agent's mark on its desktop app; nil for an engine's section and Other.
    var kind: AgentKind?
    /// The terminal engine of an engine's section; nil for a desktop app's and Other.
    var engine: MachineEngine? = nil
    var sessions: [Session]
    /// Its agents by project, as the desktop apps' sidebars list them: the latest project first (the Codex App's in its
    /// own order first), Other last.
    var projects: [AppProject] = []

    /// Its projects have headings: a desktop app's or an engine's section, unless every agent of it is in Other (then they
    /// are listed as before). Other's section is not by project: its agents are other terminals' and the like.
    var showsProjects: Bool { !projects.isEmpty && !(projects.count == 1 && projects[0].isOther) }

    /// The app as the rest of the list names it: "Claude desktop app" (a session's source), "Codex App" (the node's
    /// `agent_name`), else "<agent> desktop app"; "Other".
    var title: String {
        if let engine { return engine.name }
        return switch kind?.name {
        case nil: String(localized: "Other")
        case "Claude Code": String(localized: "Claude desktop app")
        case "Codex": String(localized: "Codex App")
        case let name?: String(localized: "\(name) desktop app", comment: "By App's section of another agent's desktop app: TRAE desktop app")
        }
    }
}

nonisolated enum AppGroups {
    static let otherID = "app/other"
    /// The desktop apps first, in this order; any other after them by name.
    private static let known = ["Claude Code", "Codex"]
    /// The terminal engines with a section of their own, in this order, after the desktop apps (`space_kind` as the node
    /// sends it: the host it placed the agent in).
    static let engines: [MachineEngine] = [.herdr, .herdrApp, .orca, .tern, .rex]

    /// The terminal engine an agent runs in, from the host the node placed it in; nil for none (a terminal or tmux pane
    /// outside the engines, a runtime, an agent the node did not place).
    static func engine(of s: Session) -> MachineEngine? {
        guard let kind = s.spaceKind else { return nil }
        return engines.first { $0.rawValue == kind }
    }

    static func build(_ sessions: [Session]) -> [AppGroup] {
        var desktop: [String: AppGroup] = [:]
        var byEngine: [String: AppGroup] = [:]
        var other: [Session] = []
        for s in sessions {
            let kind = AgentKind(s)
            guard kind.surface == .desktop else {
                if let engine = engine(of: s) {
                    let id = "app/engine/" + engine.rawValue
                    byEngine[id, default: AppGroup(id: id, kind: nil, engine: engine, sessions: [])].sessions.append(s)
                } else {
                    other.append(s)
                }
                continue
            }
            let id = "app/desktop/" + kind.name.lowercased()
            desktop[id, default: AppGroup(id: id, kind: kind, sessions: [])].sessions.append(s)
        }
        func rank(_ g: AppGroup) -> Int { known.firstIndex(of: g.kind?.name ?? "") ?? known.count }
        var groups = desktop.values.sorted { a, b in
            rank(a) != rank(b) ? rank(a) < rank(b) : a.id < b.id
        }
        groups += engines.compactMap { byEngine["app/engine/" + $0.rawValue] }
        if !other.isEmpty {
            groups.append(AppGroup(id: otherID, kind: nil, sessions: other))
        }
        return groups.map { group in
            var group = group
            group.sessions = AgentListOrder.recent(group.sessions)
            group.projects = group.id == otherID ? [] : projects(of: group)
            return group
        }
    }

    /// A desktop app's or an engine's section's agents by project: the repository a session runs in (`RepoPlace`: the node's, else its
    /// folder, a working tree's main checkout), named as its folder; a Codex App chat's project. An agent without one
    /// (a home directory, a Codex App chat in no project) is in Other. Projects go by their latest agent, the Codex App's in its
    /// own order first (`inAppOrder`), Other last.
    static func projects(of group: AppGroup) -> [AppProject] {
        var byKey: [String: AppProject] = [:]
        var order: [String] = []
        var other: [Session] = []
        for s in group.sessions { // the latest first already
            guard let (key, name) = project(of: s) else {
                other.append(s)
                continue
            }
            if byKey[key] == nil {
                // A fixed ending: a fold id that ended as some other fold's suffix would fold by default.
                byKey[key] = AppProject(id: group.id + "/project/" + key + "#", name: name, machine: s.machineRef, sessions: [])
                order.append(key)
            }
            byKey[key]?.sessions.append(s)
            if byKey[key]?.place == nil {
                byKey[key]?.place = place(of: s)
            }
        }
        var out = inAppOrder(order.compactMap { byKey[$0] })
        if !other.isEmpty {
            out.append(AppProject(id: group.id + "/project/other#", name: "", machine: nil, sessions: other))
        }
        return out
    }

    /// The projects the app numbers (`AppProject.place`) first, in its order; the others after them as they came (by
    /// their latest agent). Equal places keep their order: one order on every render.
    static func inAppOrder(_ projects: [AppProject]) -> [AppProject] {
        projects.enumerated().sorted { a, b in
            switch (a.element.place, b.element.place) {
            case let (x?, y?) where x != y: x < y
            case (.some, nil): true
            case (nil, .some): false
            default: a.offset < b.offset
            }
        }.map(\.element)
    }

    /// Where the Codex App's sidebar lists a chat's project: the node's `space_index`, from 1 down the App's own project
    /// order (`docs/integrations/codex.md`). nil for none, and for any other app's agent (herdr numbers its workspaces in
    /// the same field: not a desktop app's sidebar).
    static func place(of s: Session) -> Int? {
        guard s.spaceKind == "codex_app", let i = s.spaceIndex, i > 0 else { return nil }
        return i
    }

    /// Where an agent's project is (a key unique on its hub and machine) and its name; nil for none.
    static func project(of s: Session) -> (key: String, name: String)? {
        if s.spaceKind == "codex_app" {
            // The node names a chat's space by its repository (`codex-project:<root>`), a chat with no folder "Codex chats"
            // (`codex-chats:`). The App keeps a chat in no project in a folder of its own, <home>/Documents/Codex/<date>/
            // <name>: such a chat is in Other, as in the App's sidebar.
            guard let space = s.spaceID, !space.isEmpty, !space.hasPrefix("codex-chats:"), space != "codex-app:chats",
                  let name = s.spaceName?.trimmingCharacters(in: .whitespacesAndNewlines), !name.isEmpty else { return nil }
            if space.hasPrefix("codex-project:") {
                let root = String(space.dropFirst("codex-project:".count))
                if isHome(root) || isCodexChatFolder(root) { return nil }
            }
            return (s.machineRef.path + "/" + space, name)
        }
        let place = RepoPlace.of(s)
        var path = place.repo, name = place.name
        if isHome(path), !isHome(s.cwd) { // the home directory is a repository itself: the folder the session is in
            path = s.cwd
            name = ""
        }
        while path.count > 1 && (path.hasSuffix("/") || path.hasSuffix("\\")) { path.removeLast() }
        guard !path.isEmpty, !isHome(path) else { return nil }
        if name.isEmpty { name = (path.replacingOccurrences(of: "\\", with: "/") as NSString).lastPathComponent }
        return name.isEmpty ? nil : (s.machineRef.path + "/" + path, name)
    }

    /// The Codex App's folder for a chat in no project: <home>/Documents/Codex/<yyyy-mm-dd>/<name>.
    static func isCodexChatFolder(_ path: String) -> Bool {
        let parts = path.split(whereSeparator: { $0 == "/" || $0 == "\\" }).map(String.init)
        guard parts.count >= 4 else { return false }
        let n = parts.count
        let date = parts[n - 2]
        return parts[n - 4] == "Documents" && parts[n - 3] == "Codex" && date.count == 10 &&
            date.enumerated().allSatisfy { i, c in i == 4 || i == 7 ? c == "-" : c.isASCII && c.isNumber }
    }

    /// A home directory (or above one): no project of its own.
    static func isHome(_ path: String) -> Bool {
        let parts = path.split(whereSeparator: { $0 == "/" || $0 == "\\" }).map(String.init)
        if parts.isEmpty || path == "~" { return true }
        if parts.count == 1 { return parts[0] == "root" || parts[0] == "Users" || parts[0] == "home" || parts[0].hasSuffix(":") }
        if parts.count == 2 { return parts[0] == "Users" || parts[0] == "home" || (parts[0].hasSuffix(":") && parts[1] == "Users") }
        return parts.count == 3 && parts[0].hasSuffix(":") && parts[1] == "Users" // C:\Users\name
    }
}

/// A project of a By App section: the agents of one folder (or Codex App project); Other has no name.
nonisolated struct AppProject: Identifiable, Sendable {
    /// "<app id>/project/<hub and machine>/<path>", or "<app id>/project/other": its id and fold key (`dev.folded`).
    var id: String
    var name: String
    /// Its machine (a project is one folder on one machine); nil for Other.
    var machine: MachineRef?
    var sessions: [Session]
    /// Where its app's sidebar lists it, from 1 (`AppGroups.place`); nil when the app does not say.
    var place: Int? = nil

    var isOther: Bool { name.isEmpty }
    var title: String { isOther ? String(localized: "Other") : name }
}

/// A By App section's title, laid out as a machine's (`MachineSectionHeader`): the app's mark (a terminal for an engine's,
/// the agents symbol for Other) and name, its state counts, and how many agents it holds: offline ones too, which the state counts leave out.
struct AppSectionHeader: View {
    var app: AppGroup
    var counts: SpaceGroup
    @Environment(\.compactList) private var compact

    var body: some View {
        CountsHeading {
            if let kind = app.kind {
                AgentMark(kind: kind).accessibilityHidden(true) // the title names it
            } else { // a terminal for an engine's section, the agents symbol for Other
                ThemedIcon(app.engine != nil ? Theme.Icon.terminal : Theme.Icon.agents).foregroundStyle(.secondary)
                    .scaledFrame(width: Theme.Size.markS, height: Theme.Size.markS) // lined up with the marks above
            }
            if compact {
                Text(app.title).font(CompactList.sectionTitle).foregroundStyle(Color.primary).textCase(nil).lineLimit(1)
            } else {
                Text(app.title).font(Theme.Font.sectionHeader).foregroundStyle(.secondary).textCase(nil).lineLimit(1)
            }
        } counts: {
            StateCounts(group: counts)
            CountBadge(count: app.sessions.count, look: .tone(StatusPalette.idle))
        }
    }
}

/// A project's heading in a By App section, as the desktop apps' sidebars show a project above its sessions: its folder
/// (the agents symbol for Other), its name, how many agents it holds and the fold chevron. Tap to fold.
struct AppProjectRow: View {
    var project: AppProject
    var folded: Bool
    /// Its machine's name, when the section holds more than one machine's projects.
    var machine: String? = nil
    @Environment(\.compactList) private var compact

    var body: some View {
        HStack(spacing: Theme.Space.s) {
            ThemedIcon(project.isOther ? Theme.Icon.agents : Theme.Icon.folder)
                .font(compact ? CompactList.metaStrong : Theme.Font.secondaryMedium)
                .foregroundStyle(.secondary)
                .scaledFrame(width: compact ? CompactList.mark : Theme.Size.markS)
            Text(project.title)
                .font(compact ? CompactList.title : Theme.Font.secondaryMedium)
                .foregroundStyle(.secondary)
                .lineLimit(2)
            if let machine {
                Text(machine).font(compact ? CompactList.meta : Theme.Font.caption).foregroundStyle(.tertiary).lineLimit(1)
            }
            Spacer(minLength: Theme.Space.xs)
            CountBadge(count: project.sessions.count, look: .tone(StatusPalette.idle))
            FoldChevron(folded: folded)
        }
        .padding(.vertical, compact ? 0 : Theme.Space.xs)
        .contentShape(.rect)
        .accessibilityElement(children: .ignore)
        // As a section's heading reads (`FoldButton`): its name, folded or not, then what it holds.
        .accessibilityLabel([project.title, machine, folded ? String(localized: "Collapsed [Readiness Card ee6046]") : String(localized: "Expanded"),
                             String(localized: "\(project.sessions.count) agents")].compactMap { $0 }
            .joined(separator: String(localized: ", ", comment: "Separator between parts of an accessibility label")))
        .accessibilityAddTraits(.isButton)
    }
}
