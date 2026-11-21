// @vitest-environment jsdom
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Machine } from "../../lib/types";
import { PortsProcessesPanel, type MachinePortsResponse, type MachineProcessesResponse } from "./PortsProcessesPanel";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

vi.hoisted(() => {
  const g = globalThis as Record<string, unknown>;
  g.matchMedia = () => ({ matches: false, addEventListener() {}, removeEventListener() {} });
});

const json = (v: unknown, status = 200) =>
  new Response(JSON.stringify(v), { status, headers: { "Content-Type": "application/json" } });

let portsReply: MachinePortsResponse;
let procsReply: MachineProcessesResponse;
let requests: { url: string; method: string; body?: unknown }[] = [];

function fakeFetch(input: RequestInfo | URL, init?: RequestInit): Promise<Response> {
  const url = String(input);
  const method = init?.method ?? "GET";
  const body = typeof init?.body === "string" ? JSON.parse(init.body) : undefined;
  requests.push({ url, method, body });
  if (url.includes("/ports")) return Promise.resolve(json(portsReply));
  if (url.includes("/processes/") && url.endsWith("/kill")) return Promise.resolve(json({ pid: 300, signal: body?.signal ?? "SIGTERM", sent: true }));
  if (url.includes("/processes")) return Promise.resolve(json(procsReply));
  return Promise.resolve(json({ error: "not found" }, 404));
}

const flush = async () => {
  for (let i = 0; i < 6; i++) {
    await act(async () => {
      await new Promise((r) => setTimeout(r, 0));
    });
  }
};

const click = (el: Element) =>
  act(() => {
    el.dispatchEvent(new MouseEvent("click", { bubbles: true, cancelable: true, button: 0 }));
  });

const setInput = (input: HTMLInputElement, value: string) =>
  act(() => {
    const setter = Object.getOwnPropertyDescriptor(HTMLInputElement.prototype, "value")?.set;
    setter?.call(input, value);
    input.dispatchEvent(new Event("input", { bubbles: true }));
    input.dispatchEvent(new Event("change", { bubbles: true }));
  });

let host: HTMLDivElement;
let root: Root;

const machines: Machine[] = [
  { id: "devbox", name: "Dev Box", online: true },
  { id: "offline-box", name: "Offline Box", online: false },
];

beforeEach(() => {
  requests = [];
  portsReply = {
    at: 1_760_000_000,
    supported: true,
    ports: [
      { protocol: "tcp", bind_addr: "127.0.0.1", port: 3000, scope: "loopback", pid: 301, user: "dev", process: "node", cmdline: "node server.js --token=[REDACTED]" },
      { protocol: "tcp", bind_addr: "0.0.0.0", port: 8080, scope: "all", pid: 302, user: "dev", process: "python3", cmdline: "python3 -m http.server 8080" },
    ],
  };
  procsReply = {
    at: 1_760_000_000,
    supported: true,
    current_user: "dev",
    total_count: 3,
    orphan_count: 1,
    processes: [
      {
        pid: 300,
        ppid: 1,
        uid: 1000,
        user: "dev",
        name: "hub.test",
        cmdline: "/tmp/go-build123/hub.test --token=[REDACTED]",
        cpu_percent: 42.5,
        rss_bytes: 256 * 1024 * 1024,
        uptime_seconds: 3720,
        state: "R",
        owned: true,
        likely_orphan: true,
        orphan_reasons: ["reparented_to_init", "test_process"],
      },
      {
        pid: 200,
        ppid: 1,
        uid: 1000,
        user: "dev",
        name: "personad",
        cmdline: "personad node",
        cpu_percent: 1.0,
        rss_bytes: 64 * 1024 * 1024,
        uptime_seconds: 86400,
        state: "S",
        owned: true,
        protected: true,
        protected_reason: "persona_node",
      },
      {
        pid: 400,
        ppid: 1,
        uid: 0,
        user: "root",
        name: "nginx",
        cmdline: "nginx: master",
        cpu_percent: 0.2,
        rss_bytes: 32 * 1024 * 1024,
        uptime_seconds: 86400,
        state: "S",
        owned: false,
      },
    ],
  };
  vi.stubGlobal("fetch", vi.fn(fakeFetch));
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});

