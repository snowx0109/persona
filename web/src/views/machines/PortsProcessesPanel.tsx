// Lazy-loaded Ports & Processes inspector for Machines (#12352).
// Fetches on demand only when opened or explicitly refreshed (no background polling).
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { api, ApiError, enc, errorText, isAbort } from "../../lib/api";
import type { Machine } from "../../lib/types";
import { Button, cx, Dialog, Loading, toast } from "../../ui/atoms";
import { Icon } from "../../ui/icons";
import { t } from "../../i18n";

export interface ListeningPort {
  protocol: string;
  bind_addr: string;
  port: number;
  scope: "loopback" | "all" | "interface" | string;
  pid?: number;
  user?: string;
  process?: string;
  cmdline?: string;
}

export interface MachinePortsResponse {
  at: number;
  supported: boolean;
  unsupported?: string;
  ports: ListeningPort[];
}

export interface MachineProcess {
  pid: number;
  ppid: number;
  uid?: number;
  user?: string;
  name: string;
  cmdline: string;
  cpu_percent: number;
  rss_bytes: number;
  uptime_seconds: number;
  state?: string;
  owned: boolean;
  protected?: boolean;
  protected_reason?: string;
  likely_orphan?: boolean;
  orphan_reasons?: string[];
}

export interface MachineProcessesResponse {
  at: number;
  supported: boolean;
  unsupported?: string;
  current_user?: string;
  mem_total?: number;
  total_count: number;
  orphan_count: number;
  processes: MachineProcess[];
}

type PanelTab = "ports" | "processes";
type ProtocolFilter = "" | "tcp" | "udp";
type ProcessSort = "cpu" | "memory" | "uptime";
type KillSignal = "SIGTERM" | "SIGKILL";

interface ConfirmTarget {
  pid: number;
  name: string;
  cmdline: string;
  signal: KillSignal;
}

function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let v = bytes;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return i === 0 ? `${Math.round(v)} B` : `${v >= 100 ? v.toFixed(0) : v.toFixed(1)} ${units[i]}`;
}

function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return "0s";
  const s = Math.floor(seconds);
  const d = Math.floor(s / 86400);
  const h = Math.floor((s % 86400) / 3600);
  const m = Math.floor((s % 3600) / 60);
  const rem = s % 60;
  if (d > 0) return `${d}d ${h}h`;
  if (h > 0) return `${h}h ${m}m`;
  if (m > 0) return `${m}m ${rem}s`;
  return `${rem}s`;
}

function formatTime(unixSec: number): string {
  if (!unixSec) return "";
  try {
    return new Date(unixSec * 1000).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
  } catch {
    return "";
  }
}

function scopeLabel(scope: string): string {
  switch (scope) {
    case "loopback":
      return t("machines-procs.scope_loopback");
    case "all":
      return t("machines-procs.scope_all");
    default:
      return t("machines-procs.scope_interface");
  }
}

function orphanReasonLabel(reason: string): string {
  switch (reason) {
    case "zombie":
      return t("machines-procs.reason_zombie");
    case "reparented_to_init":
      return t("machines-procs.reason_reparented_to_init");
    case "test_process":
      return t("machines-procs.reason_test_process");
    case "temp_binary":
      return t("machines-procs.reason_temp_binary");
    case "long_running_test":
      return t("machines-procs.reason_long_running_test");
    default:
      return reason;
  }
}

function friendlyError(err: unknown): string {
  if (err instanceof ApiError && (err.code === "node_outdated" || err.message.includes("unknown method"))) {
    return t("machines-procs.node_outdated");
  }
  return errorText(err);
}

