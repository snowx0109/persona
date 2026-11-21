import { util } from '@kit.ArkTS';
import { HttpFailure } from '../platform/HubTransport';
import { ResourceRow, Wire } from './Wire';
import type { ClientModel } from './ClientModel';

const VIEW_PREFIX: string = 'admin-workspace:';

function segment(value: string): string { return encodeURIComponent(value); }
function operationId(): string { return 'admin-' + Date.now().toString(36) + '-' + Math.floor(Math.random() * 0x7fffffff).toString(36); }
function errorCode(error: Error): string {
  if (error instanceof HttpFailure) {
    if (error.resultPending) return 'pending';
    if (error.resultUnknown) return 'result_unknown';
    if (error.status === 404 && (!error.code || error.code === 'http_error' || error.code === 'request_failed')) return 'unsupported';
    if (error.status === 404) return 'not_found';
    if (error.status === 409) return 'request_updated';
    if (error.status === 401 || error.status === 403) return 'permission_denied';
  }
  return error.message === 'offline' ? 'offline' : 'failed';
}

export class AdminSnapshot {
  page: string = '';
  revision: number = 0;
  profile: string = '';
  scope: string = '';
  manager: string = '';
  generation: number = 0;
  selection: string = '';
  routeTarget: string = '';
  constructor(page: string, revision: number, profile: string, scope: string, manager: string, generation: number,
    selection: string, routeTarget: string = '') {
    this.page = page; this.revision = revision; this.profile = profile; this.scope = scope; this.manager = manager;
    this.generation = generation; this.selection = selection; this.routeTarget = routeTarget;
  }
}

export class AdminManager {
  id: string = '';
  name: string = '';
  online: boolean = false;
  isDefault: boolean = false;
  builtin: boolean = false;
  host: string = '';
  os: string = '';
  deploy: string = '';
  version: string = '';
  installing: string = '';
  lastSeen: string = '';
  raw: Record<string, Object> = {};
  constructor(raw: Record<string, Object> = {}) {
    this.id = Wire.text(raw, 'id'); this.name = Wire.text(raw, 'name', this.id);
    this.online = Wire.bool(raw, 'online'); this.isDefault = Wire.bool(raw, 'default'); this.builtin = Wire.bool(raw, 'builtin');
    this.host = Wire.text(raw, 'host'); this.os = Wire.text(raw, 'os'); this.deploy = Wire.text(raw, 'deploy');
    this.version = Wire.text(raw, 'version'); this.installing = Wire.text(raw, 'installing'); this.lastSeen = Wire.text(raw, 'last_seen'); this.raw = raw;
  }
}

export class AdminMemory {
  name: string = '';
  content: string = '';
  updatedAt: string = '';
  constructor(raw: Record<string, Object> = {}) {
    this.name = Wire.text(raw, 'name'); this.content = Wire.text(raw, 'content'); this.updatedAt = Wire.text(raw, 'updated_at');
  }
}

/** R2 administration using only the documented authenticated HubRepository routes. */
@ObservedV2
export class AdminWorkspace {
  @Trace page: string = '';
  @Trace visible: boolean = false;
  @Trace loading: boolean = false;
  @Trace saving: boolean = false;
  @Trace error: string = '';
  @Trace refreshFailed: boolean = false;
  @Trace unsupported: boolean = false;
  @Trace writeState: string = '';
  @Trace draftJson: string = '{}';
  @Trace draftDirty: boolean = false;
  @Trace assistantData: Record<string, Object> = {};
  @Trace memoryRows: AdminMemory[] = [];
  @Trace selectedMemory: string = '';
  @Trace memoryDraft: string = '';
  @Trace memoryDirty: boolean = false;
  @Trace newMemoryName: string = '';
  @Trace newMemoryContent: string = '';
  @Trace managers: AdminManager[] = [];
  @Trace defaultManager: string = '';
  @Trace selectedManager: string = '';
  @Trace managerNameDraft: string = '';
  @Trace selectedMachine: string = '';
  @Trace machineRows: ResourceRow[] = [];
  @Trace machineData: Record<string, Object> = {};
  @Trace selectedEngine: string = '';
  @Trace selectedEngineHosts: string[] = [];
  @Trace selectedEngineDefault: string = '';
  @Trace engineError: string = '';
  @Trace selectedMoveMachine: string = '';
  @Trace selectedMoveDestination: string = '';
  @Trace launchers: Record<string, Object>[] = [];
  @Trace selectedLauncher: string = '';
  @Trace launcherDetail: Record<string, Object> = {};
  @Trace launcherEditor: boolean = false;
  @Trace launcherNew: boolean = false;
  @Trace launcherSecretDraft: string = '';
  @Trace presets: Record<string, Object>[] = [];
  @Trace importer: Record<string, Object> = {};
  @Trace importerSelection: string[] = [];
  @Trace importerMachine: string = '';
  @Trace importBusy: boolean = false;
  @Trace healthRecords: Record<string, Object>[] = [];
  @Trace healthResult: Record<string, Object> = {};
  @Trace hubUpdate: Record<string, Object> = {};
  @Trace updateHistory: Record<string, Object>[] = [];
  @Trace updateHistoryUnsupported: boolean = false;
  @Trace installCode: Record<string, Object> = {};
  @Trace notice: string = '';

  private root: ClientModel | undefined;
  private routeMachine: ResourceRow | undefined;
  private rootProfile: string = '';
  private rootScope: string = '';
  private rootManager: string = '';
  private rootGeneration: number = 0;
  private revision: number = 0;
  private readEpoch: number = 0;
  private busySequence: number = 0;
  private loadingOwner: number = 0;
  private savingOwner: number = 0;
  private draftRevision: number = 0;
  private launcherSecretRevision: number = 0;
  private memoryDraftRevision: number = 0;
  private newMemoryRevision: number = 0;
  private draftQueue: Promise<void> = Promise.resolve();
  private machineOwner: ClientModel | undefined;
  private machineOwnerGeneration: number = 0;
  private machineOwnerScope: string = '';
  private managerMachineRows: ResourceRow[] = [];
  private managerMachineManager: string = '';
  private lastPage: string = '';

