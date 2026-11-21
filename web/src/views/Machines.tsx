// Machines: every computer's CPU / memory / network now and over the last hour (docs/server/machines.md), as Insight Cards. "Add computer"
// (feature machines_add) adds one from here, and a computer waiting to be confirmed (trust pending) says so on its card.
import { memo, Suspense, useCallback, useEffect, useMemo, useState } from "react";
import { lazyPage } from "../lib/lazy-page";
import { storedValue, storeValue } from "../lib/compatibility";
import { concernOf, launcherFailing, machineAlerts, orderMachines, type AlertKind, type LauncherHealthRecord } from "../lib/machine-alerts";
import { t as tr } from "../i18n";
import { canAddComputer, isPending, machineErrorText, neverConnected } from "../lib/add-computer";
import { api, enc, errorText } from "../lib/api";
import { readinessChanged } from "../lib/readiness";
import { stateOf } from "../lib/format";
import { hubLabel, routeText } from "../lib/mesh";
import { machineDeployment } from "../lib/machine-deploy";
import { classifyConnection } from "../lib/node-health";
import { href, navigate, onLinkClick, type Route } from "../lib/router";
import { currentRoster, managerOrder, multiManager, offlineManagerOf, useRoster } from "../lib/managers";
import { defaultManager, withManager } from "../lib/manager-choice";
import { onHubEvent, useStore } from "../lib/store";
import { useFullAgentSessions } from "../lib/sessions";
import { documentPage, visiblePoll } from "../lib/visible-poll";
import type { Machine, MachineMetrics, Session } from "../lib/types";
import { Button, cx, EASE, IconButton, Loading, toast } from "../ui/atoms";
import { Icon } from "../ui/icons";
import { Empty } from "./assistant/parts";
import { AddComputer, type PendingComputer } from "./machines/AddComputer";
import { HelpDialog } from "./assistant/Readiness";
import { MachineDeploy } from "./machines/MachineDeploy";
import { MachineMetricsSummary } from "./machines/MachineMetricsSummary";
import { AlertChips } from "./machines/MachineAlertChips";
import { MachineAlertSheet } from "./machines/MachineAlertSheet";
import { MachineCapabilities } from "./machines/MachineCapabilities";
import { Segmented } from "./assistant/parts";
export { Spark } from "./machines/MachineMetricsSummary";
import { useScopedMachines } from "../lib/current-assistant";
import { t } from "../i18n";
import { addTokens, formatTokens, hitRate, percentText, totalTokens, type UsageTokens } from "../lib/tokens";

// Lazy-loaded on demand when the user switches to the Ports & Processes view (#12352).
const PortsProcessesPanel = lazyPage(() => import("./machines/PortsProcessesPanel").then((m) => m.PortsProcessesPanel));
// Lazy-loaded read-only file browser chunk (keeps Machines initial bundle and first-screen budget untouched).
const FileBrowserPanel = lazyPage(() => import("./machines/FileBrowserPanel").then((m) => m.FileBrowserPanel));

