"""Opt-in SSH to Persona Link switch scenarios for Apple screenshot and runtime verification (#10671).

MOCK_LINK_SWITCH=1 turns the synthetic `server` fixture into an eligible Linux SSH computer. It is off by default and
leaves every existing mock screen unchanged. GET /mock/link_switch?state=<state> selects a fixture and pushes the full
machine list; with no query it reports the state and POST attempt count. `hold` keeps the start answer out until
`release=1`, and `uncertain`, `refused` and `forbidden` exercise the three answer classes without a second operation.
"""
import os
import threading

MODE = os.environ.get("MOCK_LINK_SWITCH", "")
MID = "server"
HUB = None

TIMES = {"available": 1, "checking": 5, "installing": 9, "connecting": 14, "rolling_back": 20,
         "failed": 30, "unconfirmed": 30, "succeeded": 40, "refused": 1, "forbidden": 1, "uncertain": 1,
         "hold": 1, "unavailable": 1}


def at(second):
    return "2026-11-19T08:00:%02dZ" % second


def wire(state):
    if state == "old":
        return None
    if state in ("available", "refused", "forbidden", "uncertain", "hold"):
        return {"available": True, "at": at(TIMES[state])}
    if state == "unavailable":
        return {"available": False, "reason": "linger_off", "at": at(1)}
    if state in ("checking", "installing", "connecting"):
        return {"available": False, "reason": "in_progress", "phase": state, "at": at(TIMES[state])}
    if state == "rolling_back":
        return {"available": False, "phase": "rolling_back", "error": "link_timeout", "at": at(20)}
    if state == "failed":
        return {"available": True, "phase": "failed", "error": "link_timeout", "at": at(30)}
    if state == "unconfirmed":
        return {"available": False, "reason": "offline", "phase": "failed", "error": "hub_unanswered",
                "rollback_unconfirmed": True, "at": at(30)}
    if state == "succeeded":
        return {"available": False, "phase": "succeeded", "at": at(40)}
    raise ValueError("unknown Link-switch state %r" % state)


class LinkSwitch:
    def __init__(self, hub):
        self.hub = hub
        self.state = "available"
        self.attempts = 0
        self.lock = threading.Lock()
        self.released = threading.Event()
        self.released.set()
        self.machine = next(m for m in hub.MACHINES if m["id"] == MID)
        self.machine.update(name="Build Server", kind="ssh", ssh_host="build-server", os="linux", arch="amd64",
                            home="/home/builder", online=True)
        self.machine.pop("error", None)
        self.machine.pop("error_code", None)
        self.apply("available", push=False)

    def apply(self, state, push=True):
        if state not in TIMES and state != "old":
            raise ValueError(state)
        with self.lock:
            self.state = state
            value = wire(state)
            if value is None:
                self.machine.pop("link_switch", None)
            else:
                self.machine["link_switch"] = value
            self.machine["kind"] = "link" if state == "succeeded" else "ssh"
            self.machine["online"] = state not in ("connecting", "rolling_back", "unconfirmed")
            if self.machine["online"]:
                self.machine.pop("error", None)
                self.machine.pop("error_code", None)
            else:
                self.machine["error_code"] = "link_offline"
                self.machine["error"] = "synthetic expected disconnect"
        if push:
            self.hub.broadcast({"type": "machines", "machines": self.hub.out_machines()})

    def control(self, h, req):
        state = (req.query.get("state") or [""])[0]
        if state:
            try:
                self.apply(state)
            except ValueError:
                return h.send_json({"error": "unknown state", "state": state}, 400)
        if (req.query.get("release") or [""])[0] == "1":
            self.released.set()
        if (req.query.get("reset") or [""])[0] == "1":
            with self.lock:
                self.attempts = 0
        with self.lock:
            answer = {"state": self.state, "attempts": self.attempts, "link_switch": wire(self.state)}
        return h.send_json(answer)

    def start(self, h, req):
        if req.match["mid"] != MID:
            return False
        with self.lock:
            self.attempts += 1
            state = self.state
        if state == "old":
            return h.send_json({"error": "not found", "code": "no_machine"}, 404)
        if state == "forbidden":
            return h.send_json({"error": "synthetic full-device refusal", "code": "forbidden"}, 403)
        if state == "refused":
            return h.send_json({"error": "synthetic offline refusal", "code": "offline"}, 409)
        if state == "uncertain":
            return h.send_json({"error": "synthetic lost answer", "code": "internal"}, 503)
        if state == "hold":
            self.released.clear()
            if not self.released.wait(timeout=120):
                return h.send_json({"error": "synthetic held-answer timeout", "code": "internal"}, 503)
        accepted = wire("checking")
        self.machine["link_switch"] = accepted
        return h.send_json({"link_switch": accepted}, 202)

    def reset(self):
        self.released.set()
        with self.lock:
            self.attempts = 0
        self.machine = next(m for m in self.hub.MACHINES if m["id"] == MID)
        self.machine.update(name="Build Server", kind="ssh", ssh_host="build-server", os="linux", arch="amd64",
                            home="/home/builder", online=True)
        self.machine.pop("error", None)
        self.machine.pop("error_code", None)
        self.apply("available")


def register(d):
    d.help("MOCK_LINK_SWITCH=1  eligible Linux SSH server plus /mock/link_switch state and start-answer controls (#10671)")
    if not MODE:
        return
    global HUB
    HUB = d.hub
    switches = LinkSwitch(d.hub)
    d.route("GET", "/mock/link_switch", switches.control)
    d.route("POST", r"/api/v1/machines/(?P<mid>[^/]+)/link-switch", switches.start)
    d.on_reset(switches.reset)