  /** Open the visible route; reopening it in the same owner preserves unsubmitted form and memory drafts. */
  async open(model: ClientModel, page: string, active: boolean, row?: ResourceRow): Promise<void> {
    if (!active) { this.hide(); return; }
    const generationChanged = this.root === model && this.rootGeneration !== model.connectionGeneration();
    const sameOwner = this.rootProfile === model.profile.id && this.rootScope === model.scope() && this.rootManager === model.profile.manager;
    const sameRoute = sameOwner && this.page === page;
    if (!sameRoute || generationChanged) this.resetBusy();
    this.visible = true; this.root = model; this.routeMachine = row;
    this.rootProfile = model.profile.id; this.rootScope = model.scope(); this.rootManager = model.profile.manager;
    this.rootGeneration = model.connectionGeneration(); this.lastPage = page;
    if (!sameRoute) {
      this.revision++; this.page = page; this.loading = false; this.saving = false; this.error = ''; this.unsupported = false;
      this.refreshFailed = false; this.writeState = ''; this.notice = ''; this.installCode = {};
      this.assistantData = {}; this.memoryRows = []; this.managers = []; this.launchers = []; this.launcherDetail = {};
      this.presets = []; this.importer = {}; this.healthRecords = []; this.healthResult = {}; this.hubUpdate = {};
      this.updateHistory = []; this.updateHistoryUnsupported = false; this.machineData = {}; this.engineError = '';
      this.machineRows = this.buildMachineRows(model, row); this.managerMachineRows = []; this.managerMachineManager = '';
      this.launcherEditor = false; this.launcherNew = false; this.launcherSecretDraft = ''; this.importBusy = false;
      this.launcherSecretRevision++;
      this.importerSelection = []; this.importerMachine = ''; this.memoryDirty = false; this.memoryDraft = ''; this.selectedMemory = '';
      this.newMemoryName = ''; this.newMemoryContent = '';
      this.selectedLauncher = ''; this.selectedManager = ''; this.selectedMachine = row?.id || '';
      this.selectedEngineHosts = []; this.selectedEngineDefault = ''; this.selectedMoveMachine = ''; this.selectedMoveDestination = '';
      this.draftJson = '{}'; this.draftDirty = false; this.draftRevision++;
      this.memoryDraftRevision++;
      await this.restoreLocalDraft(model, page, this.revision);
    } else {
      this.machineRows = this.buildMachineRows(model, row);
      if (row?.id && row.id !== this.selectedMachine) {
        this.revision++; this.selectedMachine = row.id; this.machineData = {}; this.selectedEngine = ''; this.engineError = '';
      }
    }
    if (!this.isCurrentRoot()) return;
    if (!this.selectedMachine) this.selectedMachine = this.machineChoices()[0]?.id || '';
    await this.refresh();
  }

  hide(): void {
    if (!this.visible) return;
    this.visible = false; this.revision++; this.resetBusy(); this.importBusy = false;
    this.launcherSecretDraft = ''; this.launcherSecretRevision++; this.root = undefined; this.machineOwner = undefined; this.draftRevision++; this.memoryDraftRevision++;
  }

  /** A route/selection lease for dialogs and mutations. */
  capture(target: string = ''): AdminSnapshot {
    const model = this.root;
    const snapshot = new AdminSnapshot(this.page, this.revision, this.rootProfile, this.rootScope, this.rootManager,
      this.rootGeneration, this.selectionKey(), target);
    return snapshot;
  }

  admits(snapshot: AdminSnapshot | undefined, target: string = ''): boolean {
    const model = this.root;
    return !!snapshot && this.visible && !!model && snapshot.revision === this.revision && snapshot.page === this.page &&
      snapshot.profile === model.profile.id && snapshot.scope === model.scope() && snapshot.scope === this.rootScope &&
      snapshot.manager === model.profile.manager && snapshot.generation === this.rootGeneration &&
      snapshot.selection === this.selectionKey() && (!target || snapshot.routeTarget === target) && model.owns(snapshot.generation, snapshot.scope);
  }

  private resetBusy(): void {
    if (this.saving) { this.writeState = 'result_unknown'; this.error = 'result_unknown'; }
    this.loadingOwner = ++this.busySequence; this.loading = false;
    this.savingOwner = ++this.busySequence; this.saving = false;
  }

  private isCurrentRoot(): boolean {
    const model = this.root;
    return this.visible && !!model && model.profile.id === this.rootProfile && model.scope() === this.rootScope &&
      model.profile.manager === this.rootManager && model.connectionGeneration() === this.rootGeneration &&
      model.owns(this.rootGeneration, this.rootScope);
  }

  private selectionKey(): string {
    if (this.page === 'managers') return JSON.stringify([this.selectedManager, this.selectedMoveMachine, this.selectedMoveDestination]);
    if (this.page === 'launchers') return JSON.stringify([this.selectedLauncher, this.selectedMachine, this.importerMachine, this.importerSelection]);
    if (this.page === 'engine' || this.page === 'node_update') return this.selectedMachine;
    if (this.page === 'memory') return this.selectedMemory;
    return this.rootProfile;
  }

  private buildMachineRows(model: ClientModel, row?: ResourceRow): ResourceRow[] {
    const rows = model.machines.slice();
    if (row && row.id && !rows.some((item: ResourceRow) => item.id === row.id && (item.manager || model.profile.manager) === (row.manager || model.profile.manager))) rows.unshift(row);
    return rows;
  }

  machineChoices(): ResourceRow[] {
    return this.machineRows.filter((row: ResourceRow) => {
      const data = row.data(); const trust = Wire.text(data, 'trust');
      return Wire.bool(data, 'online') && !Wire.bool(data, 'pending') && !Wire.bool(data, 'pending_confirm') && trust !== 'pending' &&
        (!trust || ['confirmed', 'personal', 'work'].includes(trust));
    });
  }

  machineName(id: string): string {
    const row = this.machineRows.find((item: ResourceRow) => item.id === id);
    return row ? row.title || row.id : id;
  }

  selectMachine(id: string): void {
    if (id === this.selectedMachine) return;
    this.revision++; this.selectedMachine = id; this.machineData = {}; this.selectedEngine = ''; this.selectedEngineHosts = [];
    this.selectedEngineDefault = ''; this.engineError = '';
    this.machineOwner = undefined;
    if (this.page === 'launchers') { this.importer = {}; this.importerSelection = []; this.importerMachine = ''; }
  }

  selectManager(id: string): void {
    if (id === this.selectedManager) return;
    this.revision++; this.selectedManager = id;
    const manager = this.managers.find((row: AdminManager) => row.id === id);
    this.managerNameDraft = manager?.name || ''; this.draftDirty = false;
    this.draftJson = JSON.stringify(Wire.payload(['name'], [this.managerNameDraft])); this.draftRevision++;
    this.selectedMoveMachine = ''; this.selectedMoveDestination = '';
  }

  setManagerNameDraft(value: string): void {
    this.managerNameDraft = value; this.draftJson = JSON.stringify(Wire.payload(['name'], [value]));
    this.draftDirty = true; this.draftRevision++; this.persistDraft();
  }

  selectMoveMachine(id: string): void {
    if (id === this.selectedMoveMachine) return;
    this.revision++; this.selectedMoveMachine = id;
  }

  selectMoveDestination(id: string): void {
    if (id === this.selectedMoveDestination) return;
    this.revision++; this.selectedMoveDestination = id;
  }

  selectLauncher(name: string): void {
    if (name === this.selectedLauncher) return;
    this.revision++; this.selectedLauncher = name; this.launcherDetail = {};
    this.launcherEditor = false; this.launcherNew = false; this.launcherSecretDraft = ''; this.launcherSecretRevision++;
  }

  selectMemory(name: string): void {
    if (name === this.selectedMemory) return;
    this.revision++; this.selectedMemory = name;
    const row = this.memoryRows.find((item: AdminMemory) => item.name === name);
    this.memoryDraft = row?.content || ''; this.memoryDirty = false; this.memoryDraftRevision++;
  }

  form(): Record<string, Object> { return Wire.parse(this.draftJson); }
  formText(key: string, fallback: string = ''): string { return Wire.text(this.form(), key, fallback); }
  formBool(key: string, fallback: boolean = false): boolean { return Wire.bool(this.form(), key, fallback); }
  formValue(key: string, fallback: string = ''): string { return String(this.form()[key] ?? fallback); }

