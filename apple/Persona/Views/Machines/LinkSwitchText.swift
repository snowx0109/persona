import Foundation

/// Plain words for the hub's Link-switch codes. The codes stay off the screen; an unknown one gets a safe generic line.
nonisolated enum MachineLinkSwitchText {
    private static let knownCodes: Set<String> = ["offline", "unsupported_os", "link_unavailable", "hub_old", "pending",
        "custom_node", "busy", "in_progress", "link_switch_in_progress", "no_systemd", "linger_off", "existing_link", "no_node", "unsafe_permissions",
        "no_package", "hub_unanswered", "transfer_failed", "install_failed", "link_timeout", "config_changed", "interrupted", "unknown"]

    static func step(_ phase: String) -> String {
        switch phase {
        case "checking": return String(localized: "Checking the computer")
        case "installing": return String(localized: "Installing Persona Link")
        case "connecting": return String(localized: "Waiting for it to connect")
        default: return phase
        }
    }

    /// The account a Linux computer's node runs as, from its home folder (/home/<user>, /root).
    static func homeUser(_ home: String?) -> String? {
        home?.split(separator: "/").last.map(String.init)
    }

    static func reason(_ code: String?, home: String? = nil) -> String {
        switch code {
        case "offline": return String(localized: "The computer needs to be online to switch.")
        case "unsupported_os": return String(localized: "Switching from here works for Linux computers. On a Mac or Windows computer, install Persona Link on it yourself.")
        case "link_unavailable": return String(localized: "The hub doesn't take Persona Link connections yet. Turn on its TLS port or give it an https address first, under Network & access on the hub's admin page.")
        case "hub_old": return String(localized: "The hub needs an update before it can switch computers.")
        case "pending": return String(localized: "Confirm that this computer is yours first.")
        case "custom_node": return String(localized: "Persona runs from a custom location on this computer, so it can't be switched from here.")
        case "unsafe_permissions": return String(localized: "Persona's folders on this computer aren't safe to switch from here: the home folder, ~/.persona or ~/.persona/bin belongs to another user or can be changed by other users. On that computer, make them yours and writable only by you (for example: chmod go-w ~ ~/.persona ~/.persona/bin), then try again.")
        case "hub_unanswered": return String(localized: "The hub didn't answer for an hour, so the switch couldn't be undone.")
        case "busy": return String(localized: "Persona is updating this computer. Try again once that's done.")
        case "in_progress", "link_switch_in_progress": return String(localized: "This computer is already switching to Persona Link.")
        case "no_systemd": return String(localized: "This computer has no systemd user services, which Persona Link needs to keep running.")
        case "linger_off":
            let user = homeUser(home) ?? String(localized: "<user>")
            let command = "sudo loginctl enable-linger \(user)"
            return String(localized: "Its services would stop whenever you sign out. An administrator of that computer must run \(command) first.")
        case "existing_link": return String(localized: "Persona Link is already installed or paired on this computer. Finish setting it up there, or remove it first.")
        case "no_node": return String(localized: "Persona couldn't find its program on this computer in the usual place.")
        case "no_package": return String(localized: "The hub has no Persona Link build for this computer's system.")
        case "transfer_failed": return String(localized: "Couldn't send Persona Link to the computer over SSH.")
        case "install_failed": return String(localized: "Persona Link couldn't be installed or paired on the computer.")
        case "link_timeout": return String(localized: "The computer didn't connect through Persona Link in time.")
        case "config_changed": return String(localized: "The computer's settings changed during the switch.")
        case "interrupted": return String(localized: "Persona restarted during the switch.")
        case "unknown": return String(localized: "Something went wrong.")
        default: return String(localized: "This computer can't switch to Persona Link right now.")
        }
    }

    static func failure(_ state: MachineLinkSwitch, home: String?) -> String {
        let reason = reason(knownCodes.contains(state.error ?? "") ? state.error : "unknown", home: home)
        if state.rollbackUnconfirmed == true {
            return String(localized: "Couldn't switch to Persona Link. \(reason) Persona couldn't confirm that the switch was undone. Check that the computer is reachable; if it isn't, reconnect it from the computer itself.")
        }
        return String(localized: "Couldn't switch to Persona Link. \(reason) It's back on SSH.")
    }
}
