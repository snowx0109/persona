import SwiftUI

// MARK: - A new agent's own worktree (#6115)

// New Agent can start the agent in a Git worktree of its own (`worktree/add`, a branch persona/<its name>); closing the
// agent or deleting its chat then asks whether to keep or remove that worktree (`worktree/status`, `worktree/remove`).
// The same routes and rules as the web's `lib/worktree.ts`.

/// What `worktree/status` says of a folder (`proto.WorktreeStatus`). `path` is set for any folder in a repository (its
/// top level), `worktree` only for a linked worktree, `ours` only for one Persona made.
nonisolated struct WorktreeStatus: Sendable, Equatable {
    var worktree = false
    var ours = false
    var path = ""
    var branch = ""
    var root = ""
    var mainBranch = ""
    /// Where a new agent's worktree of this repository goes ("" from an older node).
    var checkouts = ""
    /// Changed files in it.
    var dirty = 0
    /// Commits on its branch that the repository's main branch lacks.
    var unmerged = 0
}

nonisolated extension WorktreeStatus: Decodable {
    enum CodingKeys: String, CodingKey { case worktree, ours, path, branch, root, checkouts, dirty, unmerged, mainBranch = "main_branch" }

    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        worktree = (try? c.decodeIfPresent(Bool.self, forKey: .worktree)) ?? false
        ours = (try? c.decodeIfPresent(Bool.self, forKey: .ours)) ?? false
        path = (try? c.decodeIfPresent(String.self, forKey: .path)) ?? ""
        branch = (try? c.decodeIfPresent(String.self, forKey: .branch)) ?? ""
        root = (try? c.decodeIfPresent(String.self, forKey: .root)) ?? ""
        mainBranch = (try? c.decodeIfPresent(String.self, forKey: .mainBranch)) ?? ""
        checkouts = (try? c.decodeIfPresent(String.self, forKey: .checkouts)) ?? ""
        dirty = (try? c.decodeIfPresent(Int.self, forKey: .dirty)) ?? 0
        unmerged = (try? c.decodeIfPresent(Int.self, forKey: .unmerged)) ?? 0
    }
}

/// What `worktree/add` made: the checkout the agent starts in.
nonisolated struct WorktreeAdded: Decodable, Sendable, Equatable {
    var path: String
    var branch: String
    var root: String?
}

/// What `worktree/remove` did.
nonisolated struct WorktreeRemoved: Decodable, Sendable, Equatable {
    var removed: Bool
    var branchDeleted: Bool
    enum CodingKeys: String, CodingKey { case removed, branchDeleted = "branch_deleted" }
}

