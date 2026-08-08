#!/usr/bin/env python3
"""Prepare release-only Helm metadata without modifying the source Chart.yaml."""

from __future__ import annotations

import argparse
import json
import re
import subprocess
import sys
from pathlib import Path

CONVENTIONAL_COMMIT_RE = re.compile(
    r"^(?P<type>[A-Za-z]+)(?:\([^)]+\))?(?P<breaking>!)?:\s+(?P<description>.+)$"
)

CHANGE_KIND_BY_COMMIT_TYPE = {
    "feat": "added",
    "fix": "fixed",
    "perf": "changed",
    "refactor": "changed",
    "security": "security",
}

SEMVER_RE = re.compile(
    r"^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)"
    r"(?:-[0-9A-Za-z.-]+)?(?:\+[0-9A-Za-z.-]+)?$"
)


def run_git(repo_root: Path, *args: str, allow_failure: bool = False) -> str:
    result = subprocess.run(
        ["git", *args],
        cwd=repo_root,
        check=False,
        text=True,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
    )
    if result.returncode != 0:
        if allow_failure:
            return ""
        raise RuntimeError(result.stderr.strip() or f"git {' '.join(args)} failed")
    return result.stdout.strip()


def resolve_previous_ref(repo_root: Path, current_ref: str) -> str:
    # If current_ref is an already-created release tag, current_ref^ finds the
    # tag before it. If the release tag does not exist yet, HEAD^ still resolves
    # the latest reachable release tag.
    previous = run_git(
        repo_root,
        "describe",
        "--tags",
        "--abbrev=0",
        f"{current_ref}^",
        allow_failure=True,
    )
    if previous:
        return previous

    return run_git(
        repo_root,
        "describe",
        "--tags",
        "--abbrev=0",
        current_ref,
        allow_failure=True,
    )


def conventional_changes(repo_root: Path, previous_ref: str, current_ref: str) -> list[tuple[str, str]]:
    revision_range = f"{previous_ref}..{current_ref}" if previous_ref else current_ref
    output = run_git(
        repo_root,
        "log",
        "--no-merges",
        "--format=%s",
        revision_range,
        allow_failure=False,
    )

    changes: list[tuple[str, str]] = []
    seen: set[tuple[str, str]] = set()

    for subject in output.splitlines():
        match = CONVENTIONAL_COMMIT_RE.match(subject.strip())
        if not match:
            continue

        commit_type = match.group("type").lower()
        kind = CHANGE_KIND_BY_COMMIT_TYPE.get(commit_type)
        if kind is None:
            # docs/test/ci/build/chore and unknown commit types are intentionally
            # excluded from the user-facing Artifact Hub changelog.
            continue

        description = match.group("description").strip()
        if match.group("breaking"):
            kind = "changed"
            description = f"BREAKING: {description}"

        entry = (kind, description)
        if entry not in seen:
            seen.add(entry)
            changes.append(entry)

    return changes


def remove_changes_annotation(lines: list[str]) -> list[str]:
    result: list[str] = []
    index = 0

    while index < len(lines):
        if lines[index].startswith("  artifacthub.io/changes:"):
            index += 1
            while index < len(lines):
                line = lines[index]
                if line.strip() == "":
                    index += 1
                    break
                if line.startswith("    "):
                    index += 1
                    continue
                break
            continue

        result.append(lines[index])
        index += 1

    return result


def update_scalar(lines: list[str], key: str, value: str) -> list[str]:
    pattern = re.compile(rf"^{re.escape(key)}:\s*.*$")
    replacement = f'{key}: "{value}"' if key == "appVersion" else f"{key}: {value}"

    matches = 0
    output: list[str] = []
    for line in lines:
        if pattern.match(line):
            output.append(replacement)
            matches += 1
        else:
            output.append(line)

    if matches != 1:
        raise RuntimeError(f"expected exactly one top-level {key}, found {matches}")
    return output


def add_changes_annotation(lines: list[str], changes: list[tuple[str, str]]) -> list[str]:
    if not changes:
        return lines

    insert_at = next(
        (i for i, line in enumerate(lines) if line.startswith("  artifacthub.io/signKey:")),
        None,
    )
    if insert_at is None:
        raise RuntimeError("artifacthub.io/signKey annotation not found")

    block = ["  artifacthub.io/changes: |"]
    for kind, description in changes:
        block.append(f"    - kind: {kind}")
        # JSON string syntax is valid YAML and safely escapes quotes/newlines.
        block.append(f"      description: {json.dumps(description, ensure_ascii=False)}")
    block.append("")

    return lines[:insert_at] + block + lines[insert_at:]


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--chart", required=True, type=Path)
    parser.add_argument("--repo-root", required=True, type=Path)
    parser.add_argument("--version", required=True)
    parser.add_argument("--app-version", required=True)
    parser.add_argument("--current-ref", default="HEAD")
    parser.add_argument("--previous-ref", default="")
    args = parser.parse_args()

    chart_version = args.version.removeprefix("v")
    if not SEMVER_RE.fullmatch(chart_version):
        parser.error(f"invalid Helm semantic version: {chart_version}")

    previous_ref = args.previous_ref or resolve_previous_ref(args.repo_root, args.current_ref)
    changes = conventional_changes(args.repo_root, previous_ref, args.current_ref)

    lines = args.chart.read_text(encoding="utf-8").splitlines()
    lines = remove_changes_annotation(lines)
    lines = update_scalar(lines, "version", chart_version)
    lines = update_scalar(lines, "appVersion", args.app_version)
    lines = add_changes_annotation(lines, changes)

    args.chart.write_text("\n".join(lines) + "\n", encoding="utf-8")

    if previous_ref:
        print(f"Artifact Hub changes: {previous_ref}..{args.current_ref}", file=sys.stderr)
    else:
        print(f"Artifact Hub changes: full history up to {args.current_ref}", file=sys.stderr)
    print(f"Artifact Hub entries: {len(changes)}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
