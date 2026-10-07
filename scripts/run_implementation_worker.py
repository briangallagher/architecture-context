#!/usr/bin/env python3
"""Run one implementation/review assignment without retries or fallback.

Example:
    python3 scripts/run_implementation_worker.py --prompt path/to/prompt.txt \
        --output-dir logs/task/attempt-1 --model gpt-5.6-sol --effort high

The output directory must be new. Exit status follows the CLI (130 on interruption,
127 on launch failure). A successful exit is not implementation acceptance.
"""

import argparse
import hashlib
import json
import subprocess
from datetime import UTC, datetime
from pathlib import Path


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--prompt", required=True, type=Path)
    parser.add_argument("--output-dir", required=True, type=Path)
    parser.add_argument("--model", required=True)
    parser.add_argument("--harness", choices=["codex", "claude"], default="codex")
    parser.add_argument(
        "--effort", required=True, choices=["low", "medium", "high", "xhigh"]
    )
    parser.add_argument("--cwd", type=Path, default=Path.cwd())
    args = parser.parse_args()
    if args.harness == "claude" and args.effort == "xhigh":
        parser.error("Claude review launches support low, medium, or high effort")
    prompt = args.prompt.read_bytes()
    cwd = args.cwd.resolve(strict=True)
    output = args.output_dir.resolve()
    output.mkdir(parents=True, exist_ok=False)
    (output / "prompt.txt").write_bytes(prompt)
    command = [
        "codex", "exec", "--model", args.model,
        "-c", f"model_reasoning_effort={json.dumps(args.effort)}",
        "--sandbox", "workspace-write", "--json",
        "--output-last-message", str(output / "final.txt"), "-",
    ]
    if args.harness == "claude":
        command = [
            "claude", "-p", "--model", args.model, "--effort", args.effort,
            "--output-format", "stream-json", "--verbose",
            "--tools", "Read,Glob,Grep,Bash", "--permission-mode", "dontAsk",
            "--allowedTools", "Read", "Glob", "Grep", "Bash",
        ]
    metadata = {
        "command": command, "cwd": str(cwd),
        "harness": args.harness,
        "requested_model": args.model, "effort": args.effort,
        "prompt_sha256": hashlib.sha256(prompt).hexdigest(),
        "started_at": datetime.now(UTC).isoformat(),
    }
    (output / "invocation.json").write_text(json.dumps(metadata, indent=2) + "\n")
    code = 127
    error = None
    process = None
    print(f"Starting {args.model} ({args.effort}); evidence: {output}", flush=True)
    with (output / "prompt.txt").open("rb") as inp, \
            (output / "stdout.jsonl").open("wb") as stdout, \
            (output / "stderr.txt").open("wb") as stderr:
        try:
            process = subprocess.Popen(
                command, stdin=inp, stdout=stdout, stderr=stderr, cwd=cwd,
            )
            code = process.wait()
        except KeyboardInterrupt:
            if process is not None:
                process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
            code = 130
            error = "Interrupted; inspect retained evidence before resuming."
        except OSError as exc:
            error = str(exc)
    sessions = []
    failures = []
    reported_models = []
    with (output / "stdout.jsonl").open(errors="replace") as events:
        for line in events:
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            if not isinstance(event, dict):
                continue
            if event.get("type") == "thread.started":
                sessions.append(event.get("thread_id"))
            if event.get("session_id") and event["session_id"] not in sessions:
                sessions.append(event["session_id"])
            if event.get("model") and event["model"] not in reported_models:
                reported_models.append(event["model"])
            if event.get("type") in {"error", "turn.failed"}:
                failures.append(event)
            if args.harness == "claude" and event.get("type") == "result":
                (output / "final.txt").write_text(str(event.get("result", "")))
                if event.get("is_error"):
                    failures.append(event)
                for model in event.get("modelUsage", {}):
                    if model not in reported_models:
                        reported_models.append(model)
    result = {
        "exit_code": code, "finished_at": datetime.now(UTC).isoformat(),
        "session_ids": sessions, "reported_models": reported_models,
        "failures": failures, "launch_error": error,
    }
    (output / "result.json").write_text(json.dumps(result, indent=2) + "\n")
    print(json.dumps(result), flush=True)
    return code if code >= 0 else 128 - code


if __name__ == "__main__":
    raise SystemExit(main())