nonisolated enum AgentWorktree: Sendable {
    /// The node's `agentBranchPrefix`.
    static let prefix = "persona/"
    /// The node's `maxSlug`.
    static let maxSlug = 40
    /// `worktree/remove` tries: the node may still list the agent for a moment after it closed (`worktree_in_use`).
    static let removeTries = 6

    /// The branch a new agent's worktree gets, as the node names it (`agentBranch`): persona/<its name as a slug, 40 at
    /// most>, "agent" when the name has no ASCII letters or digits.
    static func branch(nameHint: String) -> String {
        var slug = WorktreeForm.slug(nameHint)
        if slug.count > maxSlug {
            slug = String(slug.prefix(maxSlug))
            while slug.hasSuffix("-") { slug.removeLast() }
        }
        return prefix + (slug.isEmpty ? "agent" : slug)
    }

    /// What names the agent's branch: its first message, or its folder's name when the message has no ASCII letters or
    /// digits to name a branch with (a prompt in Chinese makes persona/<folder>, not persona/agent).
    static func nameHint(prompt: String, cwd: String) -> String {
        let p = prompt.trimmingCharacters(in: .whitespacesAndNewlines)
        if !WorktreeForm.slug(p).isEmpty { return p }
        var dir = cwd.trimmingCharacters(in: .whitespacesAndNewlines)
        while dir.hasSuffix("/") { dir.removeLast() }
        return dir.split(separator: "/").last.map(String.init) ?? ""
    }

    /// Where the checkout goes: `<checkouts>/<the branch as a slug>`; "" when the computer did not say.
    static func checkout(_ repo: WorktreeStatus, branch: String) -> String {
        WorktreeForm.suggestedPath(base: repo.checkouts, branch: branch)
    }

    /// What to do with the worktree a closed or deleted agent worked in: keep it (the default), or remove it, its branch
    /// too if asked; `confirmed`: the user turned on "I know these changes will be lost".
    struct Cleanup: Equatable, Sendable {
        var remove = false
        var deleteBranch = false
        var confirmed = false
    }

    /// What removing it loses: changed files, and with the branch, its commits the main branch lacks.
    static func loss(_ st: WorktreeStatus, deleteBranch: Bool) -> (dirty: Int, unmerged: Int) {
        (st.dirty, deleteBranch ? st.unmerged : 0)
    }

    /// Removing it would lose something: the confirmation switch shows, and the button waits for it.
    static func losing(_ st: WorktreeStatus, _ c: Cleanup) -> Bool {
        let l = loss(st, deleteBranch: c.deleteBranch)
        return c.remove && (l.dirty > 0 || l.unmerged > 0)
    }

    /// Nothing unconfirmed stands in the way of closing or deleting.
    static func ready(_ st: WorktreeStatus?, _ c: Cleanup) -> Bool {
        guard let st else { return true }
        return !losing(st, c) || c.confirmed
    }

    /// `force` only for losses the user confirmed.
    static func force(_ st: WorktreeStatus, _ c: Cleanup) -> Bool { losing(st, c) && c.confirmed }

    /// The status a close or delete offers the cleanup for: one Persona made; nil otherwise.
    static func cleanupOffered(_ st: WorktreeStatus?) -> WorktreeStatus? {
        guard let st, st.ours, !st.path.isEmpty else { return nil }
        return st
    }

    static func isInUse(_ error: Error) -> Bool { (error as? APIError)?.code == "worktree_in_use" }

    /// The engines that make a session's own worktree, as `worktree/status` and `worktree/remove` take them (`kind`).
    static let engines: Set<String> = ["herdr", "herdr-core", "orca", "tern", "rex", "paseo", "cindy"]

    /// The engine a session runs in, which made its own worktree (Paseo makes its own): its terminal's host, else the
    /// runtime engine its space is; nil when it names neither, and the computer's default decides.
    static func engine(of s: Session) -> String? {
        if let host = s.hostKind, engines.contains(host) { return host }
        if let kind = s.spaceKind, engines.contains(kind) { return kind }
        return nil
    }
}

extension APIClient {
    /// nil when the computer cannot say (an older node, offline, an error): no switch and no cleanup then. `kind`: the
    /// engine the agent ran in (`AgentWorktree.engine(of:)`); nil, the computer's default.
    func worktreeStatus(machine: String, cwd: String, kind: String? = nil, timeout: TimeInterval = 15) async -> WorktreeStatus? {
        var body = ["cwd": cwd]
        if let kind { body["kind"] = kind }
        return try? await post("/api/v1/machines/\(machine)/worktree/status", body: body, timeout: timeout, as: WorktreeStatus.self)
    }

    /// A worktree for a new agent from the repository `cwd` is in; an empty `branch` is the node's own pick. `kind`: the
    /// engine the agent opens in, which makes it; nil, the computer's default.
    func addAgentWorktree(machine: String, cwd: String, nameHint: String, branch: String, kind: String? = nil) async throws -> WorktreeAdded {
        var body = ["cwd": cwd]
        if !nameHint.isEmpty { body["name_hint"] = nameHint }
        let b = branch.trimmingCharacters(in: .whitespacesAndNewlines)
        if !b.isEmpty { body["branch"] = b }
        if let kind { body["kind"] = kind }
        return try await post("/api/v1/machines/\(machine)/worktree/add", body: body, timeout: 60, as: WorktreeAdded.self)
    }