export function Machines({ onToggleSidebar }: { onToggleSidebar: () => void }) {
  // with several assistants, the current one's computers (docs/assistant/managers.md "Clients"); one: as before
  const machines = useScopedMachines();
  const roster = useRoster();
  const loaded = useStore((s) => s.loaded);
  const canAdd = useStore((s) => canAddComputer(s.features));
  const [adding, setAdding] = useState<{ again: PendingComputer | null } | null>(null);
  // Hubs without machines_add get the plain how-to instead of the in-app flow.
  const [helping, setHelping] = useState(false);
  const [answering, setAnswering] = useState("");
  const [metrics, setMetrics] = useState<MachineMetrics[] | null>(null);
  const [error, setError] = useState("");
  // Each computer's agents. A hub that pages its Agents list pushes no sessions, so the store holds none of them: they
  // are read as the Agents list reads them (each parent once, folded; the assistant's conversations are no agents),
  // and until that read is in, a card says so rather than "No agents running".
  const full = useFullAgentSessions();
  const agents = useMemo(() => agentCounts(full.sessions), [full.sessions]);
  const known: AgentsKnown = full.loaded ? "yes" : full.error ? "failed" : "loading";
  const [view, setView] = useState<MachinesView>(() => {
    const saved = storedValue(VIEW_KEY);
    return saved === "capabilities" || saved === "files" ? saved : "cards";
  });
  const chooseView = (v: MachinesView) => {
    if (v !== "procs") storeValue(VIEW_KEY, v);
    setView(v);
  };
  // The alert sheet (#8037): one for the page, outside every card, on one computer's alert.
  const [sheet, setSheet] = useState<{ mid: string; kind: AlertKind } | null>(null);
  const openAlert = useCallback((mid: string, kind: AlertKind) => setSheet({ mid, kind }), []);
  const records = useLauncherHealth();
  const issues = useMemo(() => {
    const by = new Map<string, number>();
    for (const r of records.list) if (launcherFailing(r)) by.set(r.machine, (by.get(r.machine) ?? 0) + 1);
    return by;
  }, [records.list]);
  const launcherIssues = useCallback((mid: string) => issues.get(mid) ?? 0, [issues]);
  // the cards with something wrong first, as the apps order them (MachineConcern)
  const ordered = useMemo(() => orderMachines(machines, (m) => {
    const mm = metrics?.find((x) => x.machine_id === m.id);
    return concernOf(m, machineAlerts({ machine: m, metrics: mm, launchers: issues.get(m.id) ?? 0, managerOffline: !!offlineManagerOf(roster, m) }), agents.get(m.id)?.[2] ?? 0);
  }), [machines, metrics, issues, roster, agents]);

  useEffect(() => {
    let dead = false;
    const previous = new Map<string, MachineMetrics[]>();
    const load = async () => {
      const roster = currentRoster();
      const ids = multiManager(roster) ? managerOrder(roster).filter((m) => m.online).map((m) => m.id) : [defaultManager()];
      const results = await Promise.allSettled(ids.map((id) => api<{ machines: MachineMetrics[] }>(withManager("/metrics", id))));
      if (dead) return;
      const failed = results.find((r) => r.status === "rejected");
      if (failed && results.every((r) => r.status === "rejected")) {
        setError(errorText(failed.reason));
        return;
      }
      const merged = new Map<string, MachineMetrics>();
      results.forEach((r, i) => {
        if (r.status === "fulfilled") previous.set(ids[i], r.value.machines ?? []);
        for (const m of previous.get(ids[i]) ?? []) {
          if (!merged.has(m.machine_id)) merged.set(m.machine_id, m);
        }
      });
      setMetrics([...merged.values()]);
      setError("");
    };
    // Right away and every 10 s while the tab is shown; a hidden tab asks nothing and loads again when shown.
    const stop = visiblePoll(() => void load(), 10_000, documentPage);
    return () => {
      dead = true;
      stop();
    };
  }, []);

  // Stable callbacks, so the memoized cards keep their props when the page renders again.
  const reconnect = useCallback(async (id: string): Promise<void> => {
    try {
      await api(`/machines/${encodeURIComponent(id)}/reconnect`, { method: "POST" });
      toast(t("machines.reconnecting"), "ok");
    } catch (e) {
      toast(t("machines.reconnect_failed", { error: errorText(e) }), "error");
    }
  }, []);

  // It is mine / Not mine for a computer waiting to be confirmed; the hub's next machines push shows the result.
  const answer = useCallback(async (m: { id: string; name: string }, what: "confirm" | "reject") => {
    setAnswering(m.id);
    try {
      await api(`/machines/${enc(m.id)}/${what}`, { method: "POST" });
      toast(t(what === "confirm" ? "machines.confirmed" : "machines.rejected", { name: m.name }), "ok");
      void readinessChanged();
    } catch (e) {
      toast(t("machines.answer_failed", { error: errorText(e) }), "error");
    } finally {
      setAnswering("");
    }
  }, []);
  const addAgain = useCallback((again: PendingComputer) => setAdding({ again }), []);

  const add = (
    <Button tone="primary" icon="plus" onClick={() => (canAdd ? setAdding({ again: null }) : setHelping(true))}>
      {t("machines.add")}
    </Button>
  );

  return (
    <div className="flex h-full flex-col">
      <header className="flex h-14 shrink-0 items-center gap-2 border-b border-line px-3 sm:px-4">
        <IconButton icon="sidebar" label={t("machines.sidebar")} onClick={onToggleSidebar} className="lg:hidden" />
        <h1 className="text-[14px] font-semibold text-ink">{t("machines.title")}</h1>
        <div className="flex-1" />
        {machines.length > 0 && <Segmented compact value={view} onChange={chooseView} label={t("machines.view_label")} options={[{ v: "cards", label: t("machines.view_cards") }, { v: "capabilities", label: t("machines.view_capabilities") }, { v: "procs", label: t("machines.view_procs") }, { v: "files", label: t("machines.view_files") }]} />}
        <span className="text-[11.5px] text-ink-3 max-sm:hidden">{t("machines.refresh_interval", { seconds: 10 })}</span>
        {machines.length > 0 && canAdd && add}
      </header>
      <div className="min-h-0 flex-1 overflow-y-auto">
        {/* two columns, three on a wide screen: the cards used to stop at 1000px and leave half a wide window empty; a full-row item takes col-span-full, since md:col-span-2 alone leaves a third of the row to the next card */}
        <div className="mx-auto grid max-w-[1560px] grid-cols-1 gap-3 px-5 py-6 sm:px-8 md:grid-cols-2 2xl:grid-cols-3">
          <TodayTokens machines={machines} metrics={metrics} />
          {!metrics && !error && <Loading />}
          {/* a failed refresh says so even with the last numbers on screen (iOS / Android keep their notice up) */}
          {error && (
            <p role="alert" className="flex items-start gap-2 rounded-[8px] bg-red-tint px-3 py-2 text-[12.5px] text-red col-span-full">
              <Icon name="warning" size={14} className="mt-px shrink-0" />
              <span className="min-w-0 break-words">{metrics ? t("machines.refresh_failed_stale", { error, seconds: 10 }) : t("machines.load_failed", { error })}</span>
            </p>
          )}
          {loaded && machines.length === 0 && (metrics || error) && (
            <div className="col-span-full">
              <Empty icon="machine" title={error && !metrics ? t("machines.empty_unavailable") : t("machines.empty_title")} action={add}>
                {t("machines.empty_body_add")}
              </Empty>
            </div>
          )}
          {view === "capabilities" && machines.length > 0 && (
            <div className="col-span-full">
              <MachineCapabilities machines={machines} metrics={metrics} launcherIssues={launcherIssues} onAlert={openAlert} />
            </div>
          )}
          {view === "procs" && machines.length > 0 && (
            <div className="col-span-full">
              <Suspense fallback={<Loading />}>
                <PortsProcessesPanel machines={machines} />
              </Suspense>
            </div>
          )}
          {view === "files" && machines.length > 0 && (
            <div className="col-span-full">
              <Suspense fallback={<Loading />}>
                <FileBrowserPanel machines={machines} />
              </Suspense>
            </div>
          )}
          {view === "cards" && ordered.map((m, index) => (
              <MachineCard
                key={m.id}
                m={m}
                index={index}
                metrics={metrics?.find((x) => x.machine_id === m.id)}
                hub={hubLabel(m, machines)}
                route={routeText(m, machines)}
                canAdd={canAdd}
                answering={answering === m.id}
                onReconnect={reconnect}
                onAnswer={answer}
                onAddAgain={addAgain}
                managerOffline={offlineManagerOf(roster, m)?.name}
                agents={agents.get(m.id)?.[0] ?? 0}
                working={agents.get(m.id)?.[1] ?? 0}
                known={known}
                launcherIssues={issues.get(m.id) ?? 0}
                onAlert={openAlert}
              />
          ))}
        </div>
      </div>
      <AlertSheetHost sheet={sheet} machines={machines} metrics={metrics} sessions={full.sessions} known={known} records={records} launcherIssues={launcherIssues} roster={roster} onKind={openAlert} onClose={() => setSheet(null)} onReconnect={reconnect} />
      <AddComputer open={!!adding} again={adding?.again} onClose={() => setAdding(null)} />
      {helping && <HelpDialog topic="add_computer" machine="" step={null} onClose={() => setHelping(false)} onAction={() => {}} />}
    </div>
  );
}

