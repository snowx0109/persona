"""Bounded launch/runtime evidence for one disposable CI simulator and readiness window."""

import argparse
from datetime import datetime, timedelta, timezone
import json
import os
from pathlib import Path
import re
import selectors
import stat
import subprocess
import tempfile
import time
import uuid

CONTEXT = "simulator-readiness-context.json"
BUNDLE = "org.example.persona.ci"
READINESS_SECONDS = 900
TIMEOUT = 20
MAX_BYTES = 128 * 1024
RAW_LOG_BYTES = 4 * 1024 * 1024
PROCESSES = ("CoreSimulatorService", "launchd_sim", "runningboardd", "SpringBoard", "testmanagerd", "lsd",
             "installd", "assertiond", "debugserver", "Persona")
RUNTIME_PROCESSES = ("simdiskimaged", "cryptexd")
SECRET = re.compile(r"environment|DYLD_|bearer\s|token=|password|secret|credential", re.I)
COLLECTION_ERRORS = (OSError, ValueError, TypeError, KeyError, AttributeError, subprocess.SubprocessError)


class StreamParseError(ValueError):
    """Malformed NDJSON; its raw content must never become an exported status."""


class OutputLimitError(ValueError):
    """A bounded command, stream or export exceeded its existing byte limit."""


class StatusPublicationError(Exception):
    """The caller must keep an unwritten status distinct from collection categories."""


def invalidate(status, evidence, context):
    for path in (status, evidence, context):
        try:
            path.unlink(missing_ok=True)
        except OSError:
            # A failed invalidation must not replace the original failure category.
            pass


def publish(path, data):
    # Complete the bounded write before replacement exposes it as current evidence.
    if isinstance(data, str):
        data = data.encode()
    fd, temporary = tempfile.mkstemp(dir=path.parent, prefix=path.name + ".", suffix=".tmp")
    try:
        with os.fdopen(fd, "wb", buffering=0) as stream:
            stream.write(data)
        os.replace(temporary, path)
    finally:
        try:
            os.unlink(temporary)
        except FileNotFoundError:
            pass


def publish_status(status, text):
    try:
        publish(status, text)
    except OSError:
        # Never recategorize an unwritten status as the collector's prior stage.
        raise StatusPublicationError(text) from None


def unavailable(status, category):
    publish_status(status, f"unavailable: {category}-failure; no launch cause established\n")


def identity(udid, bundle):
    if str(uuid.UUID(udid)).upper() != udid or bundle != BUNDLE:
        raise ValueError("invalid CI identity")


def record(path, udid, bundle):
    identity(udid, bundle)
    path.write_text(json.dumps({"udid": udid, "bundle_id": bundle,
                               "started_at": datetime.now(timezone.utc).isoformat()}))


def read_scope_file(path):
    fd = os.open(path, os.O_RDONLY | os.O_NONBLOCK | os.O_NOFOLLOW)
    try:
        info = os.fstat(fd)
        if not stat.S_ISREG(info.st_mode) or info.st_size > 1024:
            raise ValueError("invalid scope file")
        data = os.read(fd, 1025)
        if len(data) > 1024:
            raise ValueError("scope file exceeded byte limit")
        return data.decode()
    finally:
        os.close(fd)


def scope(output, now):
    context = json.loads(read_scope_file(output / CONTEXT))
    if set(context) != {"udid", "bundle_id", "started_at"}:
        raise ValueError("invalid context fields")
    identity(context["udid"], context["bundle_id"])
    if read_scope_file(output / "simulator.log").strip() != context["udid"]:
        raise ValueError("owned simulator does not match")
    start = datetime.fromisoformat(context["started_at"])
    if start.utcoffset() != timedelta(0) or start > now or now - start > timedelta(hours=2):
        raise ValueError("invalid context time")
    end = min(now, start + timedelta(seconds=READINESS_SECONDS))
    if end <= start:
        raise ValueError("empty context time window")
    return context, start, end