    /// Removes it once its agent is gone, a few tries a second apart while the node still lists the agent; by the
    /// engine the agent ran in (`kind`).
    func removeAgentWorktree(machine: String, path: String, deleteBranch: Bool, force: Bool, kind: String? = nil,
                             tries: Int = AgentWorktree.removeTries, wait: Duration = .seconds(1)) async throws -> WorktreeRemoved {
        struct Body: Encodable {
            var path: String
            var delete_branch: Bool?
            var force: Bool?
            var kind: String?
        }
        let body = Body(path: path, delete_branch: deleteBranch ? true : nil, force: force ? true : nil, kind: kind)
        var attempt = 1
        while true {
            do {
                return try await post("/api/v1/machines/\(machine)/worktree/remove", body: body, timeout: 60, as: WorktreeRemoved.self)
            } catch where attempt < tries && AgentWorktree.isInUse(error) {
                attempt += 1
                try await Task.sleep(for: wait)
            }
        }
    }
}

extension AppModel {
    /// The worktree Persona made that this agent works in, when its computer says so; nil otherwise (and when it is
    /// offline or cannot say).
    func ourWorktree(_ s: Session, timeout: TimeInterval = 15) async -> WorktreeStatus? {
        guard !s.cwd.isEmpty, machine(s.machineRef)?.online == true, let api = api(s.key.group) else { return nil }
        return AgentWorktree.cleanupOffered(await api.worktreeStatus(machine: s.machineID, cwd: s.cwd, kind: AgentWorktree.engine(of: s),
                                                                     timeout: timeout))
    }

    /// After its agent was closed or its chat deleted: removes the worktree when the user asked, by the engine the agent
    /// ran in (`kind`, `AgentWorktree.engine(of:)`), and says how it went.
    func cleanUpWorktree(_ machine: MachineRef, _ st: WorktreeStatus, _ c: AgentWorktree.Cleanup, kind: String? = nil) async {
        guard c.remove, !st.path.isEmpty, let api = api(machine.group) else { return }
        do {
            let r = try await api.removeAgentWorktree(machine: machine.id, path: st.path, deleteBranch: c.deleteBranch,
                                                      force: AgentWorktree.force(st, c), kind: kind)
            toast = Toast(r.branchDeleted ? String(localized: "Worktree and branch removed [Agent Worktree]")
                                          : String(localized: "Worktree removed [Agent Worktree]"), style: .success)
        } catch {
            toast = Toast(String(localized: "Couldn't remove the worktree: \(error.shown ?? error.localizedDescription) [Agent Worktree]"), style: .error)
            Haptics.error()
        }
    }
}

// MARK: - Views

/// New Agent's switch, for a folder in a Git repository: a branch and a checkout of its own. The branch can be typed;
/// empty, it is the one the computer picks (shown as the placeholder).
struct NewAgentWorktreeSection: View {
    var repo: WorktreeStatus
    @Binding var on: Bool
    @Binding var branch: String
    /// The branch the computer picks when none is typed.
    var suggested: String
    var home: String?

    var body: some View {
        Section {
            Toggle(isOn: $on.animation(Theme.Motion.standard)) {
                Label { Text("In a New Worktree [Agent Worktree]") } icon: {
                    ThemedIcon(Theme.Icon.gitBranch).foregroundStyle(Theme.accent)
                }
            }
            .tint(Theme.accent)
            .accessibilityIdentifier("newChat.worktree")
            if on {
                LabeledContent {
                    TextField(suggested, text: $branch)
                        .font(Theme.Font.code)
                        .multilineTextAlignment(.trailing)
                        .autocorrectionDisabled()
                        #if os(iOS)
                        .textInputAutocapitalization(.never)
                        #endif
                        .accessibilityIdentifier("newChat.worktree.branch")
                } label: { Text("Branch [Agent Worktree]") }
            }
        } header: { Text("Worktree [Agent Worktree]") } footer: {
            VStack(alignment: .leading, spacing: Theme.Space.xs) {
                Text("A branch and a checkout of its own: the agent changes code there, and this folder stays as it is. [Agent Worktree]")
                let path = AgentWorktree.checkout(repo, branch: branch.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ? suggested : branch)
                if on, !path.isEmpty {
                    Text("Checks out to \(Fmt.path(path, home: home)) [Agent Worktree]")
                        .font(Theme.Font.caption.monospaced())
                        .lineLimit(2)
                        .truncationMode(.head)
                        .accessibilityIdentifier("newChat.worktree.path")
                }
            }
        }
    }
}