type CardProps = {
  m: Machine;
  index: number;
  metrics: MachineMetrics | undefined;
  hub: string;
  route: string;
  canAdd: boolean;
  answering: boolean;
  onReconnect: (id: string) => void;
  onAnswer: (m: { id: string; name: string }, what: "confirm" | "reject") => Promise<void>;
  onAddAgain: (again: PendingComputer) => void;
  /** Its manager's name when that manager is offline (several managers): why it is offline, and no Reconnect. */
  managerOffline?: string;
  /** Its agents that are not offline, and how many of them work; `known`: whether the list they come from is in. */
  agents: number;
  working: number;
  known: AgentsKnown;
  /** Launchers failing or blocked on it (docs/server/launcher-health.md). */
  launcherIssues: number;
  /** Opens the alert sheet on one of its alerts. */
  onAlert: (mid: string, kind: AlertKind) => void;
};

type AgentsKnown = "yes" | "loading" | "failed";

// One computer's card. It is memoized and gets its own computer's agents as numbers: a change to another computer's
// agents leaves its props as they were, so it (and its Stat, Spark, MachineDeploy) does not render again. Nor does a
// machines push that only moved last_seen on (sameCard).
const MachineCard = memo(function MachineCard({ m, index, metrics: mm, hub, route, canAdd, answering, onReconnect, onAnswer, onAddAgain, managerOffline, agents, working, known, launcherIssues, onAlert }: CardProps) {
  const detail = { name: "machine" as const, mid: m.id };
  const { expectedDisconnect } = machineDeployment(m);
  const pending = isPending(m);
  // why it is offline: the hub's error_code in our words, else from the error text, and what to do
  const coded = pending ? "" : machineErrorText(m.error_code);
  const connFailure = expectedDisconnect || pending || coded || managerOffline ? null : classifyConnection(m);
  // what is wrong, as chips (#8037); the deployment strip below says the version's and a failed update's itself
  const alerts = machineAlerts({ machine: m, metrics: mm, launchers: launcherIssues, managerOffline: !!managerOffline }).filter((a) => a.kind !== "mismatch" && a.kind !== "deploy_failed");
  const versions = [m.claude_version && `claude ${m.claude_version}`, m.herdr_version && `herdr ${m.herdr_version}`].filter(Boolean).join(" · ");
  return (
    <section onClick={(e) => openFromCard(e, detail)} className="cursor-pointer overflow-hidden rounded-card bg-surface shadow-card" style={{ animation: `fade-up 420ms ${EASE} ${index * 60}ms both` }}>
      <div className="flex items-start gap-3 px-4 pt-3.5">
        <span className={cx("flex size-9 shrink-0 items-center justify-center rounded-[9px]", m.online ? "bg-field text-ink-2" : "bg-inset text-ink-3")}>
          <Icon name="machine" size={16} />
        </span>
        <a href={href(detail)} onClick={(e) => onLinkClick(e, detail)} className="group/title min-w-0 flex-1 rounded-[6px]">
          <div className="flex items-center gap-2">
            <h2 className="truncate text-[14px] font-semibold text-ink group-hover/title:underline">{m.name}</h2>
            {hub && (
              <span title={route ? `${hub} · ${route}` : hub} className="inline-flex h-5 shrink-0 items-center gap-1 whitespace-nowrap rounded-full bg-field px-1.5 text-[11px] font-medium text-ink-2">
                <Icon name="hub" size={11} />
                {hub}
              </span>
            )}
            <span className={cx("inline-flex h-5 items-center gap-1 rounded-full px-1.5 text-[11px] font-medium", m.online ? "bg-green-tint text-green" : "bg-field text-ink-3")}>
              <span className={cx("size-1.5 rounded-full", m.online ? "bg-green" : "bg-ink-3")} />
              {m.online ? t("machines.online") : neverConnected(m) ? t("machines.not_connected_yet") : t("machines.offline")}
            </span>
          </div>
          <p className="truncate text-[11.5px] text-ink-3">{[m.system?.name || m.os, m.system?.model, m.system?.cpu].filter(Boolean).join(" · ") || m.hostname}</p>
        </a>
        {!m.online && !expectedDisconnect && !pending && !managerOffline && (
          <Button tone="secondary" icon="retry" onClick={() => onReconnect(m.id)}>
            {t("machines.reconnect")}
          </Button>
        )}
      </div>
      {pending && (
        <div className="mx-4 mt-2 flex flex-wrap items-center gap-2 rounded-[8px] bg-orange-tint px-2.5 py-2 text-[12px] text-ink" data-trust="pending">
          <Icon name="shield" size={14} className="shrink-0 text-orange" />
          <span className="min-w-0 flex-1 basis-48">{m.online ? t("machines.pending_joined", { name: m.name }) : t("machines.pending_waiting")}</span>
          <span className="flex shrink-0 gap-1.5">
            {m.online ? (
              <>
                <Button tone="secondary" disabled={answering} onClick={() => void onAnswer(m, "reject")}>
                  {t("machines.reject")}
                </Button>
                <Button tone="primary" busy={answering} onClick={() => void onAnswer(m, "confirm")}>
                  {t("machines.confirm")}
                </Button>
              </>
            ) : (
              <>
                <Button tone="secondary" disabled={answering} onClick={() => void onAnswer(m, "reject")}>
                  {t("machines.remove")}
                </Button>
                {canAdd && (
                  <Button tone="primary" onClick={() => onAddAgain({ id: m.id, name: m.name })}>
                    {t("machines.new_code")}
                  </Button>
                )}
              </>
            )}
          </span>
        </div>
      )}
      {m.online && !pending && (coded || m.error) && <p className="mx-4 mt-2 rounded-[8px] bg-red-tint px-2.5 py-1.5 text-[12px] text-red">{coded || m.error}</p>}
      {/* its manager is offline: say so, not the hub's words for an unreachable assistant */}
      {!m.online && managerOffline && (
        <p className="mx-4 mt-2 break-words rounded-[8px] bg-field px-2.5 py-1.5 text-[12px] text-ink-2" data-error-code="manager_offline" title={m.error}>
          {t("settings-managers.machine_offline", { name: managerOffline })}
        </p>
      )}
      {!m.online && coded && !expectedDisconnect && !managerOffline && (
        <p className="mx-4 mt-2 break-words rounded-[8px] bg-red-tint px-2.5 py-1.5 text-[12px] text-red" data-error-code={m.error_code} title={m.error}>
          {coded}
        </p>
      )}
      {connFailure && (
        <p className="mx-4 mt-2 break-words rounded-[8px] bg-red-tint px-2.5 py-1.5 text-[12px] text-red" data-failure={connFailure.kind}>
          <span className="font-medium">{tr("node-health.failure_title", { title: connFailure.title })}</span>
          {connFailure.next}
        </p>
      )}
      {/* an offline card has no metrics under them: the chips keep their own room above the footer */}
      <AlertChips alerts={alerts} onOpen={(kind) => onAlert(m.id, kind)} className={m.online ? undefined : "mb-3"} />
      <MachineDeploy machine={m} className="mt-2" onDetails={() => onAlert(m.id, m.deploy_error ? "deploy_failed" : "mismatch")} />
      <MachineMetricsSummary machine={m} metrics={mm} />
      {/* the gap keeps the state and the versions apart when the versions have to shorten */}
      <div className="flex items-center justify-between gap-3 border-t border-line px-4 py-2 text-[11.5px] text-ink-3">
        <span className="whitespace-nowrap" data-agents={m.online && !pending ? known : undefined}>{pending && !m.online ? t("machines.never_connected") : !m.online ? expectedDisconnect ? t("machines.waiting_node") : t("machines.unreachable") : known === "loading" ? t("machines-machinedetail.agents_loading") : known === "failed" ? t("machines-machinedetail.agents_failed") : agents ? working ? t("machines.agents_working", { count: agents, working }) : t("machines.agents", { count: agents }) : t("machines.no_agents")}</span>
        <span className="flex min-w-0 items-center gap-3">
          <span className="truncate font-mono max-sm:hidden" title={versions || undefined}>{versions}</span>
          <a href={href(detail)} onClick={(e) => onLinkClick(e, detail)} className="shrink-0 whitespace-nowrap text-ink-2 hover:text-ink">{t("machines.details")}</a>
        </span>
      </div>
    </section>
  );
}, sameCard);

