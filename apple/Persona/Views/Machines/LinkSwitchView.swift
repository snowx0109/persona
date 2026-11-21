import SwiftUI

/// The advanced machine-detail action for moving an eligible SSH computer to Persona Link (#10671).
/// An older hub sends no `link_switch`, so this draws nothing and leaves the existing page unchanged.
struct MachineLinkSwitchView: View {
    @Environment(AppModel.self) private var model
    let machine: Machine
    @State private var confirming = false
    @State private var checking = false

    var body: some View {
        let store = model.linkSwitches
        let selfHub = model.link(machine.group)?.hello.mesh?.first { $0.isSelf == true }?.hubID
        if MachineLinkSwitchStore.ownHub(machine, selfHubID: selfHub), let state = store.shown(machine), store.shouldShow(machine) {
            VStack(alignment: .leading, spacing: Theme.Space.m) {
                content(state, store: store)
                if let notice = store.notice(machine) {
                    Text(noticeText(notice))
                        .font(Theme.Font.secondary)
                        .foregroundStyle(Theme.danger)
                        .fixedSize(horizontal: false, vertical: true)
                        .accessibilityIdentifier("machine.link-switch.notice")
                }
                if store.canStart(machine) {
                    Button { confirming = true } label: {
                        HStack(spacing: Theme.Space.s) {
                            ThemedIcon(Theme.Icon.link)
                            Text("Switch to Persona Link…").buttonLabelLine()
                        }
                        .font(Theme.Font.labelStrong)
                        .frame(maxWidth: .infinity)
                    }
                    .themedButton()
                    .accessibilityIdentifier("machine.link-switch.start")
                }
                if store.needsStatusCheck(machine) {
                    Button {
                        guard !checking else { return }
                        checking = true
                        Task {
                            defer { checking = false }
                            await model.refresh()
                        }
                    } label: {
                        HStack(spacing: Theme.Space.s) {
                            if checking { ProgressView().controlSize(.small) }
                            Text("Check Status").buttonLabelLine()
                        }
                        .font(Theme.Font.labelStrong)
                        .frame(maxWidth: .infinity)
                    }
                    .themedButton()
                    .disabled(checking)
                    .accessibilityIdentifier("machine.link-switch.check")
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .card()
            // The shared presentation keeps the dialog through xxxLarge and uses an alert above it. The explicit
            // Cancel also stays visible in this long iOS 27 popover, where a cancel-role action was dropped (#10671).
            .accessibleConfirmationDialog(confirmationTitle, isPresented: $confirming, titleVisibility: .visible, hasCancel: true) {
                Button("Switch") { start(store) }
                Button("Cancel") { confirming = false }
            } message: {
                confirmationMessage
            }
        }
    }

    private var confirmationTitle: String { String(localized: "Switch “\(machine.name)” to Persona Link?") }
    private var confirmationMessage: Text {
        Text("Persona installs Persona Link over the current SSH connection. The computer then connects to the hub by itself, from any network.\n\nIt disconnects once. Agents in a terminal keep running; an assistant reply being written stops. Its name, settings and history stay.\n\nIf it fails, Persona tries to put it back on SSH.")
    }

    @ViewBuilder private func content(_ state: MachineLinkSwitch, store: MachineLinkSwitchStore) -> some View {
        if store.isStarting(machine) {
            HStack(spacing: Theme.Space.s) {
                ProgressView().controlSize(.small)
                Text("Starting…").font(Theme.Font.secondary)
            }
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("machine.link-switch.starting")
        } else if state.phase == "rolling_back" {
            VStack(alignment: .leading, spacing: Theme.Space.s) {
                HStack(spacing: Theme.Space.s) {
                    ProgressView().controlSize(.small)
                    Text("That didn't work. Putting the computer back on SSH…").font(Theme.Font.secondaryStrong)
                }
                if state.error != nil {
                    Text(MachineLinkSwitchText.reason(state.error, home: machine.home))
                        .font(Theme.Font.secondary).foregroundStyle(.secondary)
                }
            }
            .accessibilityIdentifier("machine.link-switch.rolling-back")
        } else if MachineLinkSwitchStore.running(state), let phase = state.phase {
            VStack(alignment: .leading, spacing: Theme.Space.m) {
                Text("Switching to Persona Link…").font(Theme.Font.secondaryStrong)
                    .accessibilityIdentifier("machine.link-switch.progress")
                let current = MachineLinkSwitchStore.steps.firstIndex(of: phase) ?? 0
                ForEach(Array(MachineLinkSwitchStore.steps.enumerated()), id: \.element) { index, step in
                    stepRow(step, index: index, current: current)
                }
            }
        } else if state.phase == "succeeded" {
            Label {
                Text("Switched to Persona Link. This computer now connects to the hub by itself.")
                    .font(Theme.Font.secondary).foregroundStyle(Theme.positive)
            } icon: {
                ThemedIcon(Theme.Icon.success).foregroundStyle(Theme.positive)
            }
            .accessibilityElement(children: .combine)
            .accessibilityIdentifier("machine.link-switch.succeeded")
        } else if state.phase == "failed" {
            VStack(alignment: .leading, spacing: Theme.Space.s) {
                Label {
                    Text(MachineLinkSwitchText.failure(state, home: machine.home))
                        .font(Theme.Font.secondary).foregroundStyle(Theme.danger)
                } icon: {
                    ThemedIcon(Theme.Icon.error).foregroundStyle(Theme.danger)
                }
                if !state.available, let reason = state.reason {
                    Text(MachineLinkSwitchText.reason(reason, home: machine.home))
                        .font(Theme.Font.label).foregroundStyle(.secondary)
                }
            }
            .accessibilityIdentifier("machine.link-switch.failed")
        } else if state.available {
            Text("Connected over SSH. With Persona Link, this computer connects to the hub by itself, also from other networks.")
                .font(Theme.Font.secondary).foregroundStyle(.secondary)
                .accessibilityIdentifier("machine.link-switch.intro")
        } else {
            Text(MachineLinkSwitchText.reason(state.reason, home: machine.home))
                .font(Theme.Font.secondary).foregroundStyle(.secondary)
                .accessibilityIdentifier("machine.link-switch.unavailable")
        }
    }

    private func stepRow(_ step: String, index: Int, current: Int) -> some View {
        Label {
            Text(MachineLinkSwitchText.step(step))
                .font(Theme.Font.label)
                .foregroundStyle(index <= current ? Color.primary : Color.secondary)
        } icon: {
            if index < current {
                ThemedIcon(Theme.Icon.success).foregroundStyle(Theme.positive)
            } else if index == current {
                ProgressView().controlSize(.small)
            } else {
                Text("\(index + 1)").font(Theme.Font.labelStrong.monospacedDigit()).foregroundStyle(Theme.accent)
            }
        }
        .accessibilityElement(children: .combine)
        .accessibilityIdentifier("machine.link-switch.step.\(step)")
        .accessibilityValue(index < current ? String(localized: "Done [Task Sheets f28461]")
                            : index == current ? String(localized: "Current step") : String(localized: "To do"))
    }

    private func noticeText(_ notice: MachineLinkSwitchStore.Notice) -> String {
        switch notice {
        case .refused(let code): MachineLinkSwitchText.reason(code, home: machine.home)
        case .forbidden: String(localized: "This device can't switch computers. Use a device with full access.")
        case .uncertain: String(localized: "Couldn't confirm that the switch started. If nothing changes here, try again.")
        }
    }

    private func start(_ store: MachineLinkSwitchStore) {
        guard let api = model.api(machine.group) else { return }
        let ref = machine.ref
        Task {
            await store.start(machine, userConfirmed: true, api: api) {
                await model.refresh()
                return model.machine(ref, inFront: false)
            }
        }
    }
}