export function PortsProcessesPanel({ machines, initialMachineId }: { machines: Machine[]; initialMachineId?: string }) {
  const onlineMachines = useMemo(() => machines.filter((m) => m.online), [machines]);
  const [selectedId, setSelectedId] = useState<string>(() => {
    if (initialMachineId && onlineMachines.some((m) => m.id === initialMachineId)) return initialMachineId;
    return onlineMachines[0]?.id ?? "";
  });

  // Keep selectedId pointing to an online machine when the roster updates.
  const activeMachine = useMemo(() => {
    const found = onlineMachines.find((m) => m.id === selectedId);
    return found ?? onlineMachines[0] ?? null;
  }, [onlineMachines, selectedId]);
  const mid = activeMachine?.id ?? "";

  const [tab, setTab] = useState<PanelTab>("ports");
  const [protocol, setProtocol] = useState<ProtocolFilter>("");
  const [portQuery, setPortQuery] = useState("");
  const [sort, setSort] = useState<ProcessSort>("cpu");
  const [procQuery, setProcQuery] = useState("");
  const [orphanOnly, setOrphanOnly] = useState(false);

  const [portsData, setPortsData] = useState<MachinePortsResponse | null>(null);
  const [procsData, setProcsData] = useState<MachineProcessesResponse | null>(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState("");

  const [confirmTarget, setConfirmTarget] = useState<ConfirmTarget | null>(null);
  const [confirmPidInput, setConfirmPidInput] = useState("");
  const [killing, setKilling] = useState(false);

  const ctrlRef = useRef<AbortController | null>(null);

  const load = useCallback(async (targetMid: string, targetTab: PanelTab, targetProto: ProtocolFilter, targetSort: ProcessSort) => {
    if (!targetMid) return;
    ctrlRef.current?.abort();
    const ctrl = new AbortController();
    ctrlRef.current = ctrl;
    setLoading(true);
    setError("");
    try {
      if (targetTab === "ports") {
        const qs = targetProto ? `?protocol=${encodeURIComponent(targetProto)}` : "";
        const res = await api<MachinePortsResponse>(`/machines/${enc(targetMid)}/ports${qs}`, { signal: ctrl.signal });
        if (ctrl.signal.aborted) return;
        setPortsData({ ...res, ports: res.ports ?? [] });
      } else {
        const params = new URLSearchParams({ sort: targetSort, limit: "150" });
        const res = await api<MachineProcessesResponse>(`/machines/${enc(targetMid)}/processes?${params.toString()}`, { signal: ctrl.signal });
        if (ctrl.signal.aborted) return;
        setProcsData({ ...res, processes: res.processes ?? [] });
      }
    } catch (e) {
      if (ctrl.signal.aborted || isAbort(e)) return;
      setError(friendlyError(e));
    } finally {
      if (!ctrl.signal.aborted) setLoading(false);
    }
  }, []);

  // On-demand load when machine, tab, protocol, or sort changes (no background timer).
  useEffect(() => {
    if (!mid) {
      setPortsData(null);
      setProcsData(null);
      return;
    }
    void load(mid, tab, protocol, sort);
    return () => ctrlRef.current?.abort();
  }, [mid, tab, protocol, sort, load]);

  const filteredPorts = useMemo(() => {
    const list = portsData?.ports ?? [];
    const q = portQuery.trim().toLowerCase();
    if (!q) return list;
    return list.filter(
      (p) =>
        String(p.port).includes(q) ||
        p.bind_addr.toLowerCase().includes(q) ||
        p.protocol.toLowerCase().includes(q) ||
        (p.process ?? "").toLowerCase().includes(q) ||
        (p.cmdline ?? "").toLowerCase().includes(q) ||
        (p.user ?? "").toLowerCase().includes(q) ||
        (p.pid ? String(p.pid).includes(q) : false),
    );
  }, [portsData, portQuery]);

  const filteredProcs = useMemo(() => {
    const list = procsData?.processes ?? [];
    const q = procQuery.trim().toLowerCase();
    return list.filter((p) => {
      if (orphanOnly && !p.likely_orphan) return false;
      if (!q) return true;
      return (
        String(p.pid).includes(q) ||
        p.name.toLowerCase().includes(q) ||
        p.cmdline.toLowerCase().includes(q) ||
        (p.user ?? "").toLowerCase().includes(q)
      );
    });
  }, [procsData, procQuery, orphanOnly]);

  const openConfirm = (proc: Pick<MachineProcess, "pid" | "name" | "cmdline">, signal: KillSignal) => {
    setConfirmPidInput("");
    setConfirmTarget({ pid: proc.pid, name: proc.name, cmdline: proc.cmdline, signal });
  };

  const closeConfirm = () => {
    if (killing) return;
    setConfirmTarget(null);
    setConfirmPidInput("");
  };

  const executeKill = async () => {
    if (!confirmTarget || !mid) return;
    const { pid, signal } = confirmTarget;
    if (signal === "SIGKILL" && confirmPidInput.trim() !== String(pid)) return;
    setKilling(true);
    try {
      const payload = signal === "SIGKILL" ? { signal, confirm: true, confirm_pid: pid } : { signal, confirm: true };
      await api(`/machines/${enc(mid)}/processes/${pid}/kill`, { method: "POST", json: payload });
      toast(t("machines-procs.kill_sent", { signal, pid }), "ok");
      setConfirmTarget(null);
      setConfirmPidInput("");
      void load(mid, tab, protocol, sort);
    } catch (e) {
      toast(t("machines-procs.kill_failed", { pid, error: errorText(e) }), "error");
    } finally {
      setKilling(false);
    }
  };

  const updatedAt = tab === "ports" ? portsData?.at : procsData?.at;
  const sigkillReady = confirmTarget?.signal !== "SIGKILL" || confirmPidInput.trim() === String(confirmTarget.pid);

  if (onlineMachines.length === 0) {
    return (
      <section className="rounded-card bg-surface p-6 text-center shadow-card" data-testid="ports-processes-panel">
        <p className="text-sm font-medium text-ink">{t("machines-procs.title")}</p>
        <p className="mt-1 text-xs text-ink-3">{t("machines-procs.no_online_machines")}</p>
      </section>
    );
  }

  return (
    <section className="flex flex-col gap-3 rounded-card bg-surface p-4 shadow-card" data-testid="ports-processes-panel">
      {/* Header row: title, machine picker, and on-demand refresh */}
      <div className="flex flex-wrap items-center gap-2 border-b border-line pb-3">
        <div className="min-w-0 flex-1">
          <h2 className="text-sm font-semibold text-ink">{t("machines-procs.title")}</h2>
          <p className="mt-0.5 text-xs text-ink-3">{t("machines-procs.subtitle")}</p>
        </div>
        <div className="flex flex-wrap items-center gap-2">
          <label className="flex items-center gap-1.5 text-xs text-ink-3">
            <span>{t("machines-procs.machine_label")}</span>
            <select
              aria-label={t("machines-procs.machine_label")}
              value={mid}
              onChange={(e) => setSelectedId(e.target.value)}
              className="h-8 rounded-control bg-inset px-2 text-xs text-ink shadow-hairline outline-none"
            >
              {onlineMachines.map((m) => (
                <option key={m.id} value={m.id}>
                  {m.name}
                </option>
              ))}
            </select>
          </label>
          <Button tone="secondary" icon="retry" busy={loading} onClick={() => void load(mid, tab, protocol, sort)}>
            {loading ? t("machines-procs.refreshing") : t("machines-procs.refresh")}
          </Button>
        </div>
      </div>

      {/* Sub-tabs & summary */}
      <div className="flex flex-wrap items-center justify-between gap-2">
        <div className="flex items-center gap-1 rounded-control bg-inset p-0.5 shadow-hairline" role="tablist">
          <button
            type="button"
            role="tab"
            aria-selected={tab === "ports"}
            onClick={() => setTab("ports")}
            className={cx(
              "h-7 rounded-[7px] px-3 text-xs transition-colors",
              tab === "ports" ? "bg-surface font-medium text-ink shadow-btn" : "text-ink-3 hover:text-ink-2",
            )}
          >
            {t("machines-procs.tab_ports")}
            {portsData?.supported ? ` (${portsData.ports.length})` : ""}
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={tab === "processes"}
            onClick={() => setTab("processes")}
            className={cx(
              "h-7 rounded-[7px] px-3 text-xs transition-colors",
              tab === "processes" ? "bg-surface font-medium text-ink shadow-btn" : "text-ink-3 hover:text-ink-2",
            )}
          >
            {t("machines-procs.tab_processes")}
            {procsData?.supported ? ` (${procsData.total_count})` : ""}
          </button>
        </div>
        <div className="flex items-center gap-3 text-xs text-ink-3">
          {tab === "processes" && procsData?.supported && (
            <span>{t("machines-procs.summary_processes", { total: procsData.total_count, orphans: procsData.orphan_count })}</span>
          )}
          {updatedAt ? <span className="tabular-nums">{t("machines-procs.updated_at", { time: formatTime(updatedAt) })}</span> : null}
        </div>
      </div>

      {/* Filter toolbar */}
      {tab === "ports" ? (
        <div className="flex flex-wrap items-center gap-2">
          <div className="flex items-center gap-1 rounded-control bg-inset p-0.5 shadow-hairline">
            {(["", "tcp", "udp"] as const).map((p) => (
              <button
                key={p || "all"}
                type="button"
                onClick={() => setProtocol(p)}
                className={cx(
                  "h-7 rounded-[7px] px-2.5 text-xs transition-colors",
                  protocol === p ? "bg-surface font-medium text-ink shadow-btn" : "text-ink-3 hover:text-ink-2",
                )}
              >
                {p === "" ? t("machines-procs.protocol_all") : p === "tcp" ? t("machines-procs.protocol_tcp") : t("machines-procs.protocol_udp")}
              </button>
            ))}
          </div>
          <div className="relative min-w-[200px] flex-1">
            <Icon name="search" size={13} className="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2 text-ink-3" />
            <input
              type="search"
              value={portQuery}
              onChange={(e) => setPortQuery(e.target.value)}
              placeholder={t("machines-procs.search_ports")}
              aria-label={t("machines-procs.search_ports")}
              className="h-8 w-full rounded-control bg-inset pr-2.5 pl-8 text-xs text-ink shadow-hairline outline-none placeholder:text-ink-3"
            />
          </div>
        </div>
      ) : (
        <div className="flex flex-wrap items-center gap-2">
          <div className="flex items-center gap-1 rounded-control bg-inset p-0.5 shadow-hairline" aria-label={t("machines-procs.sort_label")}>
            {(["cpu", "memory", "uptime"] as const).map((s) => (
              <button
                key={s}
                type="button"
                onClick={() => setSort(s)}
                className={cx(
                  "h-7 rounded-[7px] px-2.5 text-xs transition-colors",
                  sort === s ? "bg-surface font-medium text-ink shadow-btn" : "text-ink-3 hover:text-ink-2",
                )}
              >
                {s === "cpu" ? t("machines-procs.sort_cpu") : s === "memory" ? t("machines-procs.sort_memory") : t("machines-procs.sort_uptime")}
              </button>
            ))}
          </div>
          <button
            type="button"
            aria-pressed={orphanOnly}
            onClick={() => setOrphanOnly((v) => !v)}
            className={cx(
              "inline-flex h-8 items-center gap-1.5 rounded-control px-2.5 text-xs font-medium transition-colors",
              orphanOnly ? "bg-orange-tint text-orange shadow-hairline" : "bg-inset text-ink-2 shadow-hairline hover:bg-hover",
            )}
          >
            <Icon name="warning" size={12} />
            {t("machines-procs.orphan_only", { count: procsData?.orphan_count ?? 0 })}
          </button>
          <div className="relative min-w-[200px] flex-1">
            <Icon name="search" size={13} className="pointer-events-none absolute top-1/2 left-2.5 -translate-y-1/2 text-ink-3" />
            <input
              type="search"
              value={procQuery}
              onChange={(e) => setProcQuery(e.target.value)}
              placeholder={t("machines-procs.search_processes")}
              aria-label={t("machines-procs.search_processes")}
              className="h-8 w-full rounded-control bg-inset pr-2.5 pl-8 text-xs text-ink shadow-hairline outline-none placeholder:text-ink-3"
            />
          </div>
        </div>
      )}

      {/* Error banner */}
      {error && (
        <p role="alert" className="flex items-start gap-2 rounded-[8px] bg-red-tint px-3 py-2 text-xs text-red">
          <Icon name="warning" size={14} className="mt-px shrink-0" />
          <span className="min-w-0 break-words">{t("machines-procs.load_failed", { error })}</span>
        </p>
      )}

      {/* Body */}
      {loading && ((tab === "ports" && !portsData) || (tab === "processes" && !procsData)) ? (
        <div className="py-6">
          <Loading />
        </div>
      ) : tab === "ports" ? (
        portsData && !portsData.supported ? (
          <p className="rounded-[8px] bg-inset px-3 py-3 text-xs text-ink-2">
            {t("machines-procs.unsupported", { reason: portsData.unsupported || "" })}
          </p>
        ) : filteredPorts.length === 0 ? (
          <p className="py-6 text-center text-xs text-ink-3">{t("machines-procs.empty_ports")}</p>
        ) : (
          <div className="overflow-x-auto rounded-[8px] border border-line">
            <table className="w-full border-collapse text-left text-xs">
              <thead>
                <tr className="border-b border-line bg-inset text-ink-3">
                  <th className="px-3 py-2 font-medium">{t("machines-procs.col_port")}</th>
                  <th className="px-3 py-2 font-medium">{t("machines-procs.col_bind")}</th>
                  <th className="px-3 py-2 font-medium">{t("machines-procs.col_proto")}</th>
                  <th className="px-3 py-2 font-medium">{t("machines-procs.col_pid")}</th>
                  <th className="px-3 py-2 font-medium">{t("machines-procs.col_process")}</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-line">
                {filteredPorts.map((p) => (
                  <tr key={`${p.protocol}-${p.bind_addr}-${p.port}-${p.pid ?? 0}`} className="hover:bg-hover/40">
                    <td className="px-3 py-2 font-mono font-medium text-ink tabular-nums">:{p.port}</td>
                    <td className="px-3 py-2">
                      <div className="flex flex-wrap items-center gap-1.5">
                        <span className="font-mono text-ink-2">{p.bind_addr}</span>
                        <span
                          className={cx(
                            "inline-flex items-center gap-1 rounded-full px-1.5 py-0.5 text-xs font-medium",
                            p.scope === "all"
                              ? "bg-orange-tint text-orange"
                              : p.scope === "loopback"
                                ? "bg-green-tint text-green"
                                : "bg-field text-ink-2",
                          )}
                        >
                          <Icon name={p.scope === "loopback" ? "lock" : "web"} size={10} />
                          {scopeLabel(p.scope)}
                        </span>
                      </div>
                    </td>
                    <td className="px-3 py-2 font-mono uppercase text-ink-2">{p.protocol}</td>
                    <td className="px-3 py-2 font-mono text-ink-2 tabular-nums">
                      {p.pid ? p.pid : <span className="text-ink-3">{t("machines-procs.pid_unknown")}</span>}
                    </td>
                    <td className="max-w-[380px] px-3 py-2">
                      <div className="flex items-center gap-1.5">
                        <span className="font-medium text-ink">{p.process || "—"}</span>
                        {p.user && <span className="text-ink-3">{p.user}</span>}
                      </div>
                      {p.cmdline && (
                        <div className="truncate font-mono text-ink-3" title={p.cmdline}>
                          {p.cmdline}
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )
      ) : procsData && !procsData.supported ? (
        <p className="rounded-[8px] bg-inset px-3 py-3 text-xs text-ink-2">
          {t("machines-procs.unsupported", { reason: procsData.unsupported || "" })}
        </p>
      ) : filteredProcs.length === 0 ? (
        <p className="py-6 text-center text-xs text-ink-3">{t("machines-procs.empty_processes")}</p>
      ) : (
        <div className="overflow-x-auto rounded-[8px] border border-line">
          <table className="w-full border-collapse text-left text-xs">
            <thead>
              <tr className="border-b border-line bg-inset text-ink-3">
                <th className="px-3 py-2 font-medium">{t("machines-procs.col_pid")}</th>
                <th className="px-3 py-2 font-medium">{t("machines-procs.col_process")}</th>
                <th className="px-3 py-2 font-medium">{t("machines-procs.col_cpu")}</th>
                <th className="px-3 py-2 font-medium">{t("machines-procs.col_memory")}</th>
                <th className="px-3 py-2 font-medium">{t("machines-procs.col_uptime")}</th>
                <th className="px-3 py-2 text-right font-medium">{t("machines-procs.col_actions")}</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-line">
              {filteredProcs.map((proc) => {
                const canSignal = proc.owned && !proc.protected;
                return (
                  <tr
                    key={proc.pid}
                    data-orphan={proc.likely_orphan ? "true" : undefined}
                    className={cx("hover:bg-hover/40", proc.likely_orphan && "bg-orange-tint/30")}
                  >
                    <td className="px-3 py-2 align-top font-mono tabular-nums">
                      <div className="font-medium text-ink">{proc.pid}</div>
                      <div className="text-ink-3">{t("machines-procs.ppid", { ppid: proc.ppid })}</div>
                    </td>
                    <td className="max-w-[420px] px-3 py-2 align-top">
                      <div className="flex flex-wrap items-center gap-1.5">
                        <span className="font-medium text-ink">{proc.name}</span>
                        {proc.user && <span className="text-ink-3">{proc.user}</span>}
                        {proc.likely_orphan && (
                          <span className="inline-flex items-center gap-1 rounded-full bg-orange-tint px-1.5 py-0.5 text-xs font-medium text-orange">
                            <Icon name="warning" size={10} />
                            {t("machines-procs.badge_orphan")}
                          </span>
                        )}
                        {proc.protected && (
                          <span className="inline-flex items-center gap-1 rounded-full bg-field px-1.5 py-0.5 text-xs font-medium text-ink-2">
                            <Icon name="shield" size={10} />
                            {t("machines-procs.badge_protected")}
                          </span>
                        )}
                        {!proc.owned && (
                          <span className="inline-flex items-center rounded-full bg-field px-1.5 py-0.5 text-xs text-ink-3">
                            {t("machines-procs.badge_other_user")}
                          </span>
                        )}
                      </div>
                      <div className="mt-0.5 truncate font-mono text-ink-3" title={proc.cmdline}>
                        {proc.cmdline}
                      </div>
                      {proc.orphan_reasons && proc.orphan_reasons.length > 0 && (
                        <div className="mt-1 flex flex-wrap gap-1">
                          {proc.orphan_reasons.map((r) => (
                            <span key={r} className="rounded bg-inset px-1.5 py-0.5 text-xs text-ink-2">
                              {orphanReasonLabel(r)}
                            </span>
                          ))}
                        </div>
                      )}
                    </td>
                    <td className="px-3 py-2 align-top font-mono text-ink-2 tabular-nums">{proc.cpu_percent.toFixed(1)}%</td>
                    <td className="px-3 py-2 align-top font-mono text-ink-2 tabular-nums">{formatBytes(proc.rss_bytes)}</td>
                    <td className="px-3 py-2 align-top font-mono text-ink-2 tabular-nums">{formatUptime(proc.uptime_seconds)}</td>
                    <td className="px-3 py-2 align-top text-right whitespace-nowrap">
                      {canSignal ? (
                        <div className="inline-flex items-center gap-1.5">
                          <Button tone="secondary" onClick={() => openConfirm(proc, "SIGTERM")}>
                            {t("machines-procs.action_term")}
                          </Button>
                          <Button tone="danger" onClick={() => openConfirm(proc, "SIGKILL")}>
                            {t("machines-procs.action_kill")}
                          </Button>
                        </div>
                      ) : (
                        <span className="text-ink-3">—</span>
                      )}
                    </td>
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}

      {/* Tiered confirmation dialog: SIGTERM (confirm) vs SIGKILL (confirm + PID match) */}
      <Dialog
        open={!!confirmTarget}
        onClose={closeConfirm}
        title={
          confirmTarget
            ? confirmTarget.signal === "SIGKILL"
              ? t("machines-procs.confirm_kill_title", { name: confirmTarget.name, pid: confirmTarget.pid })
              : t("machines-procs.confirm_term_title", { name: confirmTarget.name, pid: confirmTarget.pid })
            : undefined
        }
        footer={
          confirmTarget ? (
            <>
              <Button tone="quiet" disabled={killing} onClick={closeConfirm}>
                {t("machines-procs.cancel")}
              </Button>
              <Button
                tone={confirmTarget.signal === "SIGKILL" ? "danger" : "primary"}
                busy={killing}
                disabled={!sigkillReady}
                onClick={() => void executeKill()}
              >
                {confirmTarget.signal === "SIGKILL" ? t("machines-procs.confirm_kill_button") : t("machines-procs.confirm_term_button")}
              </Button>
            </>
          ) : undefined
        }
      >
        {confirmTarget && (
          <div className="flex flex-col gap-3 text-xs text-ink-2">
            <p>
              {confirmTarget.signal === "SIGKILL"
                ? t("machines-procs.confirm_kill_body", { pid: confirmTarget.pid })
                : t("machines-procs.confirm_term_body")}
            </p>
            <div className="rounded-[8px] bg-inset p-2.5 font-mono break-all text-ink-2">{confirmTarget.cmdline}</div>
            {confirmTarget.signal === "SIGKILL" && (
              <label className="flex flex-col gap-1.5">
                <span className="font-medium text-ink">{t("machines-procs.confirm_kill_input_label", { pid: confirmTarget.pid })}</span>
                <input
                  type="text"
                  inputMode="numeric"
                  autoFocus
                  value={confirmPidInput}
                  onChange={(e) => setConfirmPidInput(e.target.value)}
                  placeholder={String(confirmTarget.pid)}
                  className="h-8 rounded-control bg-inset px-2.5 font-mono text-xs text-ink shadow-hairline outline-none"
                />
              </label>
            )}
          </div>
        )}
      </Dialog>
    </section>
  );
}