  setForm(key: string, value: Object): void {
    const form = this.form(); form[key] = value; this.draftJson = JSON.stringify(form); this.draftDirty = true;
    this.draftRevision++; this.persistDraft();
  }

  private async restoreLocalDraft(model: ClientModel, page: string, revision: number): Promise<void> {
    try {
      const saved = Wire.parse(await model.loadView(VIEW_PREFIX + page));
      if (!this.visible || this.root !== model || this.revision !== revision || !this.isCurrentRoot()) return;
      if (Wire.bool(saved, 'dirty')) {
        this.draftJson = Wire.text(saved, 'draft', '{}'); this.draftDirty = true; this.draftRevision++;
      }
      if (this.page === 'managers') this.selectedManager = Wire.text(saved, 'selected_manager');
      if (this.page === 'managers' && Wire.bool(saved, 'dirty')) this.managerNameDraft = Wire.text(Wire.parse(this.draftJson), 'name');
      if (this.page === 'launchers') this.selectedLauncher = Wire.text(saved, 'selected_launcher');
      if (['engine', 'node_update'].includes(this.page)) this.selectedMachine = Wire.text(saved, 'selected_machine', this.selectedMachine);
    } catch (_) { /* A local draft read failure does not block a read-only page load. */ }
  }

  private persistDraft(): void {
    const model = this.root; const revision = this.revision; const page = this.page; const draftVersion = this.draftRevision;
    if (!model || !this.visible || !this.draftDirty) return;
    const scope = model.scope(), manager = model.profile.manager, profile = model.profile.id;
    const raw = JSON.stringify(Wire.payload(['dirty', 'draft', 'selected_manager', 'selected_launcher', 'selected_machine'],
      [true, this.draftJson, this.selectedManager, this.selectedLauncher, this.selectedMachine]));
    this.draftQueue = this.draftQueue.catch(() => {}).then(async (): Promise<void> => {
      if (this.root !== model || !this.visible || this.revision !== revision || this.page !== page ||
        model.scope() !== scope || model.profile.manager !== manager || model.profile.id !== profile || this.draftRevision !== draftVersion) return;
      const expectedEpoch = model.viewRevision(VIEW_PREFIX + page);
      await model.saveView(VIEW_PREFIX + page, raw, expectedEpoch);
    }).catch(() => { if (this.root === model && this.visible && this.revision === revision && this.draftRevision === draftVersion) this.error = 'admin_storage_failed'; });
  }

  private setRemoteForm(value: Record<string, Object>): void {
    if (this.draftDirty) return;
    this.draftJson = JSON.stringify(value); this.draftRevision++;
  }

  private admitsRead(snapshot: AdminSnapshot, epoch: number): boolean { return this.admits(snapshot) && epoch === this.readEpoch; }

  private async read(snapshot: AdminSnapshot, path: string): Promise<Record<string, Object> | undefined> {
    const model = this.root, epoch = this.readEpoch;
    if (!model || !this.admitsRead(snapshot, epoch)) return undefined;
    try {
      const result = await model.repo().call(path, 'GET', {}, '', (): boolean => this.admitsRead(snapshot, epoch));
      return this.admitsRead(snapshot, epoch) ? result : undefined;
    } catch (failure) {
      if (this.admitsRead(snapshot, epoch)) { this.error = errorCode(failure as Error); this.unsupported = this.error === 'unsupported'; this.refreshFailed = true; }
      return undefined;
    }
  }

  private async machineClient(snapshot: AdminSnapshot): Promise<ClientModel | undefined> {
    const root = this.root; const row = this.machineRows.find((item: ResourceRow) => item.id === this.selectedMachine);
    if (!root || !row || !this.admits(snapshot)) return undefined;
    try {
      const client = await root.viewFor(row);
      if (!this.admits(snapshot) || !client.connected || client.profile.id !== snapshot.profile ||
        client.profile.manager !== (row.manager || snapshot.manager) || !client.owns(client.connectionGeneration(), client.scope())) return undefined;
      this.machineOwner = client; this.machineOwnerGeneration = client.connectionGeneration(); this.machineOwnerScope = client.scope();
      return client;
    } catch (failure) {
      if (this.admits(snapshot)) this.error = errorCode(failure as Error);
      return undefined;
    }
  }

  private async readMachine(snapshot: AdminSnapshot): Promise<void> {
    const epoch = this.readEpoch;
    const client = await this.machineClient(snapshot);
    if (!client || !this.admitsRead(snapshot, epoch)) return;
    const generation = client.connectionGeneration(), scope = client.scope(), machineID = this.selectedMachine;
    try {
      const list = await client.repo().call('/api/v1/machines', 'GET', {}, '', (): boolean => this.admitsRead(snapshot, epoch) &&
        client.owns(generation, scope) && machineID === this.selectedMachine);
      if (!this.admitsRead(snapshot, epoch) || !client.owns(generation, scope) || machineID !== this.selectedMachine) return;
      const row = Wire.rows(list, 'machines').find((item: Record<string, Object>) => Wire.text(item, 'id') === machineID);
      if (!row) { this.error = 'not_found'; this.machineData = {}; return; }
      this.machineData = row; this.selectedEngine = Wire.text(row, 'engine', 'herdr');
      if (Wire.has(row, 'engine_hosts')) {
        this.selectedEngineHosts = Wire.strings(row, 'engine_hosts'); this.selectedEngineDefault = Wire.text(row, 'engine_default');
      } else {
        this.selectedEngineHosts = this.selectedEngine === 'herdr-both' ? ['herdr', 'herdr-core'] : [this.selectedEngine];
        this.selectedEngineDefault = this.selectedEngine === 'herdr-both' ? 'herdr' : this.selectedEngine;
      }
      this.engineError = '';
      if (this.writeState === 'running') this.writeState = Wire.bool(row, 'deploying') ? 'running' : '';
    } catch (failure) {
      if (this.admitsRead(snapshot, epoch)) { this.error = errorCode(failure as Error); this.unsupported = this.error === 'unsupported'; this.refreshFailed = true; }
    }
  }

  private async write(snapshot: AdminSnapshot, path: string, method: string, body: Record<string, Object>, completedAsAck: boolean = false,
    owner?: ClientModel): Promise<Record<string, Object> | undefined> {
    const client = owner || this.root;
    if (!client || !this.admits(snapshot, path)) return undefined;
    if (client.operationBlocked(path)) { this.writeState = 'pending'; this.error = 'pending'; return undefined; }
    this.readEpoch++;
    const busyOwner = ++this.busySequence; this.savingOwner = busyOwner;
    this.saving = true; this.writeState = 'saving'; this.error = ''; this.refreshFailed = false;
    const clientGeneration = client.connectionGeneration(), clientScope = client.scope();
    try {
      const result = await client.repo().call(path, method, body, operationId(), (): boolean => this.admits(snapshot, path) &&
        client.owns(clientGeneration, clientScope), completedAsAck);
      this.readEpoch++;
      if (this.savingOwner === busyOwner || (this.writeState === 'result_unknown' && this.visible && snapshot.page === this.page &&
        snapshot.revision === this.revision && snapshot.selection === this.selectionKey())) {
        this.writeState = 'done'; this.error = '';
      }
      if (!this.admits(snapshot)) return result;
      return result;
    } catch (failure) {
      if (this.admits(snapshot)) {
        this.error = errorCode(failure as Error); this.writeState = ['pending', 'result_unknown'].includes(this.error) ? this.error : 'failed';
        this.unsupported = this.error === 'unsupported';
      }
      return undefined;
    } finally { if (this.savingOwner === busyOwner) this.saving = false; }
  }