// Every machines push brings new machine objects with last_seen moved on. The card reads last_seen only for whether
// there is one (neverConnected), so a machine that differs in nothing else draws the same card.
function sameCard(a: CardProps, b: CardProps): boolean {
  for (const k of Object.keys(b) as (keyof CardProps)[]) if (k !== "m" && !Object.is(a[k], b[k])) return false;
  return a.m === b.m || cardKey(a.m) === cardKey(b.m);
}

const cardKey = (m: Machine) => JSON.stringify({ ...m, last_seen: !!m.last_seen });

/** By computer: its agents that are not offline (the assistant's conversations are not agents), and how many of them
 *  work. */
function agentCounts(list: readonly Session[]): Map<string, readonly [number, number, number]> {
  const by = new Map<string, [number, number, number]>();
  for (const s of list) {
    if (s.assistant) continue;
    const st = stateOf(s);
    if (st === "offline") continue;
    const n = by.get(s.machine_id) ?? [0, 0, 0];
    n[0]++;
    if (st === "working") n[1]++;
    if (st === "needs_you") n[2]++;
    by.set(s.machine_id, n);
  }
  return by;
}

type MachinesView = "cards" | "capabilities" | "procs" | "files";
const VIEW_KEY = "persona.machines.view";

/** The launcher health records (GET /api/v1/launchers/health), kept current by the hub's `launcher_health` events and
 *  read again every minute while the page is shown; none from a hub without them. `merge` takes records a check returned. */
