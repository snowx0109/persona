import { describe, expect, it } from "vitest";
import {
  canSaveEngine,
  canSelectEngine,
  currentEngine,
  detectionText,
  engineDetail,
  engineFooter,
  engineName,
  engineOptions,
  engineSaveError,
  KNOWN_ENGINES,
  alsoOpenText,
  currentHosts,
  ENGINE_HOSTS,
  hostOptions,
  hostsFooter,
  hostsName,
  normalizeHosts,
  sameHosts,
  startHosts,
  supportsHostSets,
  toggleHost,
} from "./engines";
import type { Machine } from "./types";

const machine = (extra: Partial<Machine> = {}): Machine => ({ id: "computer", name: "电脑", online: false, ...extra });

describe("engine choice (iOS EngineSelectionModel)", () => {
  it("is herdr until one is chosen, whatever happens to be detected", () => {
    expect(currentEngine(machine())).toBe("herdr");
    expect(currentEngine(machine({ engines: [{ id: "herdr-core", name: "Herdr App", available: true }] }))).toBe("herdr");
    expect(currentEngine(machine({ engine: "herdr-core" }))).toBe("herdr-core");
  });

  it("lists what the node reports, names, notes and future engines included", () => {
    const m = machine({
      online: true,
      engine: "orca",
      engines: [
        { id: "orca", name: "Orca Desktop", available: false, note: "请先更新 Orca" },
        { id: "future", name: "新引擎", available: true },
      ],
    });
    expect(engineOptions(m)).toBe(m.engines);
    expect(engineName("orca", m)).toBe("Orca Desktop");
    expect(engineName("future", m)).toBe("新引擎");
    expect(canSelectEngine("orca", m)).toBe(true);
    expect(canSelectEngine("future", m)).toBe(true);
    m.engines![1].available = false;
    expect(canSelectEngine("future", m)).toBe(false);
  });

  it("lists every known engine when the node reports none (offline, older hubs)", () => {
    const m = machine({ engine: "future" });
    expect(engineOptions(m).map((e) => e.id)).toEqual(["herdr", "herdr-core", "herdr-both", "orca", "tern", "rex", "paseo", "cindy"]);
    expect(engineOptions(m).every((e) => !e.available)).toBe(true);
    expect(engineOptions({ engines: [] }).length).toBe(KNOWN_ENGINES.length);
    // known ones stay selectable offline; an unknown one only while reported available
    for (const e of KNOWN_ENGINES) expect(canSelectEngine(e.id, m)).toBe(true);
    expect(canSelectEngine("future", m)).toBe(false);
    expect(engineName("future", m)).toBe("future");
  });

  it("names and describes the known engines like iOS", () => {
    expect(engineName("herdr-both")).toBe("herdr + Herdr App");
    expect(engineDetail("herdr-both")).toBe("终端和桌面 App 的会话都显示");
    expect(engineName("paseo")).toBe("Paseo");
    expect(engineDetail("paseo")).toBe("Paseo 里的 Agent");
    expect(engineName("tern")).toBe("Tern");
    expect(engineDetail("tern")).toBe("Tern 里的终端");
    expect(engineName("rex")).toBe("Rex");
    expect(engineDetail("rex")).toBe("Rex 里的终端");
    expect(engineName("cindy")).toBe("Cindy");
    expect(engineDetail("cindy")).toBe("Cindy 里的 Agent");
    expect(engineDetail("future")).toBeUndefined();
  });

  it("says what the node found", () => {
    const engines = [
      { id: "herdr", name: "herdr", available: true, version: "0.9.5" },
      { id: "herdr-core", name: "Herdr App", available: true },
      { id: "orca", name: "Orca", available: false, version: "1.0" },
      { id: "tern", name: "Tern", available: true, version: "0.4.1" },
    ];
    const on = machine({ online: true, engines });
    expect(detectionText("herdr", on)).toBe("检测到可用 · 0.9.5");
    expect(detectionText("herdr-core", on)).toBe("检测到可用");
    expect(detectionText("orca", on)).toBe("没检测到");
    expect(detectionText("tern", on)).toBe("检测到可用 · 0.4.1");
    expect(detectionText("paseo", on)).toBe("还没有检测结果");
    expect(detectionText("herdr", machine({ engines }))).toBe("电脑离线，没有检测结果");
  });

  it("offers 保存 only for a change the hub can take", () => {
    const m = machine({ engine: "herdr-core" });
    expect(canSaveEngine("herdr-core", m)).toBe(false);
    expect(canSaveEngine("herdr", m)).toBe(true);
    expect(canSaveEngine("future", m)).toBe(false);
    expect(canSaveEngine("herdr", machine())).toBe(false);
  });

  it("explains the choice, offline included, and a failed save", () => {
    expect(engineFooter("herdr", true)).toBe("只显示所选引擎里的会话。");
    expect(engineFooter("orca", false)).toBe("机器离线，选择会在连接后生效。只显示所选引擎里的会话。");
    expect(engineFooter("herdr-both", true)).toContain("从网页新建的会话仍开在 herdr 里");
    expect(engineFooter("herdr-both", false)).toBe("机器离线，选择会在连接后生效。同时显示两个引擎里的会话。");
    expect(engineSaveError("暂时无法保存")).toBe("保存失败：暂时无法保存");
  });
});