  async refresh(): Promise<void> {
    if (!this.isCurrentRoot() || !this.visible) return;
    this.readEpoch++;
    const snapshot = this.capture(), busyOwner = ++this.busySequence; this.loadingOwner = busyOwner;
    this.loading = true; this.error = ''; this.unsupported = false; this.refreshFailed = false;
    try {
      if (this.page === 'assistant') await this.refreshAssistant(snapshot);
      else if (this.page === 'memory') await this.refreshMemory(snapshot);
      else if (this.page === 'managers') await this.refreshManagers(snapshot);
      else if (this.page === 'launchers') await this.refreshLaunchers(snapshot);
      else if (this.page === 'engine' || this.page === 'node_update') await this.readMachine(snapshot);
      else if (this.page === 'hub_update') await this.refreshHubUpdate(snapshot);
      else if (this.admits(snapshot)) this.error = 'unsupported';
    } finally { if (this.loadingOwner === busyOwner) this.loading = false; }
  }

  private async refreshAssistant(snapshot: AdminSnapshot): Promise<void> {
    const result = await this.read(snapshot, '/api/v1/assistant'); if (!result || !this.admits(snapshot)) return;
    this.assistantData = result;
    this.setRemoteForm(Wire.payload(['machine_id', 'launcher', 'timezone', 'notes', 'main_tasks_quiet', 'agent_tasks', 'inbox_signals',
      'interject_background', 'kb_topics', 'idle_close_hours', 'archive_closed_days', 'recording_days', 'minutes_days'], [
      Wire.text(result, 'machine_id'), Wire.text(result, 'launcher'), Wire.text(result, 'timezone'), Wire.text(result, 'notes'),
      Wire.bool(result, 'main_tasks_quiet', true), Wire.bool(result, 'agent_tasks'), Wire.bool(result, 'inbox_signals'),
      Wire.bool(result, 'interject_background'), Wire.bool(result, 'kb_topics', true), String(Wire.number(result, 'idle_close_hours', 12)),
      String(Wire.number(result, 'archive_closed_days', 3)), String(Wire.number(result, 'recording_days', 14)), String(Wire.number(result, 'minutes_days'))
    ]));
  }

  private async refreshMemory(snapshot: AdminSnapshot): Promise<void> {
    const result = await this.read(snapshot, '/api/v1/assistant/memory'); if (!result || !this.admits(snapshot)) return;
    const rows = Wire.rows(result, 'files').map((item: Record<string, Object>) => new AdminMemory(item));
    this.memoryRows = rows;
    if (!rows.some((row: AdminMemory) => row.name === this.selectedMemory)) {
      const previous = this.selectedMemory;
      this.selectedMemory = rows[0]?.name || '';
      if (this.selectedMemory !== previous) {
        this.revision++; this.memoryDraft = rows[0]?.content || ''; this.memoryDirty = false; this.memoryDraftRevision++;
      }
    }
    if (!this.memoryDirty) { this.memoryDraft = rows.find((row: AdminMemory) => row.name === this.selectedMemory)?.content || ''; this.memoryDraftRevision++; }
  }

  private async refreshManagers(snapshot: AdminSnapshot): Promise<void> {
    const result = await this.read(snapshot, '/api/v1/managers'); if (!result || !this.admits(snapshot)) return;
    this.managers = Wire.rows(result, 'managers').map((item: Record<string, Object>) => new AdminManager(item));
    this.defaultManager = Wire.text(result, 'default');
    if (!this.managers.some((row: AdminManager) => row.id === this.selectedManager)) {
      const previous = this.selectedManager;
      this.selectedManager = this.managers[0]?.id || '';
      if (this.selectedManager !== previous) {
        this.revision++;
        const manager = this.managers.find((row: AdminManager) => row.id === this.selectedManager);
        this.managerNameDraft = manager?.name || ''; this.draftDirty = false;
        this.draftJson = JSON.stringify(Wire.payload(['name'], [this.managerNameDraft])); this.draftRevision++;
      }
    }
    const manager = this.managers.find((row: AdminManager) => row.id === this.selectedManager);
    if (!this.draftDirty) this.managerNameDraft = manager?.name || '';
    await this.loadMoveMachines(this.capture(snapshot.routeTarget));
  }

  private async loadMoveMachines(snapshot: AdminSnapshot): Promise<void> {
    const model = this.root; if (!model || !this.admits(snapshot)) return;
    const rows: ResourceRow[] = [];
    for (const manager of this.managers) {
      if (!manager.installing && (manager.online || manager.id === model.profile.manager)) {
        const stub = new ResourceRow('', manager.name, 'machine'); stub.manager = manager.id;
        let owner: ClientModel;
        try { owner = await model.viewFor(stub); } catch (_) { continue; }
        if (!this.admits(snapshot) || !owner.connected) return;
        const generation = owner.connectionGeneration(), scope = owner.scope();
        try {
          const response = await owner.repo().call('/api/v1/machines', 'GET', {}, '', (): boolean => this.admits(snapshot) && owner.owns(generation, scope));
          if (!this.admits(snapshot) || !owner.owns(generation, scope)) return;
          for (const data of Wire.rows(response, 'machines')) {
            const row = ResourceRow.from(data, 'machine'); row.manager = Wire.text(data, 'manager', manager.id);
            if (!rows.some((current: ResourceRow) => current.id === row.id && current.manager === row.manager)) rows.push(row);
          }
        } catch (_) { /* An unavailable manager is shown in its roster, with no movable machine list. */ }
      }
    }
    if (this.admits(snapshot)) {
      this.managerMachineRows = rows; this.managerMachineManager = this.rootManager;
      if (!rows.some((row: ResourceRow) => row.id === this.selectedMoveMachine)) this.selectedMoveMachine = rows[0]?.id || '';
      const owner = rows.find((row: ResourceRow) => row.id === this.selectedMoveMachine)?.manager || '';
      if (!this.managers.some((row: AdminManager) => row.id === this.selectedMoveDestination && row.id !== owner && !row.installing))
        this.selectedMoveDestination = this.managers.find((row: AdminManager) => row.id !== owner && !row.installing)?.id || '';
    }
  }

