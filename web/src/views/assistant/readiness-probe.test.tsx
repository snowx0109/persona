// @vitest-environment jsdom
// "Try it" (#1892): the probe's read waits past the hub's own limit for it, and a try that gets no answer says so on the
// card instead of ending silently, without an earlier try's result standing in for it.
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { HubTimeoutError, pastHub, READ_TIMEOUT_MS } from "../../lib/api";
import { PROBE_HUB_MS, refreshReadiness, resetReadiness, useReadiness, type Readiness, type ReadinessStep } from "../../lib/readiness";
import { getState } from "../../lib/store";
import { ReadinessCard, type ReadinessGate } from "./Readiness";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

// A hub that never answers: the request ends only when its signal aborts.
function hang(_url: string, init?: RequestInit): Promise<Response> {
  return new Promise((_resolve, reject) => {
    init?.signal?.addEventListener("abort", () => reject(new DOMException("aborted", "AbortError")), { once: true });
  });
}

const step: ReadinessStep = { id: "model", state: "problem", code: "auth_failed", title: "Fix the key", action: { id: "test_assistant" } };
const readiness = (probe?: Readiness["probe"]): Readiness => ({
  version: 1, mode: "setup", ready: false, next: "model", machine: null, launcher: "claude", steps: [step], dismissed: false,
  checked_at: "2026-11-09T09:00:00Z", ...(probe ? { probe } : {}),
});

let seen = { probing: false, probeError: "" };
function Probe() {
  const r = useReadiness();
  seen = { probing: r.probing, probeError: r.probeError };
  return null;
}

describe("Try it's read", () => {
  let host: HTMLDivElement;
  let root: Root;
  beforeEach(() => {
    vi.useFakeTimers();
    Object.assign(getState(), { features: ["assistant_readiness"] });
    host = document.createElement("div");
    root = createRoot(host);
  });
  afterEach(() => {
    act(() => root.unmount());
    vi.useRealTimers();
    vi.unstubAllGlobals();
    resetReadiness();
  });

  it("waits past the hub's own limit, then says the try got no answer", async () => {
    expect(PROBE_HUB_MS).toBeGreaterThanOrEqual(112_000); // the hub's 100 s check and 12 s self-check
    vi.stubGlobal("fetch", vi.fn(hang));
    resetReadiness(readiness({ ok: true, answer: "an earlier try" }));
    act(() => root.render(<Probe />));
    let done = false;
    act(() => void refreshReadiness({ probe: true }).then(() => (done = true)));
    expect(seen).toEqual({ probing: true, probeError: "" });
    // past a plain read's limit the try is still waiting for the hub
    await act(() => vi.advanceTimersByTimeAsync(READ_TIMEOUT_MS + 1));
    expect(done).toBe(false);
    expect(seen.probing).toBe(true);
    await act(() => vi.advanceTimersByTimeAsync(pastHub(PROBE_HUB_MS) - READ_TIMEOUT_MS));
    expect(done).toBe(true);
    expect(seen).toEqual({ probing: false, probeError: new HubTimeoutError().message });
  });

  it("clears a failed try's words when the next answer lands", async () => {
    vi.stubGlobal("fetch", vi.fn(() => Promise.reject(new TypeError("Failed to fetch"))));
    resetReadiness(readiness());
    act(() => root.render(<Probe />));
    await act(() => refreshReadiness({ probe: true }));
    expect(seen.probeError).not.toBe("");
    vi.stubGlobal("fetch", vi.fn(async () => new Response(JSON.stringify(readiness({ ok: true, answer: "hi" })), { status: 200, headers: { "Content-Type": "application/json" } })));
    await act(() => refreshReadiness({ probe: true }));
    expect(seen).toEqual({ probing: false, probeError: "" });
  });
});

describe("the card after a try", () => {
  const gate = (o: Partial<ReadinessGate>): ReadinessGate => ({
    readiness: readiness(), step, nudged: 0, help: () => {}, confirm: () => {}, probing: false, probeError: "", dialog: null, ...o,
  });

  it("says a try that got no answer failed, not an earlier try's result", () => {
    const html = renderToStaticMarkup(<ReadinessCard gate={gate({ readiness: readiness({ ok: true, answer: "an earlier try" }), probeError: "The hub took too long" })} />);
    expect(html).toContain("The hub took too long");
    expect(html).not.toContain("an earlier try");
  });

  it("shows the try's result when it got one, and nothing while it runs", () => {
    expect(renderToStaticMarkup(<ReadinessCard gate={gate({ readiness: readiness({ ok: true, answer: "pong" }) })} />)).toContain("pong");
    const running = renderToStaticMarkup(<ReadinessCard gate={gate({ readiness: readiness({ ok: true, answer: "pong" }), probing: true, probeError: "an-unseen-error" })} />);
    expect(running).not.toContain("pong");
    expect(running).not.toContain("an-unseen-error");
  });
});