describe("engine host sets (#2576)", () => {
  it("reads the hub's set, else the one the single engine stands for", () => {
    expect(supportsHostSets(machine())).toBe(false);
    expect(currentHosts(machine({ engine: "herdr-both" }))).toEqual({ hosts: ["herdr", "herdr-core"], default: "herdr" });
    expect(currentHosts(machine({ engine: "orca" }))).toEqual({ hosts: ["orca"], default: "orca" });
    expect(currentHosts(machine({ engine: "tern" }))).toEqual({ hosts: ["tern"], default: "tern" });
    expect(currentHosts(machine())).toEqual({ hosts: ["herdr"], default: "herdr" });
    const m = machine({ engine: "orca", engine_hosts: ["herdr", "orca"], engine_default: "orca" });
    expect(supportsHostSets(m)).toBe(true);
    expect(currentHosts(m)).toEqual({ hosts: ["herdr", "orca"], default: "orca" });
    expect(currentHosts(machine({ engine_hosts: [], engine_default: "" }))).toEqual({ hosts: [], default: "" });
  });

  it("toggles hosts in canonical order and keeps the default enabled", () => {
    let c = { hosts: ["herdr"], default: "herdr" };
    c = toggleHost(c, "orca");
    expect(c).toEqual({ hosts: ["herdr", "orca"], default: "herdr" });
    c = toggleHost(c, "herdr"); // the default goes: the first one left
    expect(c).toEqual({ hosts: ["orca"], default: "orca" });
    c = toggleHost(c, "orca"); // none
    expect(c).toEqual({ hosts: [], default: "" });
    expect(normalizeHosts(["paseo", "herdr", "nope"], "paseo")).toEqual({ hosts: ["herdr", "paseo"], default: "paseo" });
    // Tern goes between Orca and Paseo (#2154), and may be the default
    expect(toggleHost({ hosts: ["herdr", "paseo"], default: "herdr" }, "tern")).toEqual({ hosts: ["herdr", "tern", "paseo"], default: "herdr" });
    expect(normalizeHosts(["paseo", "tern", "orca"], "tern")).toEqual({ hosts: ["orca", "tern", "paseo"], default: "tern" });
    expect(sameHosts({ hosts: ["herdr"], default: "herdr" }, { hosts: ["herdr"], default: "herdr" })).toBe(true);
    expect(sameHosts({ hosts: ["herdr", "orca"], default: "herdr" }, { hosts: ["herdr", "orca"], default: "orca" })).toBe(false);
  });

  it("offers the set's members, not the combined engine, in canonical order", () => {
    const m = machine({
      online: true,
      engines: [
        { id: "paseo", name: "Paseo", available: false },
        { id: "tern", name: "Tern", available: true, version: "0.4.1" },
        { id: "orca", name: "Orca", available: true },
        { id: "herdr-both", name: "herdr + Herdr App", available: false },
        { id: "herdr", name: "herdr", available: true },
      ],
    });
    expect(hostOptions(m).map((e) => e.id)).toEqual(["herdr", "orca", "tern", "paseo"]);
    const offline = hostOptions(machine());
    expect(offline.map((e) => e.id)).toEqual(ENGINE_HOSTS);
    expect(offline.map((e) => e.name)).toEqual(["herdr", "Herdr App", "Orca", "Tern", "Rex", "Paseo", "Cindy"]);
  });

  it("offers every enabled host to start in", () => {
    expect(startHosts(machine({ engine_hosts: ["herdr", "herdr-core", "orca"], engine_default: "herdr" }))).toEqual(["herdr", "herdr-core", "orca"]);
    expect(startHosts(machine({ engine: "herdr-both" }))).toEqual(["herdr", "herdr-core"]);
    expect(startHosts(machine({ engine_hosts: ["herdr-core", "tern"], engine_default: "tern" }))).toEqual(["herdr-core", "tern"]);
  });

  it("names the other hosts a session is open in", () => {
    expect(alsoOpenText({})).toBe("");
    expect(alsoOpenText({ also_open_in: ["orca"] })).toContain("Orca");
    expect(alsoOpenText({ also_open_in: ["tern"] })).toContain("Tern");
    expect(hostsFooter({ hosts: [], default: "" }, true)).toContain("没有启用终端引擎");
    expect(hostsFooter({ hosts: ["herdr", "orca"], default: "orca" }, false)).toContain("Orca");
    expect(hostsFooter({ hosts: ["herdr", "tern"], default: "tern" }, true)).toContain("Tern");
  });

  it("names a set by its engines, for the header's engine pill", () => {
    expect(hostsName({ hosts: ["herdr", "orca"], default: "herdr" })).toBe("herdr + Orca");
    expect(hostsName({ hosts: ["orca"], default: "orca" })).toBe("Orca");
    expect(hostsName({ hosts: ["orca", "tern"], default: "tern" })).toBe("Orca + Tern");
    expect(hostsName({ hosts: [], default: "" })).not.toBe("");
  });
});
