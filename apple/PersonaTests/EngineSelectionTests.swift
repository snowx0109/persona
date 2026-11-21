import Foundation
import Testing
@testable import Persona
import PersonaWire

@MainActor struct EngineSelectionTests {
    private func machine(_ extra: String = "") throws -> Machine {
        try JSON.decoder.decode(Machine.self, from: Data("{\"id\":\"computer\",\"name\":\"电脑\",\"kind\":\"local\",\"online\":false\(extra)}".utf8))
    }

    @Test func compatibleDecoding() throws {
        let old = try machine()
        #expect(old.engine == .herdr && old.engines.isEmpty)
        let unknown = try machine(",\"engine\":\"future\",\"engines\":[{\"id\":\"herdr\",\"name\":\"herdr\",\"available\":true}]")
        #expect(unknown.engine.rawValue == "future" && unknown.engines.count == 1)
        #expect(try JSON.decoder.decode(Machine.self, from: JSON.encoder.encode(unknown)) == unknown)
        let current = try machine(",\"engine\":\"herdr-core\",\"engines\":[{\"id\":\"herdr-core\",\"name\":\"Herdr App\",\"available\":true,\"version\":\"1.2\"}]")
        #expect(current.engine == .herdrApp)
        #expect(current.engines.first?.version == "1.2")
        #expect(try JSON.decoder.decode(Machine.self, from: JSON.encoder.encode(current)) == current)
    }

    @Test func hubOptionsKeepNamesNotesAndFutureEngines() throws {
        var m = try machine(",\"engine\":\"orca\",\"engines\":[{\"id\":\"orca\",\"name\":\"Orca Desktop\",\"available\":false,\"note\":\"请先更新 Orca\"},{\"id\":\"future\",\"name\":\"新引擎\",\"available\":true}]")
        #expect(m.engine == .orca && m.engineName == "Orca Desktop")
        #expect(EngineSelectionModel.options(m) == m.engines)
        #expect(m.engines.first?.note == "请先更新 Orca")
        let future = MachineEngine(rawValue: "future")
        #expect(EngineSelectionModel.canSelect(.orca, on: m))
        #expect(EngineSelectionModel.canSelect(future, on: m))
        #expect(EngineSelectionModel.initialChoice(m, pairing: true) == .orca)
        m.engines[1].available = false
        #expect(!EngineSelectionModel.canSelect(future, on: m))
        m.engines = []
        #expect(!EngineSelectionModel.canSelect(future, on: m))
        #expect(EngineSelectionModel.options(m).map(\.id) == ["herdr", "herdr-core", "herdr-both", "orca", "tern", "rex", "paseo", "cindy"])
    }

    @Test func combinedEngineIsAKnownChoice() throws {
        let both = MachineEngine.herdrBoth
        #expect(both.rawValue == "herdr-both" && both.name == "herdr + Herdr App")
        #expect(both.detail == "终端和桌面 App 的会话都显示")
        #expect(EngineSelectionModel.canSelect(both, on: try machine()))
        var m = try machine(",\"engine\":\"herdr-both\"")
        #expect(m.engine == .herdrBoth)
        m.engines = [.init(id: "herdr-both", name: "herdr + Herdr App", available: true)]
        #expect(EngineSelectionModel.options(m).contains { $0.id == "herdr-both" })
    }

    @Test func paseoIsAKnownChoice() throws {
        let paseo = MachineEngine.paseo
        #expect(paseo.rawValue == "paseo" && paseo.name == "Paseo")
        #expect(paseo.detail == "Paseo 里的 Agent")
        #expect(EngineSelectionModel.canSelect(paseo, on: try machine()), "selectable offline, as the other engines")
        let m = try machine(",\"engine\":\"paseo\",\"engines\":[{\"id\":\"paseo\",\"name\":\"Paseo\",\"available\":true,\"version\":\"0.14.0\"}]")
        #expect(m.engine == .paseo && m.engines.first?.version == "0.14.0")
    }

    /// Tern (#2154): a terminal engine between Orca and Paseo, chosen like the others, also offline.
    @Test func ternIsAKnownChoice() throws {
        let tern = MachineEngine.tern
        #expect(tern.rawValue == "tern" && tern.name == "Tern")
        #expect(tern.detail == "Tern 里的终端")
        #expect(EngineSelectionModel.canSelect(tern, on: try machine()), "selectable offline, as the other engines")
        #expect(MachineEngine.allCases.firstIndex(of: .tern) == MachineEngine.allCases.firstIndex(of: .orca).map { $0 + 1 })
        let m = try machine(",\"engine\":\"tern\",\"engines\":[{\"id\":\"herdr\",\"name\":\"herdr\",\"available\":false},{\"id\":\"tern\",\"name\":\"Tern\",\"available\":true,\"version\":\"0.4.1\"}]")
        #expect(m.engine == .tern && m.engineName == "Tern")
        #expect(EngineSelectionModel.initialChoice(m, pairing: true) == .tern)
        #expect(EngineSelectionModel.options(m).map(\.id) == ["herdr", "tern"], "the node's list as it reports it")
        #expect(try JSON.decoder.decode(Machine.self, from: JSON.encoder.encode(m)) == m)
    }

