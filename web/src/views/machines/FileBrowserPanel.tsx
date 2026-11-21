// Lazy-loaded read-only machine file browser (Phase 6A, human-only UI).
// Single-level directory listing, quick root shortcuts, bounded text preview (<= 2 MiB),
// large log tail with refresh, binary refusal, sensitive file warnings, and streaming downloads.
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { formatDate, t } from "../../i18n";
import { api, enc, errorText, getToken, isAbort } from "../../lib/api";
import { BASE } from "../../lib/base";
import type { Machine } from "../../lib/types";
import { Button, cx, IconButton, Loading } from "../../ui/atoms";
import { Icon } from "../../ui/icons";

export interface MachineFileRoot {
  id: string;
  label: string;
  path: string;
  kind: "home" | "workspace" | "config" | string;
  exists: boolean;
}

export interface MachineFileRootsResponse {
  home: string;
  os: string;
  roots: MachineFileRoot[];
}

export interface MachineFileEntry {
  name: string;
  path: string;
  kind: "dir" | "file" | "symlink" | "other" | string;
  size: number;
  mode: string;
  perm: number;
  mtime?: string;
  symlink?: boolean;
  link_target?: string;
  link_broken?: boolean;
  target_kind?: string;
  sensitive?: boolean;
  sensitive_reason?: string;
  binary?: boolean;
}

export interface MachineFileListResponse {
  path: string;
  canonical_path: string;
  parent?: string;
  entries: MachineFileEntry[];
  total: number;
  truncated?: boolean;
}

export interface MachineFilePreviewResponse {
  path: string;
  canonical_path: string;
  name: string;
  size: number;
  mode: string;
  perm: number;
  mtime?: string;
  mime: string;
  revision: string;
  symlink?: boolean;
  link_target?: string;
  sensitive?: boolean;
  sensitive_reason?: string;
  binary?: boolean;
  text?: string;
  bytes_read: number;
  truncated?: boolean;
  max_bytes: number;
}

export interface MachineFileTailResponse {
  path: string;
  canonical_path: string;
  name: string;
  size: number;
  mtime?: string;
  mime: string;
  revision: string;
  sensitive?: boolean;
  sensitive_reason?: string;
  binary?: boolean;
  text?: string;
  lines: number;
  requested_lines: number;
  bytes_read: number;
  offset: number;
  truncated?: boolean;
}

type ViewMode = "preview" | "tail";
const TAIL_LINE_OPTIONS = [100, 200, 500, 1000] as const;
const LARGE_LOG_THRESHOLD = 256 * 1024;

/** Streaming single-file download URL on the authenticated Hub HTTP route. */
export function machineFileDownloadURL(mid: string, path: string): string {
  return `${BASE}/api/v1/machines/${enc(mid)}/files/download?path=${enc(path)}&token=${enc(getToken())}`;
}

function formatSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "0 B";
  if (bytes < 1024) return `${bytes} B`;
  const units = ["KiB", "MiB", "GiB", "TiB"];
  let value = bytes / 1024;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value >= 10 ? value.toFixed(1) : value.toFixed(2)} ${units[unit]}`;
}

function formatMtime(iso?: string): string {
  if (!iso) return "";
  const ms = Date.parse(iso);
  if (!Number.isFinite(ms)) return "";
  return formatDate(new Date(ms), { dateStyle: "short", timeStyle: "short" });
}

function rootKindLabel(kind: string): string {
  switch (kind) {
    case "home":
      return t("machines-files.root_home");
    case "workspace":
      return t("machines-files.root_workspace");
    default:
      return t("machines-files.root_config");
  }
}

function sensitiveReasonLabel(reason?: string): string {
  switch (reason) {
    case "env":
      return t("machines-files.sensitive_env");
    case "private_key":
      return t("machines-files.sensitive_private_key");
    case "hub_config":
      return t("machines-files.sensitive_hub_config");
    case "credentials":
      return t("machines-files.sensitive_credentials");
    default:
      return t("machines-files.sensitive_protected");
  }
}

function entryKindLabel(entry: MachineFileEntry): string {
  if (entry.symlink || entry.kind === "symlink") return t("machines-files.kind_symlink");
  if (entry.kind === "dir") return t("machines-files.kind_dir");
  return t("machines-files.kind_file");
}

function isNavigableDir(entry: MachineFileEntry): boolean {
  if (entry.kind === "dir") return true;
  return Boolean(entry.symlink && !entry.link_broken && entry.target_kind === "dir");
}

function shouldDefaultToTail(entry: MachineFileEntry): boolean {
  const lower = entry.name.toLowerCase();
  if (entry.size >= LARGE_LOG_THRESHOLD && !entry.binary) return true;
  return (lower.endsWith(".log") || lower.endsWith(".out") || lower.endsWith(".jsonl")) && entry.size > 32 * 1024;
}

export function FileBrowserPanel({ machines, initialMachineId }: { machines: Machine[]; initialMachineId?: string }) {
  const defaultMid = useMemo(() => {
    if (initialMachineId && machines.some((m) => m.id === initialMachineId)) return initialMachineId;
    return machines.find((m) => m.online)?.id ?? machines[0]?.id ?? "";
  }, [machines, initialMachineId]);

  const [selectedMid, setSelectedMid] = useState(defaultMid);
  const activeMid = machines.some((m) => m.id === selectedMid) ? selectedMid : defaultMid;
  const machine = machines.find((m) => m.id === activeMid);

  const [roots, setRoots] = useState<MachineFileRoot[]>([]);
  const [dir, setDir] = useState<MachineFileListResponse | null>(null);
  const [pathInput, setPathInput] = useState("~");
  const [filter, setFilter] = useState("");
  const [loadingDir, setLoadingDir] = useState(false);
  const [dirError, setDirError] = useState("");

  const [selectedEntry, setSelectedEntry] = useState<MachineFileEntry | null>(null);
  const [mode, setMode] = useState<ViewMode>("preview");
  const [tailLines, setTailLines] = useState<number>(200);
  const [wrap, setWrap] = useState(false);
  const [preview, setPreview] = useState<MachineFilePreviewResponse | null>(null);
  const [tail, setTail] = useState<MachineFileTailResponse | null>(null);
  const [loadingFile, setLoadingFile] = useState(false);
  const [fileError, setFileError] = useState("");

  const dirAbort = useRef<AbortController | null>(null);
  const fileAbort = useRef<AbortController | null>(null);

  const loadDirectory = useCallback(async (mid: string, targetPath: string) => {
    if (!mid) return;
    dirAbort.current?.abort();
    const ctrl = new AbortController();
    dirAbort.current = ctrl;
    setLoadingDir(true);
    setDirError("");
    try {
      const qs = targetPath ? `?path=${enc(targetPath)}` : "";
      const res = await api<MachineFileListResponse>(`/machines/${enc(mid)}/files/list${qs}`, { signal: ctrl.signal });
      if (ctrl.signal.aborted) return;
      setDir(res);
      setPathInput(res.path);
      setFilter("");
    } catch (err) {
      if (isAbort(err)) return;
      setDirError(t("machines-files.load_error", { error: errorText(err) }));
    } finally {
      if (!ctrl.signal.aborted) setLoadingDir(false);
    }
  }, []);

  const loadFile = useCallback(async (mid: string, targetPath: string, viewMode: ViewMode, lines: number) => {
    if (!mid || !targetPath) return;
    fileAbort.current?.abort();
    const ctrl = new AbortController();
    fileAbort.current = ctrl;
    setLoadingFile(true);
    setFileError("");
    try {
      if (viewMode === "tail") {
        const res = await api<MachineFileTailResponse>(
          `/machines/${enc(mid)}/files/tail?path=${enc(targetPath)}&lines=${lines}`,
          { signal: ctrl.signal },
        );
        if (ctrl.signal.aborted) return;
        setTail(res);
        setPreview(null);
      } else {
        const res = await api<MachineFilePreviewResponse>(
          `/machines/${enc(mid)}/files/preview?path=${enc(targetPath)}`,
          { signal: ctrl.signal },
        );
        if (ctrl.signal.aborted) return;
        setPreview(res);
        setTail(null);
      }
    } catch (err) {
      if (isAbort(err)) return;
      setFileError(t("machines-files.load_error", { error: errorText(err) }));
    } finally {
      if (!ctrl.signal.aborted) setLoadingFile(false);
    }
  }, []);

  // Load roots and initial $HOME listing whenever the active online machine changes.
  useEffect(() => {
    setRoots([]);
    setDir(null);
    setDirError("");
    setSelectedEntry(null);
    setPreview(null);
    setTail(null);
    setFileError("");
    if (!machine?.online) return;

    const ctrl = new AbortController();
    api<MachineFileRootsResponse>(`/machines/${enc(machine.id)}/files/roots`, { signal: ctrl.signal })
      .then((res) => {
        if (ctrl.signal.aborted) return;
        const list = res.roots ?? [];
        setRoots(list);
        const startPath = res.home || list[0]?.path || "~";
        void loadDirectory(machine.id, startPath);
      })
      .catch((err) => {
        if (isAbort(err)) return;
        void loadDirectory(machine.id, "~");
      });

    return () => {
      ctrl.abort();
      dirAbort.current?.abort();
      fileAbort.current?.abort();
    };
  }, [machine?.id, machine?.online, loadDirectory]);

  const openEntry = useCallback((entry: MachineFileEntry) => {
    if (!machine?.online) return;
    if (isNavigableDir(entry)) {
      setSelectedEntry(null);
      setPreview(null);
      setTail(null);
      setFileError("");
      void loadDirectory(machine.id, entry.path);
      return;
    }
    const nextMode: ViewMode = shouldDefaultToTail(entry) ? "tail" : "preview";
    setSelectedEntry(entry);
    setMode(nextMode);
    void loadFile(machine.id, entry.path, nextMode, tailLines);
  }, [machine?.id, machine?.online, loadDirectory, loadFile, tailLines]);

  const switchMode = useCallback((nextMode: ViewMode) => {
    setMode(nextMode);
    if (machine?.online && selectedEntry) {
      void loadFile(machine.id, selectedEntry.path, nextMode, tailLines);
    }
  }, [machine?.id, machine?.online, selectedEntry, loadFile, tailLines]);

  const changeTailLines = useCallback((nextLines: number) => {
    setTailLines(nextLines);
    if (machine?.online && selectedEntry && mode === "tail") {
      void loadFile(machine.id, selectedEntry.path, "tail", nextLines);
    }
  }, [machine?.id, machine?.online, selectedEntry, mode, loadFile]);

  const visibleEntries = useMemo(() => {
    const list = dir?.entries ?? [];
    const q = filter.trim().toLowerCase();
    if (!q) return list;
    return list.filter((e) => e.name.toLowerCase().includes(q));
  }, [dir?.entries, filter]);

  if (machines.length === 0) {
    return (
      <section className="rounded-card bg-surface p-6 text-center text-sm text-ink-3 shadow-card" data-testid="machine-file-browser">
        {t("machines-files.no_machines")}
      </section>
    );
  }

  const activeFile = mode === "tail" ? tail : preview;
  const activeSensitive = Boolean(activeFile?.sensitive ?? selectedEntry?.sensitive);
  const activeSensitiveReason = activeFile?.sensitive_reason ?? selectedEntry?.sensitive_reason;
  const activeBinary = Boolean(activeFile?.binary);
  const downloadPath = activeFile?.path ?? selectedEntry?.path ?? "";
  const downloadName = activeFile?.name ?? selectedEntry?.name ?? "";

  return (
    <section className="flex flex-col gap-3" data-testid="machine-file-browser">
      {/* Top toolbar: machine switcher + read-only badge + quick root shortcuts */}
      <div className="rounded-card bg-surface p-4 shadow-card">
        <div className="flex flex-wrap items-center gap-2">
          <div className="flex items-center gap-2">
            <Icon name="folder" size={16} className="text-ink-2" />
            <h2 className="text-sm font-semibold text-ink">{t("machines-files.title")}</h2>
            <span className="inline-flex items-center gap-1 rounded-full bg-field px-2 py-0.5 text-xs font-medium text-ink-2">
              <Icon name="lock" size={11} />
              {t("machines-files.read_only")}
            </span>
          </div>
          <div className="flex-1" />
          <label className="flex items-center gap-2 text-xs text-ink-3">
            <span>{t("machines-files.machine_label")}</span>
            <select
              aria-label={t("machines-files.machine_label")}
              value={activeMid}
              onChange={(e) => setSelectedMid(e.target.value)}
              className="h-8 rounded-control bg-inset px-2.5 text-xs text-ink shadow-hairline outline-none"
            >
              {machines.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.name} {!m.online ? `(${t("machines.offline")})` : ""}
                </option>
              ))}
            </select>
          </label>
        </div>

        {!machine?.online ? (
          <p className="mt-3 rounded-control bg-field px-3 py-2 text-xs text-ink-2">
            {t("machines-files.machine_offline")}
          </p>
        ) : (
          <>
            {roots.length > 0 && (
              <div className="mt-3 flex flex-wrap items-center gap-1.5" aria-label={t("machines-files.roots_label")}>
                <span className="mr-1 text-xs text-ink-3">{t("machines-files.roots_label")}</span>
                {roots.map((r) => (
                  <button
                    key={r.id}
                    type="button"
                    onClick={() => void loadDirectory(machine.id, r.path)}
                    title={r.path}
                    className={cx(
                      "inline-flex items-center gap-1.5 rounded-full px-2.5 py-1 text-xs transition-colors",
                      dir?.path === r.path
                        ? "bg-ink text-surface font-medium"
                        : "bg-inset text-ink-2 shadow-hairline hover:bg-hover hover:text-ink",
                    )}
                  >
                    <Icon name={r.kind === "home" ? "home" : r.kind === "workspace" ? "layers" : "gear"} size={12} />
                    <span>{r.label}</span>
                    <span className="opacity-70">{rootKindLabel(r.kind)}</span>
                  </button>
                ))}
              </div>
            )}

            <form
              className="mt-3 flex flex-wrap items-center gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                if (pathInput.trim()) void loadDirectory(machine.id, pathInput.trim());
              }}
            >
              <Button
                tone="secondary"
                icon="up"
                disabled={!dir?.parent || loadingDir}
                onClick={() => dir?.parent && void loadDirectory(machine.id, dir.parent)}
              >
                {t("machines-files.up")}
              </Button>
              <input
                type="text"
                value={pathInput}
                onChange={(e) => setPathInput(e.target.value)}
                placeholder={t("machines-files.path_placeholder")}
                aria-label={t("machines-files.path_label")}
                className="h-8 min-w-[200px] flex-1 rounded-control bg-inset px-2.5 font-mono text-xs text-ink shadow-hairline outline-none placeholder:text-ink-3"
              />
              <Button type="submit" tone="primary" disabled={loadingDir || !pathInput.trim()}>
                {t("machines-files.go")}
              </Button>
              <IconButton
                icon="retry"
                label={t("machines-files.refresh")}
                disabled={loadingDir}
                onClick={() => void loadDirectory(machine.id, dir?.path || pathInput || "~")}
              />
            </form>
          </>
        )}
      </div>

      {machine?.online && (
        <div className="grid grid-cols-1 gap-3 md:grid-cols-2">
          {/* Single-level directory listing */}
          <div className="flex flex-col overflow-hidden rounded-card bg-surface shadow-card">
            <div className="flex items-center gap-2 border-b border-line px-3 py-2">
              <Icon name="search" size={13} className="shrink-0 text-ink-3" />
              <input
                type="search"
                value={filter}
                onChange={(e) => setFilter(e.target.value)}
                placeholder={t("machines-files.filter_placeholder")}
                aria-label={t("machines-files.filter_label")}
                className="min-w-0 flex-1 bg-transparent text-xs text-ink outline-none placeholder:text-ink-3"
              />
              {dir && (
                <span className="shrink-0 text-xs text-ink-3 tabular-nums">
                  {t("machines-files.entries_count", { count: visibleEntries.length })}
                </span>
              )}
            </div>

            {dir?.truncated && (
              <p className="border-b border-line bg-orange-tint px-3 py-1.5 text-xs text-ink">
                {t("machines-files.entries_truncated", { shown: dir.entries.length, total: dir.total })}
              </p>
            )}

            {dirError && (
              <p role="alert" className="m-3 rounded-control bg-red-tint px-3 py-2 text-xs text-red">
                {dirError}
              </p>
            )}

            {loadingDir && !dir ? (
              <div className="flex h-48 items-center justify-center">
                <Loading />
              </div>
            ) : visibleEntries.length === 0 ? (
              <p className="px-4 py-8 text-center text-xs text-ink-3">
                {filter.trim() ? t("machines-files.no_filter_matches", { query: filter.trim() }) : t("machines-files.empty_dir")}
              </p>
            ) : (
              <div className="divide-y divide-line overflow-y-auto" style={{ maxHeight: "34rem" }} role="table" aria-label={t("machines-files.title")}>
                <div className="hidden grid-cols-[minmax(0,1fr)_auto_auto_auto] gap-2 border-b border-line bg-field/60 px-3 py-1.5 text-xs font-medium text-ink-3 sm:grid" role="row">
                  <span role="columnheader">{t("machines-files.col_name")}</span>
                  <span role="columnheader">{t("machines-files.col_size")}</span>
                  <span role="columnheader">{t("machines-files.col_mode")}</span>
                  <span role="columnheader">{t("machines-files.col_mtime")}</span>
                </div>
                {visibleEntries.map((entry) => {
                  const selected = selectedEntry?.path === entry.path;
                  const dirLike = isNavigableDir(entry);
                  return (
                    <button
                      key={entry.path}
                      type="button"
                      onClick={() => openEntry(entry)}
                      className={cx(
                        "flex w-full items-center gap-2 px-3 py-2 text-left text-xs transition-colors hover:bg-hover",
                        selected && "bg-accent-tint/60",
                      )}
                      data-testid={`file-entry-${entry.name}`}
                    >
                      <Icon
                        name={entry.symlink ? "link" : dirLike ? "folder" : "file"}
                        size={14}
                        className={cx("shrink-0", dirLike ? "text-accent-ink" : "text-ink-3")}
                      />
                      <span className="min-w-0 flex-1">
                        <span className="flex flex-wrap items-center gap-1.5">
                          <span className="truncate font-medium text-ink" title={`${entryKindLabel(entry)}: ${entry.name}`}>
                            {entry.name}
                          </span>
                          {entry.symlink && (
                            <span
                              className={cx(
                                "inline-flex items-center rounded px-1 py-0.5 font-mono text-xs",
                                entry.link_broken ? "bg-red-tint text-red" : "bg-field text-ink-3",
                              )}
                              title={entry.link_target ? t("machines-files.symlink_target", { target: entry.link_target }) : undefined}
                            >
                              {entry.link_broken ? t("machines-files.symlink_broken") : `→ ${entry.link_target ?? ""}`}
                            </span>
                          )}
                          {entry.sensitive && (
                            <span
                              className="inline-flex items-center gap-1 rounded bg-orange-tint px-1.5 py-0.5 text-xs font-medium text-orange"
                              title={sensitiveReasonLabel(entry.sensitive_reason)}
                            >
                              <Icon name="shield" size={11} />
                              {t("machines-files.badge_sensitive")}
                            </span>
                          )}
                          {entry.binary && (
                            <span className="inline-flex items-center rounded bg-field px-1.5 py-0.5 text-xs text-ink-3">
                              {t("machines-files.badge_binary")}
                            </span>
                          )}
                        </span>
                        <span className="mt-0.5 flex flex-wrap items-center gap-2 font-mono text-xs text-ink-3">
                          <span>{dirLike ? "—" : formatSize(entry.size)}</span>
                          <span>{entry.mode}</span>
                          {entry.mtime && <span>{formatMtime(entry.mtime)}</span>}
                        </span>
                      </span>
                    </button>
                  );
                })}
              </div>
            )}
          </div>

          {/* Bounded preview, log tail & streaming download pane */}
          <div className="flex flex-col overflow-hidden rounded-card bg-surface shadow-card">
            {!selectedEntry ? (
              <div className="flex flex-1 flex-col items-center justify-center gap-2 p-6 text-center text-xs text-ink-3" style={{ minHeight: "16rem" }}>
                <Icon name="read" size={20} className="text-ink-3" />
                <p>{t("machines-files.select_file")}</p>
              </div>
            ) : (
              <>
                <div className="flex flex-wrap items-center gap-2 border-b border-line px-4 py-2.5">
                  <div className="min-w-0 flex-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <h3 className="truncate font-mono text-sm font-semibold text-ink">{selectedEntry.name}</h3>
                      <span className="font-mono text-xs text-ink-3">{formatSize(activeFile?.size ?? selectedEntry.size)}</span>
                      {preview?.mode && <span className="font-mono text-xs text-ink-3">{preview.mode}</span>}
                    </div>
                    <p className="truncate font-mono text-xs text-ink-3" title={activeFile?.canonical_path || selectedEntry.path}>
                      {activeFile?.canonical_path || selectedEntry.path}
                    </p>
                  </div>

                  <div className="flex flex-wrap items-center gap-1.5">
                    <div className="flex items-center rounded-control bg-inset p-0.5 shadow-hairline" role="tablist">
                      <button
                        type="button"
                        role="tab"
                        aria-selected={mode === "preview"}
                        onClick={() => switchMode("preview")}
                        className={cx(
                          "rounded px-2 py-1 text-xs transition-colors",
                          mode === "preview" ? "bg-surface font-medium text-ink shadow-btn" : "text-ink-3 hover:text-ink",
                        )}
                      >
                        {t("machines-files.mode_preview")}
                      </button>
                      <button
                        type="button"
                        role="tab"
                        aria-selected={mode === "tail"}
                        onClick={() => switchMode("tail")}
                        className={cx(
                          "rounded px-2 py-1 text-xs transition-colors",
                          mode === "tail" ? "bg-surface font-medium text-ink shadow-btn" : "text-ink-3 hover:text-ink",
                        )}
                      >
                        {t("machines-files.mode_tail")}
                      </button>
                    </div>

                    {mode === "tail" && (
                      <>
                        <select
                          aria-label={t("machines-files.tail_lines_label")}
                          value={tailLines}
                          onChange={(e) => changeTailLines(Number(e.target.value))}
                          className="h-7 rounded-control bg-inset px-2 text-xs text-ink shadow-hairline outline-none"
                        >
                          {TAIL_LINE_OPTIONS.map((n) => (
                            <option key={n} value={n}>
                              {t("machines-files.tail_lines_option", { count: n })}
                            </option>
                          ))}
                        </select>
                        <Button
                          tone="secondary"
                          icon="retry"
                          disabled={loadingFile}
                          onClick={() => void loadFile(machine.id, selectedEntry.path, "tail", tailLines)}
                        >
                          {t("machines-files.tail_refresh")}
                        </Button>
                      </>
                    )}

                    {!activeBinary && (
                      <Button tone="quiet" onClick={() => setWrap((w) => !w)}>
                        {wrap ? t("machines-files.wrap_on") : t("machines-files.wrap_off")}
                      </Button>
                    )}

                    {downloadPath && (
                      <a
                        href={machineFileDownloadURL(machine.id, downloadPath)}
                        download={downloadName}
                        className="inline-flex h-8 items-center gap-1.5 rounded-control bg-inset px-2.5 text-xs font-medium text-ink shadow-hairline transition-colors hover:bg-hover"
                        data-testid="machine-file-download"
                      >
                        <Icon name="download" size={13} />
                        {t("machines-files.download")}
                      </a>
                    )}
                  </div>
                </div>

                {activeSensitive && (
                  <div className="flex items-center gap-2 border-b border-line bg-orange-tint px-4 py-2 text-xs text-ink" data-testid="sensitive-banner">
                    <Icon name="shield" size={14} className="shrink-0 text-orange" />
                    <span>{t("machines-files.sensitive_banner", { reason: sensitiveReasonLabel(activeSensitiveReason) })}</span>
                  </div>
                )}

                {mode === "preview" && preview?.truncated && !preview.binary && (
                  <div className="flex flex-wrap items-center justify-between gap-2 border-b border-line bg-field px-4 py-2 text-xs text-ink-2">
                    <span>
                      {t("machines-files.preview_truncated", {
                        max: formatSize(preview.max_bytes),
                        shown: formatSize(preview.bytes_read),
                        total: formatSize(preview.size),
                      })}
                    </span>
                    <Button tone="secondary" onClick={() => switchMode("tail")}>
                      {t("machines-files.switch_to_tail")}
                    </Button>
                  </div>
                )}

                {mode === "tail" && tail && !tail.binary && (
                  <div className="border-b border-line bg-field px-4 py-1.5 font-mono text-xs text-ink-3">
                    {t("machines-files.tail_meta", {
                      lines: tail.lines,
                      bytes: formatSize(tail.bytes_read),
                      offset: tail.offset,
                    })}
                  </div>
                )}

                {fileError && (
                  <p role="alert" className="m-4 rounded-control bg-red-tint px-3 py-2 text-xs text-red">
                    {fileError}
                  </p>
                )}

                {loadingFile ? (
                  <div className="flex h-56 items-center justify-center">
                    <Loading />
                  </div>
                ) : activeBinary ? (
                  <div className="flex flex-col items-center justify-center gap-3 p-8 text-center" data-testid="binary-refusal">
                    <Icon name="warning" size={22} className="text-orange" />
                    <h4 className="text-sm font-semibold text-ink">{t("machines-files.binary_title")}</h4>
                    <p className="max-w-md text-xs text-ink-2">
                      {t("machines-files.binary_body", {
                        mime: activeFile?.mime || "application/octet-stream",
                        size: formatSize(activeFile?.size ?? selectedEntry.size),
                      })}
                    </p>
                    {downloadPath && (
                      <a
                        href={machineFileDownloadURL(machine.id, downloadPath)}
                        download={downloadName}
                        className="inline-flex h-8 items-center gap-1.5 rounded-control bg-ink px-3 text-xs font-medium text-surface transition-opacity hover:opacity-90"
                      >
                        <Icon name="download" size={13} />
                        {t("machines-files.download")}
                      </a>
                    )}
                  </div>
                ) : activeFile && !(activeFile.text ?? "") ? (
                  <p className="p-6 text-center text-xs text-ink-3">{t("machines-files.empty_file")}</p>
                ) : activeFile ? (
                  <pre
                    className={cx(
                      "flex-1 overflow-auto bg-inset p-4 font-mono text-xs leading-relaxed text-ink select-text",
                      wrap ? "whitespace-pre-wrap break-words" : "whitespace-pre",
                    )}
                    style={{ maxHeight: "34rem" }}
                    data-testid="machine-file-content"
                  >
                    {activeFile.text}
                  </pre>
                ) : null}
              </>
            )}
          </div>
        </div>
      )}
    </section>
  );
}