function useLauncherHealth(): { list: LauncherHealthRecord[]; merge: (records: LauncherHealthRecord[]) => void } {
  const [list, setList] = useState<LauncherHealthRecord[]>([]);
  const merge = useCallback((records: LauncherHealthRecord[]) => {
    if (!records.length) return;
    setList((now) => {
      const key = (r: LauncherHealthRecord) => `${r.machine}\u0000${r.launcher}`;
      const next = new Map(now.map((r) => [key(r), r]));
      for (const r of records) next.set(key(r), r);
      return [...next.values()];
    });
  }, []);
  useEffect(() => {
    let dead = false;
    const load = () => {
      api<{ records?: LauncherHealthRecord[] }>("/launchers/health").then(
        (r) => !dead && setList(r.records ?? []),
        () => {}, // an older hub has none: no launcher alerts
      );
    };
    const stop = visiblePoll(load, 60_000, documentPage);
    const off = onHubEvent("launcher_health", (msg) => {
      const record = (msg as { record?: LauncherHealthRecord }).record;
      if (record?.machine && record.launcher) merge([record]);
    });
    return () => {
      dead = true;
      stop();
      off();
    };
  }, [merge]);
  return { list, merge };
}

/** The alert sheet for the computer and alert asked for; closed (with its exit) when none is. */
function AlertSheetHost({ sheet, machines, metrics, sessions, known, records, launcherIssues, roster, onKind, onClose, onReconnect }: {
  sheet: { mid: string; kind: AlertKind } | null;
  machines: Machine[];
  metrics: MachineMetrics[] | null;
  sessions: readonly Session[];
  known: AgentsKnown;
  records: { list: LauncherHealthRecord[]; merge: (records: LauncherHealthRecord[]) => void };
  launcherIssues: (mid: string) => number;
  roster: ReturnType<typeof useRoster>;
  onKind: (mid: string, kind: AlertKind) => void;
  onClose: () => void;
  onReconnect: (id: string) => Promise<void>;
}) {
  const m = sheet ? machines.find((x) => x.id === sheet.mid) : undefined;
  const mm = m ? metrics?.find((x) => x.machine_id === m.id) : undefined;
  const agents = useMemo(() => (m ? sessions.filter((s) => s.machine_id === m.id && !s.assistant && stateOf(s) !== "offline") : []), [m, sessions]);
  const alerts = m ? machineAlerts({ machine: m, metrics: mm, launchers: launcherIssues(m.id), managerOffline: !!offlineManagerOf(roster, m) }) : [];
  return (
    <MachineAlertSheet
      machine={m}
      kind={sheet?.kind ?? "memory"}
      alerts={alerts}
      metrics={mm}
      agents={agents}
      agentsKnown={known}
      machines={machines}
      allMetrics={metrics}
      records={records.list}
      launcherIssues={launcherIssues}
      onKind={(kind) => m && onKind(m.id, kind)}
      onClose={onClose}
      onReconnect={onReconnect}
      onRecords={records.merge}
    />
  );
}