/// In Close Agent and Delete Chat when the agent worked in a worktree Persona made: keep it (the default) or remove it,
/// its branch too, and what removing it loses, said with the counts and to be confirmed.
struct WorktreeCleanupSection: View {
    var status: WorktreeStatus
    @Binding var cleanup: AgentWorktree.Cleanup
    var home: String?

    var body: some View {
        let loss = AgentWorktree.loss(status, deleteBranch: cleanup.deleteBranch)
        VStack(alignment: .leading, spacing: Theme.Space.m) {
            Label {
                VStack(alignment: .leading, spacing: Theme.Space.xxs) {
                    Text("This agent worked in a worktree Persona made [Agent Worktree]").font(Theme.Font.labelStrong)
                    if !status.branch.isEmpty {
                        Text(status.branch).font(Theme.Font.caption.monospaced()).foregroundStyle(.secondary).lineLimit(1)
                    }
                    Text(Fmt.path(status.path, home: home))
                        .font(Theme.Font.caption.monospaced())
                        .foregroundStyle(.secondary)
                        .lineLimit(1)
                        .truncationMode(.head)
                }
            } icon: {
                ThemedIcon(Theme.Icon.gitBranch).font(Theme.Font.secondary).foregroundStyle(.secondary).scaledFrame(width: Theme.Size.iconS)
            }
            Picker(String(localized: "Worktree [Agent Worktree]"), selection: Binding(get: { cleanup.remove }, set: { remove in
                withAnimation(Theme.Motion.standard) { cleanup = .init(remove: remove, deleteBranch: cleanup.deleteBranch) }
            })) {
                Text("Keep Worktree [Agent Worktree]").tag(false)
                Text("Remove [Agent Worktree]").tag(true)
            }
            .pickerStyle(.segmented)
            .labelsHidden()
            .accessibilityIdentifier("worktree.cleanup")
            if cleanup.remove {
                if !status.branch.isEmpty {
                    Toggle(isOn: Binding(get: { cleanup.deleteBranch }, set: { cleanup.deleteBranch = $0; cleanup.confirmed = false })) {
                        Text("Delete the branch \(status.branch) too [Agent Worktree]").font(Theme.Font.secondary)
                    }
                    .tint(Theme.danger)
                    .accessibilityIdentifier("worktree.deleteBranch")
                }
                if loss.dirty > 0 {
                    warning(String(localized: "\(loss.dirty) files have uncommitted changes. Removing the worktree loses them. [Agent Worktree]"))
                }
                if loss.unmerged > 0 {
                    warning(String(localized: "The branch has \(loss.unmerged) commits that aren't on \(status.mainBranch.isEmpty ? "main" : status.mainBranch). Deleting the branch loses them. [Agent Worktree]"))
                }
                if AgentWorktree.losing(status, cleanup) {
                    Toggle(isOn: $cleanup.confirmed) {
                        Text("I understand these changes will be lost [Agent Worktree]").font(Theme.Font.secondaryMedium)
                    }
                    .tint(Theme.danger)
                    .accessibilityIdentifier("worktree.confirmLoss")
                }
            }
        }
        .padding(Theme.Space.l)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(Theme.Fill.well, in: .rect(cornerRadius: Theme.Radius.card, style: .continuous))
    }

    private func warning(_ text: String) -> some View {
        Text(text).font(Theme.Font.caption).foregroundStyle(Theme.danger).fixedSize(horizontal: false, vertical: true)
    }
}