  private async refreshLaunchers(snapshot: AdminSnapshot): Promise<void> {
    const result = await this.read(snapshot, '/api/v1/launchers'); if (!result || !this.admits(snapshot)) return;
    this.launchers = Wire.rows(result, 'launchers');
    let currentSnapshot = snapshot;
    if (!this.launcherNew && !this.launchers.some((item: Record<string, Object>) => Wire.text(item, 'name') === this.selectedLauncher)) {
      const previous = this.selectedLauncher;
      this.selectedLauncher = this.launchers[0] ? Wire.text(this.launchers[0], 'name') : '';
      if (this.selectedLauncher !== previous) {
        this.revision++; this.launcherDetail = {}; this.launcherEditor = false; this.launcherNew = false;
        this.launcherSecretDraft = ''; this.launcherSecretRevision++; this.draftDirty = false; this.draftRevision++;
        currentSnapshot = this.capture(snapshot.routeTarget);
      }
    }
    if (this.root?.features.includes('builtin_presets')) {
      const presets = await this.read(currentSnapshot, '/api/v1/launchers/presets'); if (presets && this.admits(currentSnapshot)) this.presets = Wire.rows(presets, 'presets');
    } else this.presets = [];
    if (this.selectedLauncher && !this.launcherEditor) await this.loadLauncherDetail(currentSnapshot, this.selectedLauncher);
    if (this.selectedMachine) {
      const machine = this.machineRows.find((row: ResourceRow) => row.id === this.selectedMachine);
      if (machine) {
        const suffix = '?machine=' + segment(this.selectedMachine);
        const health = await this.read(currentSnapshot, '/api/v1/launchers/health' + suffix);
        if (health && this.admits(currentSnapshot)) this.healthRecords = Wire.rows(health, 'records');
      }
    }
  }

  private async loadLauncherDetail(snapshot: AdminSnapshot, name: string): Promise<void> {
    const result = await this.read(snapshot, '/api/v1/launchers/' + segment(name)); if (!result || !this.admits(snapshot)) return;
    this.launcherDetail = Wire.object(result, 'launcher');
  }

  private async refreshHubUpdate(snapshot: AdminSnapshot): Promise<void> {
    const result = await this.read(snapshot, '/api/v1/hub/update'); if (!result || !this.admits(snapshot)) return;
    this.hubUpdate = result;
    const history = await this.read(snapshot, '/api/v1/hub/update/history');
    if (history && this.admits(snapshot)) this.updateHistory = Wire.rows(history, 'history');
    else if (this.admits(snapshot) && this.error === 'unsupported') {
      this.updateHistoryUnsupported = true; this.unsupported = false; this.refreshFailed = false; this.error = '';
    }
  }

  hubUpdateWorking(): boolean {
    return ['checking', 'downloading', 'waiting_idle', 'restarting', 'confirming'].includes(Wire.text(this.hubUpdate, 'state'));
  }

  hubBusyLabels(): string[] {
    return Wire.rows(this.hubUpdate, 'busy').map((item: Record<string, Object>) => {
      const machine = Wire.text(item, 'machine', Wire.text(item, 'machine_id'));
      const title = Wire.text(item, 'title');
      return title ? machine + ' · ' + title : machine;
    }).filter((value: string) => !!value);
  }

  async saveAssistant(snapshot: AdminSnapshot): Promise<boolean> {
    const form = this.form(), body: Record<string, Object> = {};
    const sentRevision = this.draftRevision;
    const editable = ['machine_id', 'launcher', 'timezone', 'notes', 'main_tasks_quiet', 'agent_tasks', 'inbox_signals', 'interject_background',
      'kb_topics', 'idle_close_hours', 'archive_closed_days', 'recording_days', 'minutes_days'];
    for (const key of editable) {
      if (!Object.keys(form).includes(key)) continue;
      let value: Object = form[key];
      if (['idle_close_hours', 'archive_closed_days', 'recording_days', 'minutes_days'].includes(key)) {
        const number = Number(value); if (!Number.isInteger(number)) { this.error = 'admin_invalid_number'; return false; }
        value = number;
      }
      const previous: Object | undefined = this.assistantData[key];
      if (JSON.stringify(value) !== JSON.stringify(previous)) body[key] = value;
    }
    if (!Object.keys(body).length) {
      if (this.admits(snapshot)) { this.draftDirty = false; await this.clearPersistedDraft(snapshot); }
      return true;
    }
    const result = await this.write(snapshot, '/api/v1/assistant', 'PUT', body);
    if (!result) return false;
    if (!this.admits(snapshot)) return true;
    this.assistantData = result;
    if (sentRevision !== this.draftRevision) return true;
    this.draftDirty = false; this.setRemoteForm(Wire.payload(['machine_id', 'launcher', 'timezone', 'notes', 'main_tasks_quiet', 'agent_tasks',
      'inbox_signals', 'interject_background', 'kb_topics', 'idle_close_hours', 'archive_closed_days', 'recording_days', 'minutes_days'], [
      Wire.text(result, 'machine_id'), Wire.text(result, 'launcher'), Wire.text(result, 'timezone'), Wire.text(result, 'notes'),
      Wire.bool(result, 'main_tasks_quiet', true), Wire.bool(result, 'agent_tasks'), Wire.bool(result, 'inbox_signals'), Wire.bool(result, 'interject_background'),
      Wire.bool(result, 'kb_topics', true), String(Wire.number(result, 'idle_close_hours', 12)), String(Wire.number(result, 'archive_closed_days', 3)),
      String(Wire.number(result, 'recording_days', 14)), String(Wire.number(result, 'minutes_days'))]));
    await this.clearPersistedDraft(snapshot);
    return true;
  }

  setMemoryDraft(value: string): void { this.memoryDraft = value; this.memoryDirty = true; this.memoryDraftRevision++; }

  setNewMemoryName(value: string): void { this.newMemoryName = value; this.newMemoryRevision++; }
  setNewMemoryContent(value: string): void { this.newMemoryContent = value; this.newMemoryRevision++; }

  private validNewMemoryName(value: string): boolean {
    return /^[A-Za-z0-9][A-Za-z0-9._-]{0,100}\.md$/.test(value);
  }

  canCreateMemory(): boolean {
    const name = this.newMemoryName.trim();
    return this.validNewMemoryName(name) && !this.memoryRows.some((row: AdminMemory) => row.name === name);
  }

  memoryCreateError(): string {
    const name = this.newMemoryName.trim();
    if (!this.validNewMemoryName(name)) return name ? 'admin_memory_invalid_name' : '';
    return this.memoryRows.some((row: AdminMemory) => row.name === name) ? 'admin_memory_exists' : '';
  }

  async createMemory(snapshot: AdminSnapshot): Promise<boolean> {
    const name = this.newMemoryName.trim(), content = this.newMemoryContent, sentRevision = this.newMemoryRevision;
    const path = '/api/v1/assistant/memory/' + segment(name);
    if (!this.admits(snapshot, path)) return false;
    if (!this.validNewMemoryName(name)) { this.error = 'admin_memory_invalid_name'; return false; }
    if (this.memoryRows.some((row: AdminMemory) => row.name === name)) { this.error = 'admin_memory_exists'; return false; }
    const result = await this.write(snapshot, path, 'PUT', Wire.payload(['content'], [content]));
    if (!result) return false;
    if (this.admits(snapshot)) {
      if (!this.memoryRows.some((row: AdminMemory) => row.name === name)) this.memoryRows = this.memoryRows.concat([new AdminMemory(Wire.payload(['name', 'content'], [name, content]))]);
      if (!this.selectedMemory) {
        this.selectedMemory = name; this.revision++;
        this.memoryDraft = content; this.memoryDirty = false; this.memoryDraftRevision++;
      }
      if (sentRevision === this.newMemoryRevision) { this.newMemoryName = ''; this.newMemoryContent = ''; this.newMemoryRevision++; this.notice = 'admin_memory_created'; }
    }
    try { await this.refreshMemory(this.capture()); } catch (_) { this.refreshFailed = true; this.error = 'admin_refresh_failed'; }
    return true;
  }