afterEach(() => {
  act(() => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});

describe("PortsProcessesPanel", () => {
  it("is lazy-loaded from Machines.tsx via lazyPage without idle preloading", () => {
    const src = readFileSync(join(__dirname, "../Machines.tsx"), "utf8");
    expect(src).toMatch(/lazyPage\(\(\) => import\("\.\/machines\/PortsProcessesPanel"\)\.then\(\(m\) => m\.PortsProcessesPanel\)\)/);
    expect(src).not.toMatch(/PortsProcessesPanel.*idle:\s*true/);
  });

  it("loads listening ports on demand and renders redacted cmdline and bind scope", async () => {
    act(() => root.render(<PortsProcessesPanel machines={machines} />));
    await flush();

    expect(requests.map((r) => r.url)).toEqual(["/api/v1/machines/devbox/ports"]);
    expect(host.textContent).toContain(":3000");
    expect(host.textContent).toContain("127.0.0.1");
    expect(host.textContent).toContain(":8080");
    expect(host.textContent).toContain("0.0.0.0");
    expect(host.textContent).toContain("[REDACTED]");
  });

  it("highlights likely orphan processes, protects Persona/other-user PIDs, and enforces tiered SIGTERM/SIGKILL confirmation", async () => {
    act(() => root.render(<PortsProcessesPanel machines={machines} />));
    await flush();

    // Switch to Processes tab
    const tabs = [...host.querySelectorAll('[role="tab"]')];
    click(tabs[1]!);
    await flush();

    expect(requests.some((r) => r.url.startsWith("/api/v1/machines/devbox/processes?sort=cpu"))).toBe(true);
    const orphanRows = host.querySelectorAll('tr[data-orphan="true"]');
    expect(orphanRows.length).toBe(1);
    expect(orphanRows[0]!.textContent).toContain("hub.test");

    // Only the owned non-protected process (PID 300) should have Terminate & Force kill buttons.
    const actionButtons = [...host.querySelectorAll("tbody button")];
    expect(actionButtons.length).toBe(2);

    // 1. SIGTERM confirmation dialog
    click(actionButtons[0]!);
    await flush();
    const dialog = document.body.querySelector('[role="dialog"]')!;
    expect(dialog).not.toBeNull();
    expect(dialog.textContent).toContain("300");
    const confirmTerm = [...dialog.querySelectorAll("button")].find((b) => b.textContent?.includes("SIGTERM"))!;
    expect(confirmTerm.hasAttribute("disabled")).toBe(false);
    click(confirmTerm);
    await flush();

    const termReq = requests.find((r) => r.method === "POST" && r.url.endsWith("/processes/300/kill"));
    expect(termReq?.body).toEqual({ signal: "SIGTERM", confirm: true });

    // 2. SIGKILL tiered confirmation dialog: confirm button stays disabled until exact PID is typed.
    const killBtn = [...host.querySelectorAll("tbody button")][1]!;
    click(killBtn);
    await flush();
    const killDialog = document.body.querySelector('[role="dialog"]')!;
    const confirmKill = [...killDialog.querySelectorAll("button")].find((b) => b.textContent?.includes("SIGKILL"))!;
    expect((confirmKill as HTMLButtonElement).disabled).toBe(true);

    const pidInput = killDialog.querySelector("input")!;
    setInput(pidInput, "999");
    expect((confirmKill as HTMLButtonElement).disabled).toBe(true);

    setInput(pidInput, "300");
    expect((confirmKill as HTMLButtonElement).disabled).toBe(false);

    click(confirmKill);
    await flush();

    const killReqs = requests.filter((r) => r.method === "POST" && r.url.endsWith("/processes/300/kill"));
    expect(killReqs[1]?.body).toEqual({ signal: "SIGKILL", confirm: true, confirm_pid: 300 });
  });
});
