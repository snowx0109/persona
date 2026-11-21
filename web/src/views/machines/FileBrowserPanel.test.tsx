// @vitest-environment jsdom
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { Machine } from "../../lib/types";

vi.hoisted(() => {
  const g = globalThis as Record<string, unknown>;
  g.matchMedia = () => ({ matches: false, addEventListener() {}, removeEventListener() {} });
  const kept = new Map<string, string>();
  g.localStorage = {
    getItem: (k: string) => kept.get(k) ?? null,
    setItem: (k: string, v: string) => void kept.set(k, v),
    removeItem: (k: string) => void kept.delete(k),
  };
});

import { setToken } from "../../lib/api";
import { FileBrowserPanel, machineFileDownloadURL } from "./FileBrowserPanel";

(globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;

const json = (v: unknown, status = 200) => new Response(JSON.stringify(v), { status, headers: { "Content-Type": "application/json" } });

let host: HTMLDivElement;
let root: Root;
let calls: string[];
let tailCalls = 0;

const flush = async () => {
  for (let i = 0; i < 6; i++) {
    await act(async () => {
      await Promise.resolve();
    });
  }
};

function stubHub(input: RequestInfo | URL): Promise<Response> {
  const url = String(input);
  calls.push(url);
  if (url.includes("/files/roots")) {
    return Promise.resolve(json({
      home: "/home/dev",
      os: "linux",
      roots: [
        { id: "home", label: "$HOME", path: "/home/dev", kind: "home", exists: true },
        { id: "ws:1", label: "persona", path: "/home/dev/persona", kind: "workspace", exists: true },
        { id: "cfg:1", label: ".config", path: "/home/dev/.config", kind: "config", exists: true },
      ],
    }));
  }
  if (url.includes("/files/list")) {
    return Promise.resolve(json({
      path: "/home/dev",
      canonical_path: "/home/dev",
      parent: "/home",
      total: 4,
      entries: [
        { name: "logs", path: "/home/dev/logs", kind: "dir", size: 4096, mode: "drwxr-xr-x", perm: 0o755, mtime: "2026-11-20T00:00:00Z" },
        { name: ".env", path: "/home/dev/.env", kind: "file", size: 48, mode: "-rw-------", perm: 0o600, mtime: "2026-11-20T00:00:00Z", sensitive: true, sensitive_reason: "env" },
        { name: "app.log", path: "/home/dev/app.log", kind: "file", size: 600000, mode: "-rw-r--r--", perm: 0o644, mtime: "2026-11-20T00:00:00Z" },
        { name: "image.bin", path: "/home/dev/image.bin", kind: "file", size: 2048, mode: "-rw-r--r--", perm: 0o644, mtime: "2026-11-20T00:00:00Z", binary: true },
      ],
    }));
  }
  if (url.includes("/files/preview") && url.includes(".env")) {
    return Promise.resolve(json({
      path: "/home/dev/.env",
      canonical_path: "/home/dev/.env",
      name: ".env",
      size: 48,
      mode: "-rw-------",
      perm: 0o600,
      mime: "text/plain; charset=utf-8",
      revision: "r-env",
      sensitive: true,
      sensitive_reason: "env",
      text: "DEMO_FLAG=1\n",
      bytes_read: 12,
      max_bytes: 262144,
    }));
  }
  if (url.includes("/files/preview") && url.includes("image.bin")) {
    return Promise.resolve(json({
      path: "/home/dev/image.bin",
      canonical_path: "/home/dev/image.bin",
      name: "image.bin",
      size: 2048,
      mode: "-rw-r--r--",
      perm: 0o644,
      mime: "application/octet-stream",
      revision: "r-bin",
      binary: true,
      bytes_read: 2048,
      max_bytes: 262144,
    }));
  }
  if (url.includes("/files/tail")) {
    tailCalls++;
    return Promise.resolve(json({
      path: "/home/dev/app.log",
      canonical_path: "/home/dev/app.log",
      name: "app.log",
      size: 600000,
      mime: "text/plain; charset=utf-8",
      revision: `r-log-${tailCalls}`,
      text: `tail-batch-${tailCalls}\nline-end\n`,
      lines: 2,
      requested_lines: 200,
      bytes_read: 24,
      offset: 599976,
      truncated: true,
    }));
  }
  return Promise.resolve(json({ error: "not found" }, 404));
}

beforeEach(() => {
  calls = [];
  tailCalls = 0;
  setToken("tok-test");
  vi.stubGlobal("fetch", vi.fn(stubHub));
  host = document.createElement("div");
  document.body.append(host);
  root = createRoot(host);
});

afterEach(() => {
  act(() => root.unmount());
  host.remove();
  vi.unstubAllGlobals();
});

describe("FileBrowserPanel", () => {
  const onlineMachines: Machine[] = [{ id: "m1", name: "DevBox", online: true }];

  it("loads roots and single-level listing, labels sensitive/binary files, previews, tails, and builds download links", async () => {
    act(() => root.render(<FileBrowserPanel machines={onlineMachines} />));
    await flush();

    // Quick root shortcuts rendered.
    expect(host.textContent).toContain("$HOME");
    expect(host.textContent).toContain("persona");
    expect(host.textContent).toContain(".config");

    // Sensitive file preview shows warning banner and download link.
    const envRow = host.querySelector<HTMLButtonElement>('[data-testid="file-entry-.env"]');
    expect(envRow).not.toBeNull();
    await act(async () => { envRow!.click(); });
    await flush();
    expect(host.querySelector('[data-testid="sensitive-banner"]')).not.toBeNull();
    expect(host.querySelector('[data-testid="machine-file-content"]')?.textContent).toContain("DEMO_FLAG=1");
    const dl = host.querySelector<HTMLAnchorElement>('[data-testid="machine-file-download"]');
    expect(dl?.getAttribute("href")).toBe(machineFileDownloadURL("m1", "/home/dev/.env"));

    // Binary file refuses raw text render cleanly.
    const binRow = host.querySelector<HTMLButtonElement>('[data-testid="file-entry-image.bin"]');
    await act(async () => { binRow!.click(); });
    await flush();
    expect(host.querySelector('[data-testid="binary-refusal"]')).not.toBeNull();
    expect(host.querySelector('[data-testid="machine-file-content"]')).toBeNull();

    // Large log defaults to tail mode and supports refresh.
    const logRow = host.querySelector<HTMLButtonElement>('[data-testid="file-entry-app.log"]');
    await act(async () => { logRow!.click(); });
    await flush();
    expect(host.querySelector('[data-testid="machine-file-content"]')?.textContent).toContain("tail-batch-1");
    const refreshBtn = [...host.querySelectorAll("button")].find((b) => b.textContent?.includes("刷新末尾") || b.textContent?.includes("Refresh tail"));
    expect(refreshBtn).toBeTruthy();
    await act(async () => { refreshBtn!.click(); });
    await flush();
    expect(host.querySelector('[data-testid="machine-file-content"]')?.textContent).toContain("tail-batch-2");
  });

  it("shows offline notice without issuing file requests when selected machine is offline", async () => {
    act(() => root.render(<FileBrowserPanel machines={[{ id: "m2", name: "OfflineBox", online: false }]} />));
    await flush();
    expect(calls).toEqual([]);
    expect(host.querySelector('[data-testid="machine-file-browser"]')?.textContent).toBeTruthy();
  });
});