def bounded_output(command, timeout=TIMEOUT, limit=MAX_BYTES):
    """A bounded command reader; stderr is never exported as diagnostic evidence."""
    process = subprocess.Popen(command, stdin=subprocess.DEVNULL,
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    data = bytearray()
    errors = bytearray()
    deadline = time.monotonic() + timeout
    try:
        with selectors.DefaultSelector() as selector:
            selector.register(process.stdout, selectors.EVENT_READ, data)
            selector.register(process.stderr, selectors.EVENT_READ, errors)
            while selector.get_map():
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise TimeoutError("command timed out")
                for key, _ in selector.select(remaining):
                    chunk = os.read(key.fd, 4096)
                    if not chunk:
                        selector.unregister(key.fileobj)
                        continue
                    key.data.extend(chunk[:limit + 1 - len(key.data)])
                    if len(key.data) > limit:
                        raise OutputLimitError("command exceeded byte limit")
        code = process.wait(timeout=max(0, deadline - time.monotonic()))
        if code:
            raise ValueError(f"command failed (exit {code})")
        return bytes(data)
    finally:
        if process.poll() is None:
            process.kill()
            process.wait()
        process.stdout.close()
        process.stderr.close()


def anchored(pattern, text):
    if re.fullmatch(r"[0-9A-Fa-f-]{36}", pattern):
        return re.search(r"(?<![0-9A-Fa-f])" + re.escape(pattern) + r"(?![0-9A-Fa-f])", text, re.I)
    return re.search(r"(?<![\w.-])" + re.escape(pattern) + r"(?![\w.-])", text, re.I)


def ndjson(raw):
    try:
        entry = json.loads(raw)
    except (json.JSONDecodeError, UnicodeDecodeError):
        raise StreamParseError("invalid NDJSON") from None
    if not isinstance(entry, dict):
        raise StreamParseError("invalid launch log entry")
    return entry


def relevant(entry, udid, bundle, start, end):
    """Launchd may omit the UDID, so require a process allowlist and one owned anchor."""
    try:
        message = entry["eventMessage"]
        process = Path(entry["processImagePath"]).name
        stamp = datetime.fromisoformat(entry["timestamp"])
    except (KeyError, TypeError, ValueError):
        return None
    if stamp.utcoffset() is None or not start <= stamp <= end or SECRET.search(message):
        return None
    if process in RUNTIME_PROCESSES:
        if not re.search(r"mount|cryptex|duplicate|disappear|23A8464|iOS[ _]26\.0", message, re.I):
            return None
    elif process in PROCESSES:
        if not (anchored(udid, message) or anchored(bundle, message) or anchored(bundle + ".tests", message)):
            return None
    else:
        return None
    return {"timestamp": stamp.isoformat(), "process": process, "eventMessage": message}


def stream_output(command, start, end, udid, bundle, timeout=TIMEOUT):
    """Filter NDJSON while reading, before either raw or retained output can exhaust a cap."""
    process = subprocess.Popen(command, stdin=subprocess.DEVNULL,
                               stdout=subprocess.PIPE, stderr=subprocess.PIPE)
    deadline = time.monotonic() + timeout
    raw = 0
    line = bytearray()
    kept = []
    try:
        with selectors.DefaultSelector() as selector:
            selector.register(process.stdout, selectors.EVENT_READ, "events")
            selector.register(process.stderr, selectors.EVENT_READ, "errors")
            while selector.get_map():
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise TimeoutError("launch log timed out")
                for key, _ in selector.select(remaining):
                    chunk = os.read(key.fd, 4096)
                    if not chunk:
                        selector.unregister(key.fileobj)
                        continue
                    raw += len(chunk)
                    if raw > RAW_LOG_BYTES:
                        raise OutputLimitError("raw launch log exceeded byte limit")
                    if key.data == "errors":
                        continue
                    line.extend(chunk)
                    while True:
                        try:
                            newline = line.index(10)
                        except ValueError:
                            break
                        entry = line[:newline]
                        del line[:newline + 1]
                        item = relevant(ndjson(entry), udid, bundle, start, end)
                        if item:
                            kept.append(item)
                    if len(line) > MAX_BYTES:
                        raise OutputLimitError("single launch event exceeded byte limit")
        code = process.wait(timeout=max(0, deadline - time.monotonic()))
        if code:
            raise ValueError(f"launch log command failed (exit {code})")
        if line:
            item = relevant(ndjson(line), udid, bundle, start, end)
            if item:
                kept.append(item)
        encoded = json.dumps(kept, ensure_ascii=False).encode()
        if len(encoded) > MAX_BYTES:
            raise OutputLimitError("launch log export exceeded byte limit")
        return kept
    finally:
        if process.poll() is None:
            process.kill()
            process.wait()
        process.stdout.close()
        process.stderr.close()


def collect(output, destination, reader=bounded_output, now=None, stream_reader=stream_output, scope_root=None):
    status = destination / "simulator-launch.status.txt"
    evidence = destination / "simulator-launch.json"
    context = destination / CONTEXT
    category = "publication"
    try:
        invalidate(status, evidence, context)
        category = "scope-validation"
        context_data, start, end = scope(scope_root or output, now or datetime.now(timezone.utc))
        udid, bundle = context_data["udid"], context_data["bundle_id"]
        predicate = ("(" + " OR ".join(f'process == "{name}"' for name in PROCESSES + RUNTIME_PROCESSES) +
                     f') AND (eventMessage CONTAINS[c] "{udid}" OR eventMessage CONTAINS "{bundle}"' +
                     ' OR process == "simdiskimaged" OR process == "cryptexd")')
        command = ["/usr/bin/log", "show", "--style", "ndjson", "--info", "--debug",
                   "--start", start.strftime("%Y-%m-%d %H:%M:%S") + "+00:00",
                   "--end", end.strftime("%Y-%m-%d %H:%M:%S") + "+00:00", "--predicate", predicate]
        category = "log-command"
        if reader is bounded_output and stream_reader is stream_output:
            kept = stream_reader(command, start, end, udid, bundle)
        else:
            raw = reader(command)
            if len(raw) > MAX_BYTES:
                raise OutputLimitError("launch log exceeded byte limit")
            category = "stream-parse"
            kept = []
            for line in raw.decode().splitlines():
                if not line.strip():
                    continue
                item = relevant(ndjson(line), udid, bundle, start, end)
                if item:
                    kept.append(item)
        if not kept:
            category = "publication"
            publish_status(status, "unavailable: no scoped launch/runtime events; no cause established\n")
            return
        category = "publication"
        encoded = json.dumps(kept, ensure_ascii=False).encode()
        if len(encoded) > MAX_BYTES:
            raise OutputLimitError("export exceeded byte limit")
        # Context alone cannot be mistaken for a bundle; evidence is its final commit point.
        publish(context, json.dumps(context_data))
        try:
            publish(evidence, encoded)
            publish_status(status, f"captured {len(kept)} scoped events; diagnosis still required\n")
        except COLLECTION_ERRORS + (StatusPublicationError,):
            evidence.unlink(missing_ok=True)
            context.unlink(missing_ok=True)
            raise
    except StatusPublicationError:
        invalidate(status, evidence, context)
        raise
    except COLLECTION_ERRORS as error:
        invalidate(status, evidence, context)
        if isinstance(error, (TimeoutError, subprocess.TimeoutExpired)):
            # Preserve the established command-deadline category, including the default stream.
            publish_status(status, "unavailable: scoped log command timed out\n")
        elif isinstance(error, OutputLimitError):
            unavailable(status, "output-limit")
        elif isinstance(error, StreamParseError):
            unavailable(status, "stream-parse")
        else:
            # Only fixed stage categories are exported, never raw errors or a fallback query.
            unavailable(status, category)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--record", type=Path, required=True)
    parser.add_argument("--udid", required=True)
    parser.add_argument("--bundle-id", required=True)
    args = parser.parse_args()
    record(args.record, args.udid, args.bundle_id)


if __name__ == "__main__":
    main()