  async saveMemory(snapshot: AdminSnapshot): Promise<boolean> {
    if (!this.selectedMemory || !this.memoryDirty) return false;
    const target = this.selectedMemory, draft = this.memoryDraft, sentRevision = this.memoryDraftRevision;
    const path = '/api/v1/assistant/memory/' + segment(target);
    const result = await this.write(snapshot, path, 'PUT', Wire.payload(['content'], [draft]));
    if (!result) return false;
    if (!this.admits(snapshot)) return true;
    const file = this.memoryRows.find((item: AdminMemory) => item.name === target);
    if (file) { file.content = draft; }
    if (sentRevision === this.memoryDraftRevision) this.memoryDirty = false;
    try { await this.refreshMemory(snapshot); } catch (_) { this.refreshFailed = true; this.error = 'admin_refresh_failed'; }
    return true;
  }

  async deleteMemory(snapshot: AdminSnapshot, target: string): Promise<boolean> {
    if (!target || this.selectedMemory !== target) return false;
    const result = await this.write(snapshot, '/api/v1/assistant/memory/' + segment(target), 'DELETE', {});
    if (!result) return false;
    if (!this.admits(snapshot)) return true;
    this.memoryRows = this.memoryRows.filter((item: AdminMemory) => item.name !== target);
    this.selectedMemory = this.memoryRows[0]?.name || ''; this.memoryDraft = ''; this.memoryDirty = false;
    try { await this.refreshMemory(this.capture()); } catch (_) { this.refreshFailed = true; this.error = 'admin_refresh_failed'; }
    return true;
  }

  async createManager(snapshot: AdminSnapshot, name: string): Promise<boolean> {
    const body = Wire.payload(['name'], [name.trim()]);
    const result = await this.write(snapshot, '/api/v1/managers', 'POST', body);
    if (!result) return false;
    if (this.admits(snapshot)) { this.installCode = result; this.notice = 'manager_install_code'; }
    try { await this.refreshManagers(snapshot); } catch (_) { if (this.admits(snapshot)) { this.refreshFailed = true; this.error = 'admin_refresh_failed'; } }
    return true;
  }

  async renameManager(snapshot: AdminSnapshot, id: string, name: string): Promise<boolean> {
    if (!id || this.selectedManager !== id) return false;
    const sentDraftRevision = this.draftRevision;
    const result = await this.write(snapshot, '/api/v1/managers/' + segment(id), 'PATCH', Wire.payload(['name'], [name.trim()]));
    if (!result) return false;
    if (this.admits(snapshot) && this.draftRevision === sentDraftRevision) {
      this.managerNameDraft = Wire.text(Wire.object(result, 'manager'), 'name', name.trim());
      this.draftJson = JSON.stringify(Wire.payload(['name'], [this.managerNameDraft])); this.draftDirty = false; this.draftRevision++;
      await this.clearPersistedDraft(snapshot);
    }
    try { await this.refreshManagers(snapshot); } catch (_) { if (this.admits(snapshot)) { this.refreshFailed = true; this.error = 'admin_refresh_failed'; } }
    return true;
  }

  async makeDefaultManager(snapshot: AdminSnapshot, id: string): Promise<boolean> {
    const result = await this.write(snapshot, '/api/v1/managers/' + segment(id), 'PATCH', Wire.payload(['default'], [true]));
    if (!result) return false;
    try { await this.refreshManagers(snapshot); } catch (_) { if (this.admits(snapshot)) { this.refreshFailed = true; this.error = 'admin_refresh_failed'; } }
    return true;
  }

  async deleteManager(snapshot: AdminSnapshot, id: string): Promise<boolean> {
    const result = await this.write(snapshot, '/api/v1/managers/' + segment(id), 'DELETE', {});
    if (!result) return false;
    try { await this.refreshManagers(snapshot); } catch (_) { if (this.admits(snapshot)) { this.refreshFailed = true; this.error = 'admin_refresh_failed'; } }
    return true;
  }

  managerCanDelete(manager: AdminManager | undefined): boolean {
    return !!manager && !manager.builtin && !manager.isDefault &&
      !this.managerMachineRows.some((row: ResourceRow) => row.manager === manager.id);
  }

  managerInstallCommandKinds(): string[] {
    const commands = Wire.object(this.installCode, 'commands');
    return ['service', 'join', 'docker'].filter((name: string) => !!Wire.text(commands, name));
  }

  managerInstallCommand(name: string): string {
    return Wire.text(Wire.object(this.installCode, 'commands'), name);
  }

  async renewManagerCode(snapshot: AdminSnapshot, id: string): Promise<boolean> {
    const result = await this.write(snapshot, '/api/v1/managers/' + segment(id) + '/code', 'POST', {});
    if (!result) return false;
    if (this.admits(snapshot)) { this.installCode = result; this.notice = 'manager_install_code'; }
    try { await this.refreshManagers(snapshot); } catch (_) { if (this.admits(snapshot)) { this.refreshFailed = true; this.error = 'admin_refresh_failed'; } }
    return true;
  }

  moveChoices(): ResourceRow[] { return this.managerMachineRows; }

  async moveMachine(snapshot: AdminSnapshot, destination: string, machineID: string): Promise<boolean> {
    const row = this.managerMachineRows.find((item: ResourceRow) => item.id === machineID);
    if (!row || !destination || destination === row.manager || this.selectedMoveMachine !== machineID || this.selectedMoveDestination !== destination ||
      this.managerMachineManager !== this.rootManager || !this.managers.some((item: AdminManager) => item.id === destination && !item.installing)) return false;
    const path = '/api/v1/managers/' + segment(destination) + '/machines';
    const result = await this.write(snapshot, path, 'POST', Wire.payload(['machine_id'], [machineID]));
    if (!result) return false;
    try { await this.refreshManagers(snapshot); } catch (_) { if (this.admits(snapshot)) { this.refreshFailed = true; this.error = 'admin_refresh_failed'; } }
    return true;
  }

  beginLauncher(newProvider: boolean): void {
    this.revision++; this.launcherEditor = true; this.launcherNew = newProvider; this.launcherSecretDraft = ''; this.launcherSecretRevision++;
    if (newProvider) {
      this.selectedLauncher = '';
      this.draftJson = JSON.stringify(Wire.payload(['name', 'display_name', 'kind', 'api', 'base_url', 'model', 'auth'], ['', '', 'launcher', 'messages', '', '', 'api_key']));
    } else {
      const l = this.launcherDetail;
      this.draftJson = JSON.stringify(Wire.payload(['name', 'display_name', 'kind', 'api', 'base_url', 'model', 'auth'], [
        Wire.text(l, 'name'), Wire.text(l, 'display_name'), Wire.text(l, 'kind') === 'api' ? 'api' : 'launcher', Wire.text(l, 'api', 'messages'),
        Wire.text(l, 'base_url'), Wire.text(l, 'model'), Wire.text(l, 'auth', 'api_key')]));
    }
    this.draftDirty = true; this.draftRevision++; this.persistDraft();
  }

