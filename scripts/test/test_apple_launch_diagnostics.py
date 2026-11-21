"""Launch diagnostics must fail closed without exposing another simulator or raw output."""
from datetime import datetime, timedelta, timezone
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
from unittest import mock

ROOT = Path(__file__).resolve().parents[2]
sys.path.insert(0, str(ROOT / "scripts/ci"))
import apple_launch_diagnostics as launch
import apple_diagnostics

UDID = "11111111-1111-1111-1111-111111111111"
OTHER = "22222222-2222-2222-2222-222222222222"
START = datetime(2026, 1, 1, 12, tzinfo=timezone.utc)


class LaunchDiagnosticsTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.output = Path(self.tmp.name) / "output"
        self.destination = Path(self.tmp.name) / "artifacts"
        self.output.mkdir()
        self.destination.mkdir()
        self.context = {"udid": UDID, "bundle_id": launch.BUNDLE, "started_at": START.isoformat()}
        self.save()
        (self.output / "simulator.log").write_text(UDID + "\n")
        self.calls = []

    def save(self):
        (self.output / launch.CONTEXT).write_text(json.dumps(self.context))

    def entry(self, **changes):
        entry = {"timestamp": "2026-01-01 12:01:00.000000+0000",
                 "processImagePath": "/System/Library/CoreSimulatorService",
                 "eventMessage": f"Unable to launch {launch.BUNDLE} on {UDID}: service unavailable",
                 "privateConfiguration": "DO NOT EXPORT", "extraCredential": "DO NOT EXPORT"}
        return dict(entry, **changes)

    def collect(self, entries=None, reader=None):
        def fake(command):
            self.calls.append(command)
            return ("\n".join(json.dumps(self.entry() if entries is None else entry)
                              for entry in ([None] if entries is None else entries)) + "\n").encode()
        launch.collect(self.output, self.destination, reader=reader or fake,
                       now=START + timedelta(minutes=17))

    def status(self):
        return (self.destination / "simulator-launch.status.txt").read_text()

    def test_exact_owned_identity_and_original_readiness_window(self):
        self.collect()
        command, = self.calls
        self.assertEqual(command[:7], ["/usr/bin/log", "show", "--style", "ndjson", "--info", "--debug", "--start"])
        self.assertEqual(command[7], "2026-01-01 12:00:00+00:00")
        self.assertEqual(command[command.index("--end") + 1], "2026-01-01 12:15:00+00:00")
        predicate = command[command.index("--predicate") + 1]
        self.assertIn(f'(eventMessage CONTAINS[c] "{UDID}" OR ', predicate)
        self.assertIn(f'eventMessage CONTAINS "{launch.BUNDLE}" OR ', predicate)
        self.assertNotIn(OTHER, predicate)
        self.assertNotIn("--last", command)
        exported = json.loads((self.destination / "simulator-launch.json").read_text())
        self.assertEqual(len(exported), 1)
        self.assertEqual(set(exported[0]), {"timestamp", "process", "eventMessage"})
        self.assertNotIn("DO NOT EXPORT", json.dumps(exported))
        self.assertIn("captured 1 scoped events", self.status())

    def test_missing_invalid_or_mismatched_context_never_runs_a_command(self):
        cases = [None, {**self.context, "udid": OTHER}, {**self.context, "udid": '" OR TRUE'},
                 {**self.context, "bundle_id": "org.example.someone.else"},
                 {**self.context, "started_at": "invalid"},
                 {**self.context, "started_at": "2026-01-01T12:00:00"},
                 {**self.context, "started_at": "2026-01-01T13:00:00+00:00"},
                 {**self.context, "started_at": "2025-12-31T12:00:00+00:00"},
                 {**self.context, "credentials": "DO NOT EXPORT"}, {}]
        for context in cases:
            with self.subTest(context=context):
                path = self.output / launch.CONTEXT
                if context is None:
                    path.unlink()
                else:
                    path.write_text(json.dumps(context))
                self.collect()
                self.assertEqual(self.calls, [])
                self.assertEqual(self.status(),
                                 "unavailable: scope-validation-failure; no launch cause established\n")
                self.assertFalse((self.destination / "simulator-launch.json").exists())
                self.assertFalse((self.destination / launch.CONTEXT).exists())

    def test_context_symlink_is_not_read_or_copied(self):
        path = self.output / launch.CONTEXT
        path.unlink()
        private = self.output / "private-config.json"
        private.write_text(json.dumps(self.context))
        path.symlink_to(private)
        self.collect()
        self.assertFalse(self.calls)
        self.assertEqual(self.status(),
                         "unavailable: scope-validation-failure; no launch cause established\n")
        self.assertEqual({p.name for p in self.destination.iterdir()}, {"simulator-launch.status.txt"})

    def test_both_scope_files_reject_links_large_files_and_fifos(self):
        for name in (launch.CONTEXT, "simulator.log"):
            for kind in ("symlink", "large", "fifo", "directory"):
                with self.subTest(name=name, kind=kind):
                    path = self.output / name
                    original = path.read_bytes()
                    path.unlink()
                    if kind == "symlink":
                        target = self.output / "private-input"
                        target.write_bytes(original)
                        path.symlink_to(target)
                    elif kind == "large":
                        path.write_bytes(b"x" * 1025)
                    elif kind == "fifo":
                        os.mkfifo(path)
                    else:
                        path.mkdir()
                    self.collect()
                    self.assertFalse(self.calls)
                    self.assertEqual(self.status(),
                                     "unavailable: scope-validation-failure; no launch cause established\n")
                    self.assertFalse((self.destination / "simulator-launch.json").exists())
                    if kind == "directory":
                        path.rmdir()
                    else:
                        path.unlink()
                    path.write_bytes(original)

    def test_output_is_rechecked_and_empty_is_not_evidence(self):
        self.collect(entries=[
            self.entry(eventMessage=f"launch unrelated.example on {OTHER}"),
            self.entry(eventMessage=f"launch {launch.BUNDLE}.other on {OTHER}"),
            self.entry(eventMessage=f"launch unrelated.example on {OTHER}"),
            self.entry(processImagePath="/usr/libexec/launchd_sim",
                       eventMessage=f"Launch {launch.BUNDLE}.other failed"),
            self.entry(processImagePath="/Applications/UnrelatedApp"),
            self.entry(timestamp="2026-01-01 11:59:59+0000"),
            self.entry(timestamp="2026-01-01 12:15:02+0000"),
            self.entry(eventMessage=f"launch {launch.BUNDLE} on {UDID} environment SECRET=DO NOT EXPORT"),
        ])
        self.assertIn("no scoped launch/runtime events; no cause established", self.status())
        self.assertFalse((self.destination / "simulator-launch.json").exists())
        self.assertEqual(len(self.calls), 1)  # No broader fallback query.

    def test_udid_only_launchd_line_is_retained_but_not_attributed(self):
        self.collect(entries=[self.entry(processImagePath="/usr/libexec/launchd_sim",
                                        eventMessage=f"Service for {UDID} exited")])
        self.assertEqual(len(json.loads((self.destination / "simulator-launch.json").read_text())), 1)
        self.assertIn("diagnosis still required", self.status())

    def test_launchd_error_without_udid_and_runtime_events_remain_scoped(self):
        self.collect(entries=[
            self.entry(processImagePath="/usr/libexec/launchd_sim",
                       eventMessage=f"Launch of {launch.BUNDLE} failed: ipc/mig server died"),
            self.entry(processImagePath="/usr/libexec/simdiskimaged",
                       eventMessage="Performing a cryptex force unmount for iOS 26.0 (23A8464)"),
            self.entry(processImagePath="/usr/libexec/launchd_sim",
                       eventMessage="Unrelated application failed"),
            self.entry(processImagePath="/usr/libexec/cryptexd",
                       eventMessage="Ordinary runtime inventory"),
            self.entry(eventMessage=f"Launch {launch.BUNDLE} on {UDID} environment DYLD_PRIVATE=DO NOT EXPORT"),
        ])
        exported = json.loads((self.destination / "simulator-launch.json").read_text())
        self.assertEqual(len(exported), 2)
        self.assertEqual({item["process"] for item in exported}, {"launchd_sim", "simdiskimaged"})
        self.assertNotIn("DO NOT EXPORT", json.dumps(exported))
        self.assertIn("diagnosis still required", self.status())

    def test_streamed_collection_filters_before_the_export_limit(self):
        path = self.output / "events.ndjson"
        path.write_text("\n".join([json.dumps(self.entry(eventMessage="unrelated",
                        privateConfiguration="X" * 5000))] * 750 + [json.dumps(self.entry())]) + "\n")
        self.assertGreater(path.stat().st_size, launch.MAX_BYTES)
        self.assertLess(path.stat().st_size, launch.RAW_LOG_BYTES)
        command = ["/usr/bin/log", "show", "--style", "ndjson", "--info", "--debug",
                   "--start", START.strftime("%Y-%m-%d %H:%M:%S") + "+00:00",
                   "--end", (START + timedelta(minutes=15)).strftime("%Y-%m-%d %H:%M:%S") + "+00:00",
                   "--predicate", UDID + " " + launch.BUNDLE]
        input_value = mock.Mock()
        input_value.stdout = os.popen("cat " + str(path), "r")
        input_value.stderr = os.popen("cat /dev/null", "r")
        input_value.poll.return_value = 0
        input_value.wait.return_value = 0
        with mock.patch.object(launch.subprocess, "Popen", return_value=input_value):
            kept = launch.stream_output(command, START, START + timedelta(minutes=15), UDID, launch.BUNDLE)
        self.assertEqual(len(kept), 1)
        self.assertEqual(launch.RAW_LOG_BYTES, 4 * 1024 * 1024)

    def test_streamed_large_export_fails_closed(self):
        command = [sys.executable, "-c", "import os; os.write(1,b'x'*" + str(launch.MAX_BYTES + 100) + ")"]
        with self.assertRaisesRegex(ValueError, "single launch event"):
            launch.stream_output(command, START, START + timedelta(minutes=15), UDID, launch.BUNDLE)

    def test_collection_failures_name_the_boundary_not_a_cause(self):
        for error in (TimeoutError(), subprocess.TimeoutExpired("/usr/bin/log", 20)):
            with self.subTest(error=type(error).__name__):
                self.collect(reader=lambda command, error=error: (_ for _ in ()).throw(error))
                self.assertEqual(self.status(), "unavailable: scoped log command timed out\n")
        self.collect(reader=lambda command: b"not JSON DO NOT EXPORT")
        self.assertEqual(self.status(),
                         "unavailable: stream-parse-failure; no launch cause established\n")
        self.assertNotIn("DO NOT EXPORT", self.status())

    def test_log_command_failures_are_distinct_from_stream_parsing(self):
        for error in (OSError("credential /private/command-path DO NOT EXPORT"),
                      ValueError("command failed (exit 1) DO NOT EXPORT"),
                      subprocess.SubprocessError("DO NOT EXPORT")):
            with self.subTest(error=type(error).__name__):
                def reader(command):
                    self.calls.append(command)
                    raise error
                self.collect(reader=reader)
                self.assertEqual(self.status(),
                                 "unavailable: log-command-failure; no launch cause established\n")
                self.assertNotIn("DO NOT EXPORT", self.status())
                self.assertNotIn("/private/", self.status())
                self.assertEqual(len(self.calls), 1)  # No raw-error or broader fallback query.
                self.calls.clear()

    def test_reader_output_limit_has_its_own_category(self):
        self.collect(reader=lambda command: b"x" * (launch.MAX_BYTES + 1))
        self.assertEqual(self.status(),
                         "unavailable: output-limit-failure; no launch cause established\n")

    def test_default_stream_malformed_ndjson_is_not_a_command_or_publication_failure(self):
        real_popen = launch.subprocess.Popen
        def fake_log(command, **kwargs):
            return real_popen([sys.executable, "-c", "print('not NDJSON DO NOT EXPORT')"], **kwargs)
        with mock.patch.object(launch.subprocess, "Popen", side_effect=fake_log):
            launch.collect(self.output, self.destination, now=START + timedelta(minutes=17))
        self.assertEqual(self.status(),
                         "unavailable: stream-parse-failure; no launch cause established\n")
        self.assertNotIn("DO NOT EXPORT", self.status())
        self.assertFalse((self.destination / "simulator-launch.json").exists())

    def test_default_stream_raw_limit_is_not_a_parse_failure(self):
        with mock.patch.object(launch, "RAW_LOG_BYTES", 8):
            with self.assertRaises(launch.OutputLimitError):
                launch.stream_output([sys.executable, "-c", "import os; os.write(1,b'123456789')"],
                                     START, START + timedelta(minutes=15), UDID, launch.BUNDLE)

    def test_default_stream_command_exit_is_a_log_command_failure(self):
        real_popen = launch.subprocess.Popen
        def fake_log(command, **kwargs):
            return real_popen([sys.executable, "-c", "raise SystemExit(7)"], **kwargs)
        with mock.patch.object(launch.subprocess, "Popen", side_effect=fake_log):
            launch.collect(self.output, self.destination, now=START + timedelta(minutes=17))
        self.assertEqual(self.status(),
                         "unavailable: log-command-failure; no launch cause established\n")
        self.assertNotIn("exit", self.status())

    def test_default_stream_raw_limit_is_reported_at_collect_boundary(self):
        real_popen = launch.subprocess.Popen
        def fake_log(command, **kwargs):
            return real_popen([sys.executable, "-c", "import os; os.write(1,b'123456789')"], **kwargs)
        with mock.patch.object(launch.subprocess, "Popen", side_effect=fake_log), \
                mock.patch.object(launch, "RAW_LOG_BYTES", 8):
            launch.collect(self.output, self.destination, now=START + timedelta(minutes=17))
        self.assertEqual(self.status(),
                         "unavailable: output-limit-failure; no launch cause established\n")

    def test_default_stream_timeout_keeps_the_existing_category(self):
        real_popen = launch.subprocess.Popen
        def fake_log(command, **kwargs):
            return real_popen([sys.executable, "-c", "import signal; signal.pause()"], **kwargs)
        with mock.patch.object(launch.subprocess, "Popen", side_effect=fake_log), \
                mock.patch.object(launch.time, "monotonic", side_effect=[0, 100]):
            launch.collect(self.output, self.destination, now=START + timedelta(minutes=17))
        self.assertEqual(self.status(), "unavailable: scoped log command timed out\n")

    def test_partial_evidence_and_context_writes_leave_no_bundle(self):
        evidence = self.destination / "simulator-launch.json"
        context = self.destination / launch.CONTEXT
        status = self.destination / "simulator-launch.status.txt"
        stale = b"stale evidence DO NOT EXPORT"
        for name in (evidence.name, launch.CONTEXT):
            with self.subTest(name=name, failure="partial-write"):
                evidence.write_bytes(stale)
                context.write_bytes(stale)
                status.write_text("stale status")
                real_fdopen = launch.os.fdopen
                descriptors = {}

                def fixture_temporary(dir, prefix, suffix):
                    path = Path(dir) / (prefix + "partial" + suffix)
                    fd = os.open(path, os.O_RDWR | os.O_CREAT | os.O_EXCL, 0o600)
                    descriptors[fd] = path
                    return fd, str(path)

                partial_paths = []

                class PartialWriter:
                    def __init__(self, stream):
                        self.stream = stream

                    def __enter__(self):
                        return self

                    def __exit__(self, *args):
                        return self.stream.__exit__(*args)

                    def write(self, data):
                        self.stream.write(data[:1])
                        raise OSError("private partial write DO NOT EXPORT")

                def partial_fdopen(fd, mode, buffering=-1):
                    stream = real_fdopen(fd, mode, buffering=buffering)
                    if mode == "wb" and buffering == 0 and descriptors.pop(fd).name.startswith(name + "."):
                        partial_paths.append(name)
                        return PartialWriter(stream)
                    return stream

                with mock.patch.object(launch.tempfile, "mkstemp", fixture_temporary), \
                        mock.patch.object(launch.os, "fdopen", partial_fdopen):
                    self.collect()
                self.assertEqual(partial_paths, [name])
                self.assertEqual(self.status(),
                                 "unavailable: publication-failure; no launch cause established\n")
                self.assertFalse(evidence.exists())
                self.assertFalse(context.exists())
                self.assertEqual(descriptors, {})
                self.assertFalse(any(path.suffix == ".tmp" for path in self.destination.iterdir()))

            with self.subTest(name=name, failure="replacement"):
                evidence.write_bytes(stale)
                context.write_bytes(stale)
                status.write_text("stale status")
                real_replace = launch.os.replace

                def fail_replace(source, destination):
                    if Path(destination).name == name:
                        raise OSError("private replacement DO NOT EXPORT")
                    real_replace(source, destination)

                with mock.patch.object(launch.os, "replace", fail_replace):
                    self.collect()
                self.assertEqual(self.status(),
                                 "unavailable: publication-failure; no launch cause established\n")
                self.assertFalse(evidence.exists())
                self.assertFalse(context.exists())
                self.assertFalse(any(path.suffix == ".tmp" for path in self.destination.iterdir()))

    def test_status_failure_propagates_the_fixed_intended_text_and_invalidates_bundle(self):
        evidence = self.destination / "simulator-launch.json"
        context = self.destination / launch.CONTEXT
        status = self.destination / "simulator-launch.status.txt"
        cases = ((None, "captured 1 scoped events; diagnosis still required\n"),
                 ([], "unavailable: no scoped launch/runtime events; no cause established\n"))
        for entries, intended in cases:
            with self.subTest(intended=intended):
                evidence.write_text("stale evidence")
                context.write_text("stale context")
                status.write_text("stale status")
                real_replace = launch.os.replace

                def fail_status(source, destination):
                    real_replace(source, destination)
                    if Path(destination) == status:
                        raise OSError("private status path credential DO NOT EXPORT")

                with mock.patch.object(launch.os, "replace", fail_status), \
                        self.assertRaises(launch.StatusPublicationError) as raised:
                    self.collect(entries=entries)
                self.assertEqual(str(raised.exception), intended)
                self.assertIsNone(raised.exception.__cause__)
                self.assertNotIn("DO NOT EXPORT", str(raised.exception))
                self.assertFalse(status.exists())
                self.assertFalse(evidence.exists())
                self.assertFalse(context.exists())
                self.assertFalse(any(path.suffix == ".tmp" for path in self.destination.iterdir()))

    def test_publication_replaces_complete_files_in_the_artifact_directory(self):
        self.collect()
        real_replace = launch.os.replace
        replacements = []

        def same_directory_replace(source, destination):
            replacements.append((source, destination))
            self.assertEqual(Path(source).parent, Path(destination).parent)
            self.assertTrue(Path(source).name.endswith(".tmp"))
            real_replace(source, destination)

        self.collect()
        with mock.patch.object(launch.os, "replace", same_directory_replace):
            self.collect()
        self.assertEqual([Path(destination).name for _, destination in replacements],
                         [launch.CONTEXT, "simulator-launch.json", "simulator-launch.status.txt"])
        self.assertFalse(any(path.suffix == ".tmp" for path in self.destination.iterdir()))

    def test_publication_failure_does_not_export_paths_or_hide_behind_stream_parse(self):
        real_replace = launch.os.replace
        with mock.patch.object(launch.os, "replace",
                               side_effect=lambda source, destination: real_replace(source, destination)
                               if Path(destination).name != "simulator-launch.json"
                               else (_ for _ in ()).throw(OSError("private evidence DO NOT EXPORT"))):
            self.collect()
        self.assertEqual(self.status(),
                         "unavailable: publication-failure; no launch cause established\n")
        self.assertNotIn("DO NOT EXPORT", self.status())
        self.assertNotIn("/private/", self.status())
        self.assertFalse((self.destination / "simulator-launch.json").exists())
        self.assertFalse((self.destination / launch.CONTEXT).exists())

    def test_status_failure_does_not_mask_an_existing_collection_category(self):
        status = self.destination / "simulator-launch.status.txt"
        intended = "unavailable: log-command-failure; no launch cause established\n"

        def reader(command):
            raise OSError("private command failure DO NOT EXPORT")

        with mock.patch.object(launch, "publish_status",
                               side_effect=launch.StatusPublicationError(intended)) as publish_status, \
                self.assertRaises(launch.StatusPublicationError) as raised:
            self.collect(reader=reader)
        publish_status.assert_called_once_with(status, intended)
        self.assertEqual(str(raised.exception), intended)
        self.assertNotIn("publication-failure", str(raised.exception))
        self.assertNotIn("DO NOT EXPORT", str(raised.exception))
        self.assertFalse((self.destination / "simulator-launch.json").exists())
        self.assertFalse((self.destination / launch.CONTEXT).exists())

    def test_bad_output_timeout_and_command_errors_remain_unavailable(self):
        for raw, category in ((b"not JSON DO NOT EXPORT", "stream-parse"),
                              (b"[]\n", "stream-parse"),
                              (b"\xff", "stream-parse"),
                              (b"x" * (launch.MAX_BYTES + 1), "output-limit")):
            with self.subTest(category=category, raw=raw[:20]):
                self.collect(reader=lambda command, raw=raw: raw)
                self.assertEqual(self.status(),
                                 f"unavailable: {category}-failure; no launch cause established\n")
                self.assertNotIn("DO NOT EXPORT", self.status())
                self.assertFalse((self.destination / "simulator-launch.json").exists())

    def test_real_reader_enforces_timeout_bytes_and_exit_status(self):
        self.assertEqual(launch.bounded_output([sys.executable, "-c", "print('ok')"]), b"ok\n")
        with self.assertRaises(TimeoutError):
            launch.bounded_output([sys.executable, "-c", "import signal; signal.pause()"], timeout=0.05)
        with self.assertRaisesRegex(ValueError, "byte limit"):
            launch.bounded_output([sys.executable, "-c", "import os; os.write(1,b'x'*4096)"], limit=64)
        with self.assertRaisesRegex(ValueError, "exit 7"):
            launch.bounded_output([sys.executable, "-c", "raise SystemExit(7)"])
        self.assertEqual(launch.TIMEOUT, 20)
        self.assertEqual(launch.MAX_BYTES, 128 * 1024)

    def test_existing_collector_calls_the_scoped_capture(self):
        def runner(command, **kwargs):
            return subprocess.CompletedProcess(command, 0, "{}", "")
        with mock.patch.object(launch, "collect") as capture:
            apple_diagnostics.collect(self.output, self.destination, runner)
        capture.assert_called_once_with(self.output, self.destination)
        self.assertEqual((self.destination / "simctl-list.status.txt").read_text(), "0\n")

    def test_record_cli_only_writes_synthetic_scope(self):
        command = [sys.executable, str(ROOT / "scripts/ci/apple_launch_diagnostics.py"),
                   "--record", str(self.output / launch.CONTEXT), "--udid", UDID, "--bundle-id", launch.BUNDLE]
        result = subprocess.run(command, text=True, capture_output=True, timeout=5)
        self.assertEqual(result.returncode, 0, result.stderr)
        context = json.loads((self.output / launch.CONTEXT).read_text())
        self.assertEqual(set(context), {"udid", "bundle_id", "started_at"})
        self.assertEqual(context["udid"], UDID)
        self.assertEqual(datetime.fromisoformat(context["started_at"]).utcoffset(), timedelta(0))


if __name__ == "__main__":
    unittest.main()