    /// Rex (#12334): a terminal engine after Tern and before Paseo, chosen like the others, also offline.
    @Test func rexIsAKnownChoice() throws {
        let rex = MachineEngine.rex
        #expect(rex.rawValue == "rex" && rex.name == "Rex")
        #expect(rex.detail == "Rex 里的终端")
        #expect(EngineSelectionModel.canSelect(rex, on: try machine()), "selectable offline, as the other engines")
        #expect(MachineEngine.allCases.firstIndex(of: .rex) == MachineEngine.allCases.firstIndex(of: .tern).map { $0 + 1 })
        let m = try machine(",\"engine\":\"rex\",\"engines\":[{\"id\":\"herdr\",\"name\":\"herdr\",\"available\":false},{\"id\":\"rex\",\"name\":\"Rex\",\"available\":true,\"version\":\"0.1.0-preview.21\"}]")
        #expect(m.engine == .rex && m.engineName == "Rex")
        #expect(EngineSelectionModel.initialChoice(m, pairing: true) == .rex)
        #expect(EngineSelectionModel.options(m).map(\.id) == ["herdr", "rex"], "the node's list as it reports it")
        #expect(try JSON.decoder.decode(Machine.self, from: JSON.encoder.encode(m)) == m)
    }

    @Test func cindyIsAKnownChoice() throws {
        let cindy = MachineEngine.cindy
        #expect(cindy.rawValue == "cindy" && cindy.name == "Cindy")
        #expect(cindy.detail == "Cindy 里的 Agent")
        #expect(EngineSelectionModel.canSelect(cindy, on: try machine()), "selectable offline, as the other engines")
        let m = try machine(",\"engine\":\"cindy\",\"engines\":[{\"id\":\"cindy\",\"name\":\"Cindy\",\"available\":true,\"version\":\"0.1.128\"}]")
        #expect(m.engine == .cindy && m.engines.first?.version == "0.1.128")
    }

    @Test func noTerminalEngineIsTheEmptyHostSet() throws {
        // A hub before host sets reports no engine_hosts: nothing new is offered.
        let old = try machine()
        #expect(old.engineHosts == nil)
        #expect(!EngineSelectionModel.options(old).contains { $0.id == MachineEngine.noEngine.rawValue })
        #expect(!EngineSelectionModel.canSelect(.noEngine, on: old))
        #expect(!MachineEngine.allCases.contains(.noEngine), "never sent as an engine id")
        // A hub with host sets: offered last; herdr stays the choice of a computer that never picked.
        var sets = try machine(",\"engine\":\"herdr\",\"engine_hosts\":[\"herdr\"]")
        #expect(sets.engineHosts == ["herdr"])
        #expect(EngineSelectionModel.options(sets).last?.id == MachineEngine.noEngine.rawValue)
        #expect(EngineSelectionModel.canSelect(.noEngine, on: sets))
        #expect(EngineSelectionModel.initialChoice(sets, pairing: true) == .herdr)
        sets.engines = [.init(id: "herdr", name: "herdr", available: true)]
        #expect(EngineSelectionModel.options(sets).map(\.id) == ["herdr", "none"])
        // The empty set reads as no terminal engine, though `engine` still says herdr for older clients.
        let none = try machine(",\"engine\":\"herdr\",\"engine_hosts\":[]")
        #expect(none.engineHosts == [] && none.engine == .herdr)
        #expect(EngineSelectionModel.initialChoice(none, pairing: false) == .noEngine)
        #expect(none.engineName == "不用终端引擎")
        #expect(MachineEngine.noEngine.detail == "只显示桌面 App 的会话")
        #expect(try JSON.decoder.decode(Machine.self, from: JSON.encoder.encode(none)) == none)
    }