  cancelLauncherEdit(): void {
    this.launcherEditor = false; this.launcherNew = false; this.launcherSecretDraft = ''; this.launcherSecretRevision++; this.draftDirty = false; this.draftRevision++;
    this.clearPersistedDraft(this.capture()).catch(() => undefined);
  }

  setLauncherSecretDraft(value: string): void { this.launcherSecretDraft = value; this.launcherSecretRevision++; }

  async saveLauncher(snapshot: AdminSnapshot): Promise<boolean> {
    const form = this.form(), name = Wire.text(form, 'name').trim(), kind = Wire.text(form, 'kind', 'launcher');
    const sentDraftRevision = this.draftRevision, sentSecretRevision = this.launcherSecretRevision, wasNew = this.launcherNew;
    const existing: Record<string, Object> = this.launcherNew ? {} : this.launcherDetail;
    const api = Wire.text(form, 'api', Wire.text(existing, 'api', 'messages'));
    const body: Record<string, Object> = Wire.payload(['name', 'display_name', 'kind', 'api', 'base_url', 'model', 'auth', 'agent',
      'small_model', 'subagent_model', 'custom_model', 'custom_model_name', 'custom_model_description', 'args'], [name,
      Wire.text(form, 'display_name').trim(), kind, api, Wire.text(form, 'base_url').trim(), Wire.text(form, 'model').trim(),
      Wire.text(form, 'auth', 'api_key'), kind === 'api' ? '' : Wire.text(existing, 'agent', 'claude'),
      kind === 'api' ? '' : Wire.text(existing, 'small_model'), kind === 'api' ? '' : Wire.text(existing, 'subagent_model'),
      kind === 'api' ? '' : Wire.text(existing, 'custom_model'), kind === 'api' ? '' : Wire.text(existing, 'custom_model_name'),
      kind === 'api' ? '' : Wire.text(existing, 'custom_model_description'), kind === 'api' ? [] : Wire.strings(existing, 'args')]);
    if (!this.launcherNew) {
      for (const key of ['models', 'opus_model', 'sonnet_model', 'haiku_model', 'fable_model', 'subagent_force',
        'proxy', 'no_proxy', 'timeout_ms', 'compact_window', 'max_context', 'effort']) {
        if (Object.keys(existing).includes(key)) body[key] = existing[key];
      }
    }
    if (this.launcherSecretDraft) body['api_key'] = this.launcherSecretDraft;
    const path = this.launcherNew ? '/api/v1/launchers' : '/api/v1/launchers/' + segment(this.selectedLauncher);
    const result = await this.write(snapshot, path, this.launcherNew ? 'POST' : 'PUT', body);
    if (!result) return false;
    const ownerStillCurrent = this.admits(snapshot);
    if (ownerStillCurrent && sentDraftRevision === this.draftRevision && sentSecretRevision === this.launcherSecretRevision) {
      const savedName = Wire.text(Wire.object(result, 'launcher'), 'name', name);
      this.launcherSecretDraft = ''; this.launcherSecretRevision++; this.launcherEditor = false; this.launcherNew = false; this.draftDirty = false; this.draftRevision++;
      const cleanup = this.clearPersistedDraft(snapshot), cleanupRevision = this.draftRevision;
      await cleanup;
      if (!this.admits(snapshot) || this.draftRevision !== cleanupRevision) return true;
      if (this.selectedLauncher !== savedName) { this.selectedLauncher = savedName; this.revision++; }
    }
    if (ownerStillCurrent && wasNew && this.launcherEditor && this.formText('name').trim() === name) {
      // A successful creation establishes identity even when the user has edited the remaining draft.
      this.launcherNew = false; this.selectedLauncher = name; this.revision++;
      const saved: Record<string, Object> = {};
      for (const key of Object.keys(body)) if (key !== 'api_key') saved[key] = body[key];
      const acknowledged = Wire.object(result, 'launcher');
      for (const key of Object.keys(acknowledged)) if (key !== 'api_key') saved[key] = acknowledged[key];
      this.launcherDetail = saved; this.persistDraft();
    }
    if (ownerStillCurrent) {
      try { await this.refreshLaunchers(this.capture()); } catch (_) { if (this.visible) { this.refreshFailed = true; this.error = 'admin_refresh_failed'; } }
    }
    return true;
  }

  async deleteLauncher(snapshot: AdminSnapshot, name: string): Promise<boolean> {
    const result = await this.write(snapshot, '/api/v1/launchers/' + segment(name), 'DELETE', {});
    if (!result) return false;
    if (this.admits(snapshot)) { this.selectedLauncher = ''; this.launcherDetail = {}; this.launcherEditor = false; }
    try { await this.refreshLaunchers(this.capture()); } catch (_) { if (this.visible) { this.refreshFailed = true; this.error = 'admin_refresh_failed'; } }
    return true;
  }

  async addPreset(snapshot: AdminSnapshot, id: string): Promise<boolean> {
    const result = await this.write(snapshot, '/api/v1/launchers', 'POST', Wire.payload(['preset'], [id]));
    if (!result) return false;
    if (this.admits(snapshot)) this.notice = 'launcher_saved';
    try { await this.refreshLaunchers(this.capture()); } catch (_) { if (this.visible) { this.refreshFailed = true; this.error = 'admin_refresh_failed'; } }
    return true;
  }

  async loadImporter(snapshot: AdminSnapshot, machine: string): Promise<void> {
    if (!machine) return;
    const result = await this.read(snapshot, '/api/v1/launchers/import?machine=' + segment(machine));
    if (!result || !this.admits(snapshot)) return;
    this.importer = result; this.importerMachine = machine;
    this.importerSelection = Wire.rows(result, 'providers').map((item: Record<string, Object>) => Wire.text(item, 'name'));
    this.revision++;
  }

  selectImport(name: string, selected: boolean): void {
    const before = this.importerSelection.slice().sort().join('\u0000');
    this.importerSelection = selected ? this.importerSelection.concat([name]).filter((v: string, i: number, all: string[]) => all.indexOf(v) === i) :
      this.importerSelection.filter((item: string) => item !== name);
    if (before !== this.importerSelection.slice().sort().join('\u0000')) this.revision++;
  }

  async importLaunchers(snapshot: AdminSnapshot, machine: string, providers: string[]): Promise<boolean> {
    if (!machine || machine !== this.selectedMachine || machine !== this.importerMachine || !providers.length ||
      providers.slice().sort().join('\u0000') !== this.importerSelection.slice().sort().join('\u0000')) return false;
    const body = Wire.payload(['machine', 'providers'], [machine, providers.slice()]);
    const result = await this.write(snapshot, '/api/v1/launchers/import', 'POST', body);
    if (!result) return false;
    if (this.admits(snapshot)) { this.notice = 'launcher_imported'; this.importer = {}; this.importerSelection = []; this.importerMachine = ''; }
    try { await this.refreshLaunchers(this.capture()); } catch (_) { if (this.visible) { this.refreshFailed = true; this.error = 'admin_refresh_failed'; } }
    return true;
  }

  async checkLauncher(snapshot: AdminSnapshot, name: string): Promise<boolean> {
    const path = '/api/v1/launchers/' + segment(name) + '/check' + (this.selectedMachine ? '?machine=' + segment(this.selectedMachine) : '');
    const result = await this.write(snapshot, path, 'POST', {}, true);
    if (!result) return false;
    if (this.admits(snapshot)) this.healthResult = Wire.object(result, 'check');
    return true;
  }