// Elements that keep their own click inside a card (the links, Reconnect, Deploy …).
const INTERACTIVE = "a,button,input,select,textarea,label,summary,[role=button],[tabindex]";

// A tap anywhere else on a machine's card opens its page (#3578): on a phone the "Details" link alone is a 48 × 17 target.
// The card stays a plain section; the title link is what keyboards and screen readers use, so no control nests in another.
function openFromCard(e: React.MouseEvent<HTMLElement>, to: Route) {
  const own = e.target instanceof Element ? e.target.closest(INTERACTIVE) : null;
  if (own && e.currentTarget.contains(own)) return;
  if (e.button !== 0 || window.getSelection()?.toString()) return; // selecting the error text to copy it is not a tap
  navigate(to);
}

// Today's tokens across the computers (#4973): the total, every token per minute now, the cache hit rate, each
// computer's share in shades of the text ink (the four kinds' colors are the charts').
function TodayTokens({ machines, metrics }: { machines: Machine[]; metrics: MachineMetrics[] | null }) {
  const shares = machines
    .map((m) => ({ id: m.id, name: m.name, usage: metrics?.find((x) => x.machine_id === m.id)?.usage }))
    .filter((s): s is { id: string; name: string; usage: NonNullable<MachineMetrics["usage"]> } => !!s.usage);
  if (!shares.length) return null;
  const today = shares.reduce<UsageTokens>((a, s) => addTokens(a, s.usage.today), {});
  const all = totalTokens(today);
  const ranked = shares.filter((s) => totalTokens(s.usage.today) > 0).sort((a, b) => totalTokens(b.usage.today) - totalTokens(a.usage.today));
  const shade = (i: number) => Math.max(0.85 - i * 0.2, 0.2);
  return (
    <section className="rounded-card bg-surface px-4 py-3 shadow-card col-span-full" data-testid="machines-tokens">
      <h2 className="text-[13px] font-medium text-ink">{t("machines-usage.today_title")}</h2>
      <div className="mt-2 flex flex-wrap items-end gap-x-8 gap-y-2">
        <Stat label={t("machines-usage.total")} value={formatTokens(all)} />
        <Stat label={t("machines-usage.now")} value={t("machines-usage.per_minute_value", { n: formatTokens(shares.reduce((a, s) => a + s.usage.per_minute, 0)) })} />
        <Stat label={t("machines-usage.hit_rate_title")} value={percentText(hitRate(today))} />
        {all > 0 && (
          <div className="min-w-[220px] flex-1 pb-1">
            <div className="flex h-2 gap-0.5 overflow-hidden rounded-full" aria-hidden>
              {ranked.map((s, i) => (
                <div key={s.id} className="h-2 bg-ink" style={{ flexGrow: totalTokens(s.usage.today), opacity: shade(i) }} />
              ))}
            </div>
            <div className="mt-1.5 flex flex-wrap gap-x-3 gap-y-0.5 text-[11.5px]">
              {ranked.map((s, i) => (
                <span key={s.id} className="flex items-center gap-1 whitespace-nowrap">
                  <span aria-hidden className="inline-block size-2 rounded-full bg-ink" style={{ opacity: shade(i) }} />
                  <span className="text-ink-2">{s.name}</span>
                  <span className="text-ink-3 tabular-nums">{formatTokens(totalTokens(s.usage.today))}</span>
                </span>
              ))}
            </div>
          </div>
        )}
      </div>
    </section>
  );
}

function Stat({ label, value, sub, warn }: { label: string; value: string; sub?: string; warn?: boolean }) {
  return (
    <div className="min-w-0">
      <div className="text-[11px] text-ink-3">{label}</div>
      <div className={cx("font-mono text-[18px] leading-tight font-medium tabular-nums", warn ? "text-red" : "text-ink")}>{value}</div>
      {sub && <div className="truncate text-[10.5px] text-ink-3">{sub}</div>}
    </div>
  );
}
