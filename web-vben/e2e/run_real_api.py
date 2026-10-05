#!/usr/bin/env python3
"""Run real frontend API modules against a disposable Go/SQLite/local instance.

No browser, fake HTTP server, service replacement, database seeding, or external
network is used. Requires the existing binary and installed web-vben packages.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import shutil
import signal
import sqlite3
import struct
import subprocess
import tempfile
import threading
import time
import zlib


ROOT = Path(__file__).resolve().parents[2]
WEB = ROOT / "web-vben"


def make_png(seed):
    def chunk(kind, data):
        return struct.pack(">I", len(data)) + kind + data + struct.pack(">I", zlib.crc32(kind + data))

    pixels = b"".join(b"\0" + bytes((seed + x * 17 + y * 23) % 256 for x in range(24)) for y in range(6))
    return b"\x89PNG\r\n\x1a\n" + chunk(b"IHDR", struct.pack(">IIBBBBB", 8, 6, 8, 2, 0, 0, 0)) + chunk(b"IDAT", zlib.compress(pixels)) + chunk(b"IEND", b"")


def run(args):
    binary = args.binary.resolve()
    if not binary.is_file():
        raise RuntimeError("Build bin/imgnest-vben first; this runner never builds or installs dependencies")
    node = shutil.which(args.node)
    if not node:
        raise RuntimeError("Node executable not found; use --node or the locked toolchain on PATH")
    evidence = args.results.resolve()
    evidence.mkdir(parents=True, exist_ok=True)
    env = {key: value for key, value in os.environ.items() if not key.startswith("IMGNEST_")}
    env["PATH"] = str(Path(node).parent) + os.pathsep + env.get("PATH", "")
    report = {
        "result": "FAIL", "real_tcp": True, "browser_executed": False,
        "database": "temporary SQLite with production migrations",
        "storage": "temporary local driver with real libvips",
        "binary_sha256": hashlib.sha256(binary.read_bytes()).hexdigest(),
        "node_version": subprocess.check_output([node, "--version"], text=True).strip(),
    }
    logs = []
    sensitive = []
    test_output = ""
    process = None
    reader = None
    failure = None
    with tempfile.TemporaryDirectory(prefix="imgnest-real-api-") as directory:
        state = Path(directory)
        database = state / "imgnest.db"
        config = state / "config.yaml"
        config.write_text(
            "database:\n  driver: sqlite\n  dsn: " + str(database) +
            "\nserver:\n  addr: 127.0.0.1:0\nimages:\n  thumb_cache: " + str(state / "thumbs") +
            "\nsecurity:\n  master_key: \"\"\n"
        )
        nonce = secrets.token_hex(6)
        fixture = {
            "nonce": nonce,
            "adminEmail": f"admin-{nonce}@imgnest.invalid",
            "adminPassword": secrets.token_urlsafe(32),
            "userEmail": f"member-{nonce}@imgnest.invalid",
            "userPassword": secrets.token_urlsafe(32),
            "nextPassword": secrets.token_urlsafe(32),
            "images": [], "secretsPath": str(state / "secrets.json"),
            "transportPath": str(state / "transport.json"),
        }
        sensitive.extend(fixture[key] for key in ("adminPassword", "userPassword", "nextPassword"))
        pixel_seed = secrets.randbelow(256)
        for index in range(2):
            image = state / f"fixture-{index}.png"
            image.write_bytes(make_png(pixel_seed + index * 37))
            fixture["images"].append(str(image))

        def cli(arguments, stdin=None):
            result = subprocess.run([str(binary), "--config", str(config), *arguments],
                                    input=stdin, text=True, capture_output=True, env=env, timeout=60)
            logs.append(result.stdout + result.stderr)
            if result.returncode:
                raise RuntimeError(f"Fixture CLI failed: {arguments[0]} (exit {result.returncode})")

        try:
            cli(["migrate"])
            cli(["init-admin", "--username", f"admin-{nonce}", "--email", fixture["adminEmail"]], fixture["adminPassword"] + "\n")
            cli(["init-local", "--root", str(state / "objects"), "--base-url", "http://127.0.0.1"])
            ready = threading.Event()
            address = []
            process = subprocess.Popen([str(binary), "--config", str(config), "serve"],
                                       stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, env=env)

            def read_logs():
                for line in process.stdout:
                    logs.append(line)
                    try:
                        item = json.loads(line)
                    except ValueError:
                        continue
                    if item.get("msg") == "server listening" and item.get("address"):
                        address.append(item["address"])
                        ready.set()

            reader = threading.Thread(target=read_logs, daemon=True)
            reader.start()
            deadline = time.monotonic() + 20
            while not ready.wait(0.1):
                if process.poll() is not None or time.monotonic() >= deadline:
                    raise RuntimeError("Go server did not report an ephemeral loopback address")
            fixture["origin"] = "http://" + address[0]
            fixture_path = state / "fixture.json"
            fixture_path.write_text(json.dumps(fixture))
            fixture_path.chmod(0o600)
            env["IMGNEST_REAL_API_FIXTURE"] = str(fixture_path)
            print("Running frontend API modules against the real isolated Go/SQLite/local server", flush=True)
            result = subprocess.run([
                node, str(WEB / "node_modules/vitest/vitest.mjs"), "run", "--config", "e2e/real-api.config.ts",
                "--reporter=verbose", "--reporter=json", "--outputFile=" + str(state / "vitest.json"),
            ], cwd=WEB, env=env, capture_output=True, text=True, timeout=240)
            test_output = result.stdout + result.stderr
            if (state / "secrets.json").exists():
                sensitive.extend(json.loads((state / "secrets.json").read_text()))
            if (state / "vitest.json").exists():
                vitest = json.loads((state / "vitest.json").read_text())
                report["tests"] = {key: vitest[key] for key in ("numTotalTests", "numPassedTests", "numFailedTests", "numPendingTests")}
                report["checks"] = [{"name": item["title"], "status": item["status"]}
                                    for suite in vitest["testResults"] for item in suite["assertionResults"]]
            if (state / "transport.json").exists():
                report["native_fetch_requests"] = json.loads((state / "transport.json").read_text())
            if result.returncode:
                raise RuntimeError(f"Real API integration suite failed (exit {result.returncode})")
            # Independent read-only verification of actual persistent state.
            with sqlite3.connect(database) as connection:
                counts = {table: connection.execute(f"SELECT count(*) FROM {table}").fetchone()[0]
                          for table in ("users", "images", "image_exif", "albums", "tokens", "schema_migrations")}
                used = connection.execute("SELECT coalesce(sum(used_bytes),0) FROM users").fetchone()[0]
                member_count = connection.execute("SELECT count(*) FROM users WHERE id > 0").fetchone()[0]
                guest_count = connection.execute("SELECT count(*) FROM users WHERE id = 0 AND status = 'disabled'").fetchone()[0]
                hashes = [row[0] for row in connection.execute("SELECT token_hash FROM tokens")]
                cover_schema = connection.execute("PRAGMA foreign_key_check").fetchall()
            # Migration 0003 owns a disabled id=0 guest anchor, excluded from the
            # administrative account list. Only the two id>0 accounts are ours.
            assert member_count == 2 and guest_count == 1 and counts["users"] == 3 and counts["tokens"] == 1, "Unexpected persisted fixture identities/tokens"
            assert all(counts[table] == 0 for table in ("images", "image_exif", "albums")), "Fixture lifecycle left database rows"
            assert used == 0 and not cover_schema, "Quota or foreign-key invariant failed"
            assert not any(value in sensitive for value in hashes), "Plaintext token persisted"
            objects = [path for path in (state / "objects").rglob("*") if path.is_file()]
            assert not objects, "Purged fixtures left physical objects"
            report["persistence"] = {"counts": counts, "fixture_accounts": member_count, "disabled_guest_anchors": guest_count, "charged_bytes": used, "foreign_key_violations": 0, "remaining_object_files": len(objects), "token_plaintext_absent": True}
            report["result"] = "PASS"
        except Exception as error:
            failure = str(error)
        finally:
            if process is not None and process.poll() is None:
                process.send_signal(signal.SIGINT)
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait(timeout=5)
                    failure = "Go server required forced cleanup"
            if reader is not None:
                reader.join(timeout=5)
            if process is not None:
                process.stdout.close()
                report["server_exit_code"] = process.returncode
                report["graceful_shutdown"] = process.returncode == 0 and not reader.is_alive()
                if not report["graceful_shutdown"]:
                    failure = failure or "Go server failed graceful cleanup"
            combined_logs = "".join(logs)
            leaked = any(value and value in combined_logs + test_output for value in sensitive)
            report["credentials_absent_from_logs"] = not leaked
            if leaked:
                failure = "Generated fixture credentials appeared in captured output (redacted)"
            for value in sensitive:
                combined_logs = combined_logs.replace(value, "[REDACTED]")
                test_output = test_output.replace(value, "[REDACTED]")
            (evidence / "real-api-server.log").write_text(combined_logs)
            (evidence / "real-api-vitest.log").write_text(test_output)
            print(test_output, end="", flush=True)
    report["temporary_state_removed"] = not database.exists()
    if failure:
        for value in sensitive:
            failure = failure.replace(value, "[REDACTED]")
        report["result"] = "FAIL"
        report["failure"] = failure
    (evidence / "real-api-report.json").write_text(json.dumps(report, indent=2, ensure_ascii=False) + "\n")
    print(f"{report['result']}: real frontend API → Go HTTP → SQLite/local; browser not executed")
    if failure:
        print("Failure: " + failure)
    print(f"Evidence: {evidence / 'real-api-report.json'}")
    return 1 if failure else 0


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--binary", type=Path, default=ROOT / "bin/imgnest-vben")
    parser.add_argument("--node", default="node")
    parser.add_argument("--results", type=Path, default=WEB / "e2e/results")
    raise SystemExit(run(parser.parse_args()))