  async checkLauncherHealth(snapshot: AdminSnapshot): Promise<boolean> {
    const path = '/api/v1/launchers/health/check' + (this.selectedMachine ? '?machine=' + segment(this.selectedMachine) : '');
    const result = await this.write(snapshot, path, 'POST', {}, true);
    if (!result) return false;
    if (this.admits(snapshot)) this.healthRecords = Wire.rows(result, 'records');
    return true;
  }

  async saveEngine(snapshot: AdminSnapshot, engine: string): Promise<boolean> {
    const machineID = this.selectedMachine, owner = await this.machineClient(snapshot);
    if (!owner || !this.admits(snapshot) || !machineID) return false;
    const path = '/api/v1/machines/' + segment(machineID) + '/engine';
    const result = await this.write(snapshot, path, 'PUT', Wire.payload(['engine'], [engine]), false, owner);
    if (!result) return false;
    if (this.admits(snapshot)) this.selectedEngine = Wire.text(result, 'engine', engine);
    return true;
  }

  engineSetsSupported(): boolean { return Wire.has(this.machineData, 'engine_hosts'); }

  engineOptions(): string[] {
    const reported = Wire.rows(this.machineData, 'engines').map((item: Record<string, Object>) => Wire.text(item, 'id')).filter((id: string) => !!id);
    const options = reported.length ? reported : ['herdr', 'herdr-core', 'herdr-both', 'orca', 'tern', 'rex', 'paseo', 'cindy'];
    return this.engineSetsSupported() ? options.filter((id: string) => id !== 'herdr-both') : options;
  }

  engineName(id: string): string {
    const found = Wire.rows(this.machineData, 'engines').find((item: Record<string, Object>) => Wire.text(item, 'id') === id);
    if (found) return Wire.text(found, 'name', id);
    const names: Record<string, string> = { 'herdr': 'herdr', 'herdr-core': 'Herdr App', 'herdr-both': 'herdr + Herdr App',
      'orca': 'Orca', 'tern': 'Tern', 'rex': 'Rex', 'paseo': 'Paseo', 'cindy': 'Cindy', 'none': 'No terminal engine' };
    return names[id] || id;
  }

  selectEngineHost(id: string, checked: boolean): void {
    if (id === 'none') {
      if (checked && this.engineSetsSupported()) { this.selectedEngineHosts = []; this.selectedEngineDefault = ''; this.revision++; }
      return;
    }
    const next = this.selectedEngineHosts.filter((item: string) => item !== id);
    if (checked) next.push(id);
    this.selectedEngineHosts = this.engineOptions().filter((item: string) => next.includes(item));
    if (!this.selectedEngineHosts.includes(this.selectedEngineDefault)) this.selectedEngineDefault = this.selectedEngineHosts[0] || '';
    this.revision++;
  }

  selectEngineDefault(id: string): void {
    if (!this.selectedEngineHosts.includes(id) || id === this.selectedEngineDefault) return;
    this.selectedEngineDefault = id; this.revision++;
  }

  chooseLegacyEngine(id: string): void {
    if (this.engineSetsSupported() || !this.engineOptions().includes(id)) return;
    this.selectedEngineHosts = [id]; this.selectedEngineDefault = id; this.revision++;
  }

  async saveEngineSelection(snapshot: AdminSnapshot): Promise<boolean> {
    const machineID = this.selectedMachine, owner = await this.machineClient(snapshot);
    if (!owner || !this.admits(snapshot) || !machineID) return false;
    let body: Record<string, Object>;
    if (this.engineSetsSupported()) {
      const hosts = this.selectedEngineHosts.slice();
      const defaultHost = hosts.includes(this.selectedEngineDefault) ? this.selectedEngineDefault : '';
      body = Wire.payload(['hosts', 'default'], [hosts, defaultHost]);
    } else {
      const hosts = this.selectedEngineHosts;
      const selected = hosts.length === 2 && hosts.includes('herdr') && hosts.includes('herdr-core') ? 'herdr-both' : hosts[0] || 'herdr';
      body = Wire.payload(['engine'], [selected]);
    }
    const path = '/api/v1/machines/' + segment(machineID) + '/engine';
    const result = await this.write(snapshot, path, 'PUT', body, false, owner);
    if (!result) return false;
    if (this.admits(snapshot)) {
      this.selectedEngine = Wire.text(result, 'engine', this.selectedEngine);
      if (Wire.has(result, 'engine_hosts')) {
        this.selectedEngineHosts = Wire.strings(result, 'engine_hosts'); this.selectedEngineDefault = Wire.text(result, 'engine_default');
      }
    }
    return true;
  }

  async deployNode(snapshot: AdminSnapshot): Promise<boolean> {
    const machineID = this.selectedMachine, machine = this.machineRows.find((row: ResourceRow) => row.id === machineID);
    if (!machine || !Wire.bool(this.machineData, 'can_deploy') || !machineID) return false;
    const owner = await this.machineClient(snapshot); if (!owner || !this.admits(snapshot)) return false;
    const path = '/api/v1/machines/' + segment(machineID) + '/deploy';
    const result = await this.write(snapshot, path, 'POST', {}, false, owner);
    if (!result) return false;
    // The route only starts an asynchronous job. A true `ok` is not a completed node update.
    if (this.admits(snapshot)) this.writeState = 'running';
    return true;
  }

  async checkHubUpdate(snapshot: AdminSnapshot): Promise<boolean> {
    const result = await this.write(snapshot, '/api/v1/hub/update/check', 'POST', {}, true);
    if (!result) return false;
    if (this.admits(snapshot)) this.hubUpdate = result;
    return true;
  }

  async applyHubUpdate(snapshot: AdminSnapshot, now: boolean): Promise<boolean> {
    const result = await this.write(snapshot, '/api/v1/hub/update', 'POST', Wire.payload(['now'], [now]), true);
    if (!result) return false;
    if (this.admits(snapshot)) this.hubUpdate = result;
    return true;
  }

  async setHubUpdateSetting(snapshot: AdminSnapshot, key: 'auto' | 'auto_update_nodes', enabled: boolean): Promise<boolean> {
    const result = await this.write(snapshot, '/api/v1/hub/update/settings', 'PUT', Wire.payload([key], [enabled]));
    if (!result) return false;
    if (this.admits(snapshot)) this.hubUpdate = result;
    return true;
  }

  private clearPersistedDraft(snapshot: AdminSnapshot): Promise<void> {
    const model = this.root; if (!model || !this.admits(snapshot)) return Promise.resolve();
    const page = this.page, version = ++this.draftRevision;
    this.draftQueue = this.draftQueue.catch(() => {}).then(async (): Promise<void> => {
      if (this.root !== model || !this.admits(snapshot) || this.page !== page || this.draftRevision !== version) return;
      const epoch = model.viewRevision(VIEW_PREFIX + page);
      await model.saveView(VIEW_PREFIX + page, JSON.stringify(Wire.payload(['dirty'], [false])), epoch);
    }).catch(() => { if (this.root === model && this.admits(snapshot) && this.draftRevision === version) this.error = 'admin_storage_failed'; });
    return this.draftQueue;
  }
}
