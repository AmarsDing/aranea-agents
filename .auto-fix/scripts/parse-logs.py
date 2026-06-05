#!/usr/bin/env python3
"""Parse CI logs into FailureReport JSON for the Aranea-Agents auto-fix pipeline.

Usage:
    parse-logs.py [--job JOB_NAME] [--source SOURCE] [LOG_FILE]

If LOG_FILE is omitted, reads from stdin. Outputs a JSON array of FailureReport
objects to stdout, compatible with the Go FailureReport struct.

Examples:
    parse-logs.py --job build build.log
    parse-logs.py --job test --source ci test-output.log
    cat failure.log | parse-logs.py --job lint
"""

import argparse
import json
import re
import sys
import uuid
from typing import Optional


# ---------------------------------------------------------------------------
# Failure type constants (must match Go FailureType values)
# ---------------------------------------------------------------------------
FAILURE_TYPE_LINT = "lint_error"
FAILURE_TYPE_TEST = "test_failure"
FAILURE_TYPE_BUILD = "build_failure"
FAILURE_TYPE_PROTO_SYNC = "proto_sync"
FAILURE_TYPE_RUNTIME = "runtime_error"


# ---------------------------------------------------------------------------
# Regex patterns
# ---------------------------------------------------------------------------
RE_GO_BUILD_ERROR = re.compile(r"^([\w./\-]+\.go):(\d+):\d+:\s*(.+)$")
RE_GO_TEST_FAILURE = re.compile(r"^\s+([\w./\-]+\.go):(\d+):\s*(.+)$")
RE_GO_LINT_ERROR = re.compile(r"^([\w./\-]+\.go):(\d+):\d+:\s*(.+)\s+\((\w+)\)$")
RE_PROTO_SYNC = re.compile(
    r"(?i)proto generated files are out of date|generated files.*out of date|wire_gen\.go.*out of date"
)


def new_failure_report(
    failure_type: str,
    source: str,
    job: str,
    file: str = "",
    line: int = 0,
    error_code: str = "",
    message: str = "",
    stack_trace: str = "",
    related_code: str = "",
    metadata: Optional[dict] = None,
) -> dict:
    """Create a FailureReport dict matching the Go struct."""
    return {
        "id": str(uuid.uuid4()),
        "type": failure_type,
        "source": source,
        "job": job,
        "file": file,
        "line": line,
        "error_code": error_code,
        "message": message,
        "stack_trace": stack_trace,
        "related_code": related_code,
        "metadata": metadata or {},
    }


def parse_ci_logs(logs: str, job: str, source: str = "ci") -> list[dict]:
    """Parse CI pipeline logs into FailureReport dicts."""
    if not logs.strip():
        return []

    reports: list[dict] = []

    for line in logs.splitlines():
        # Proto sync check
        if RE_PROTO_SYNC.search(line):
            reports.append(
                new_failure_report(
                    failure_type=FAILURE_TYPE_PROTO_SYNC,
                    source=source,
                    job=job,
                    message=line.strip(),
                    metadata={"raw_line": line},
                )
            )
            continue

        # Lint error (has linter name in parentheses)
        m = RE_GO_LINT_ERROR.match(line)
        if m:
            reports.append(
                new_failure_report(
                    failure_type=FAILURE_TYPE_LINT,
                    source=source,
                    job=job,
                    file=m.group(1),
                    line=int(m.group(2)),
                    message=m.group(3),
                    error_code=m.group(4),
                    metadata={"raw_line": line},
                )
            )
            continue

        # Build error (not indented)
        m = RE_GO_BUILD_ERROR.match(line)
        if m and not line.startswith((" ", "\t")):
            reports.append(
                new_failure_report(
                    failure_type=FAILURE_TYPE_BUILD,
                    source=source,
                    job=job,
                    file=m.group(1),
                    line=int(m.group(2)),
                    message=m.group(3),
                    metadata={"raw_line": line},
                )
            )
            continue

        # Test failure (indented)
        m = RE_GO_TEST_FAILURE.match(line)
        if m:
            reports.append(
                new_failure_report(
                    failure_type=FAILURE_TYPE_TEST,
                    source=source,
                    job=job,
                    file=m.group(1),
                    line=int(m.group(2)),
                    message=m.group(3),
                    metadata={"raw_line": line},
                )
            )
            continue

    return reports


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Parse CI logs into FailureReport JSON"
    )
    parser.add_argument(
        "logfile",
        nargs="?",
        default=None,
        help="Log file path (reads stdin if omitted)",
    )
    parser.add_argument(
        "--job",
        default="build",
        help="CI job name (default: build)",
    )
    parser.add_argument(
        "--source",
        default="ci",
        help='Error source: "ci" or "runtime" (default: ci)',
    )
    args = parser.parse_args()

    if args.logfile:
        with open(args.logfile, encoding="utf-8") as f:
            logs = f.read()
    else:
        logs = sys.stdin.read()

    reports = parse_ci_logs(logs, args.job, args.source)
    json.dump(reports, sys.stdout, indent=2, ensure_ascii=False)
    print()


if __name__ == "__main__":
    main()