    @Test func noTerminalEngineSavesTheEmptyHostSet() async throws {
        let hub = FakeHub()
        let current = try machine(",\"engine\":\"herdr\",\"engine_hosts\":[\"herdr\"]")
        let saved = try machine(",\"engine\":\"herdr\",\"engine_hosts\":[]")
        let response = try JSON.encoder.encode(saved)
        hub.on("PUT", "/api/v1/machines/computer/engine") { req in
            let raw = try #require(req.body)
            let body = try #require(try JSONSerialization.jsonObject(with: raw) as? [String: Any])
            #expect(body.keys.sorted() == ["hosts"])
            #expect((body["hosts"] as? [Any])?.isEmpty == true)
            return response
        }
        var updated: Machine?
        let model = EngineSelectionModel(machine: current, pairing: true, deps: .init(api: { hub.client }, updated: { updated = $0 }))
        #expect(model.selected == .herdr)
        model.choose(.noEngine)
        #expect(await model.save(machineID: "computer"))
        // The hub answers engine herdr with an empty set: the picker stays on no terminal engine.
        #expect(updated == saved && model.selected == .noEngine && model.error == nil && !model.saving)
        model.synchronize(saved, pairing: true)
        #expect(model.selected == .noEngine)
    }

    @Test func pairingTargetAndPreselection() throws {
        var local = try machine()
        var remote = local
        remote.id = "remote"
        remote.kind = "ssh"
        #expect(EngineSelectionModel.pairingMachine([remote, local])?.id == local.id)
        #expect(EngineSelectionModel.pairingMachine([remote])?.id == remote.id)
        #expect(EngineSelectionModel.pairingMachine([]) == nil)
        // Always the machine's current engine (herdr when never chosen), whatever happens to be detected.
        #expect(EngineSelectionModel.initialChoice(local, pairing: true) == .herdr)
        local.engines = [.init(id: "herdr-core", name: "Herdr App", available: true)]
        #expect(EngineSelectionModel.initialChoice(local, pairing: true) == .herdr)
        #expect(EngineSelectionModel.initialChoice(local, pairing: false) == .herdr)
        local.engine = .herdrApp
        #expect(EngineSelectionModel.initialChoice(local, pairing: true) == .herdrApp)
        local.engines = []
        #expect(EngineSelectionModel.initialChoice(local, pairing: true) == .herdrApp)
    }

    @Test func savesOfflineAndDoesNotLoseAnEditedChoiceToPushes() async throws {
        let hub = FakeHub()
        var offline = try machine()
        offline.engine = .herdrApp
        let response = try JSON.encoder.encode(offline)
        hub.on("PUT", "/api/v1/machines/computer/engine") { req in
            let body = try JSON.decoder.decode([String: String].self, from: #require(req.body))
            #expect(body == ["engine": "herdr-core"])
            return response
        }
        var updated: Machine?
        let model = EngineSelectionModel(machine: try machine(), pairing: false, deps: .init(api: { hub.client }, updated: { updated = $0 }))
        model.choose(.herdrApp)
        model.synchronize(try machine(), pairing: false)
        #expect(model.selected == .herdrApp)
        #expect(await model.save(machineID: "computer"))
        #expect(updated == offline && model.error == nil && !model.saving)
        model.synchronize(try machine(), pairing: false)
        #expect(model.selected == .herdr)
    }

    @Test func failureKeepsChoiceAndAllowsRetry() async throws {
        let hub = FakeHub()
        hub.on("PUT", "/api/v1/machines/computer/engine") { _ in throw APIError(status: 503, message: "暂时无法保存") }
        var updates = 0
        let model = EngineSelectionModel(machine: try machine(), pairing: true, deps: .init(api: { hub.client }, updated: { _ in updates += 1 }))
        model.choose(.herdrApp)
        #expect(!(await model.save(machineID: "computer")))
        #expect(model.selected == .herdrApp && model.error?.contains("暂时无法保存") == true && !model.saving && updates == 0)
        var saved = try machine()
        saved.engine = .herdrApp
        hub.on("PUT", "/api/v1/machines/computer/engine", json: String(decoding: try JSON.encoder.encode(saved), as: UTF8.self))
        #expect(await model.save(machineID: "computer"))
        #expect(updates == 1 && model.error == nil)
    }

    @Test func demoSupportsEngineChangesAndValidation() async throws {
        DemoHub.install()
        DemoHub.shared.reset()
        let api = APIClient(config: .demo)
        let original = try #require(try await api.machines().first { !$0.online })
        let saved = try await api.setEngine(machine: original.id, engine: .herdrApp)
        #expect(saved.engine == .herdrApp && !saved.online && saved.engines.isEmpty)
        #expect(try await api.machines().first { $0.id == original.id }?.engine == .herdrApp)
        let orca = try await api.setEngine(machine: original.id, engine: .orca)
        #expect(orca.engine == .orca)
        do {
            _ = try await api.request("PUT", "/api/v1/machines/\(original.id)/engine", body: ["engine": "unknown"])
            Issue.record("unknown engine accepted")
        } catch let error as APIError { #expect(error.status == 400) }
        do {
            _ = try await api.setEngine(machine: "missing", engine: .herdr)
            Issue.record("unknown machine accepted")
        } catch let error as APIError { #expect(error.status == 404) }
    }
}
