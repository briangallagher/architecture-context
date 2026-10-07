#!/usr/bin/env python3
"""Validate platforms.yaml schema and safety constraints."""

import re
import sys
from pathlib import Path

import yaml

PLATFORMS_FILE = Path(__file__).resolve().parent.parent / "platforms.yaml"
INTEGRATION_STATUSES = {"current", "planned", "not-integrated", "unknown"}

KNOWN_KEYS = {
    "suffix",
    "branch",
    "version",
    "orgs",
    "extra_orgs",
    "extra_repos",
    "exclude_repos",
    "repo_branch_policies",
    "exclude_components",
    "include_components",
    "component_overrides",
    "integration_status",
    "post_checkout",
    "sync_config",
    "reuse_from",
}


def _check_integration_status(value, label, errors):
    if not isinstance(value, str) or value not in INTEGRATION_STATUSES:
        expected = ", ".join(sorted(INTEGRATION_STATUSES))
        errors.append(f"'{label}' must be one of: {expected}")


def _check_integration_status_map(value, errors):
    if not isinstance(value, dict):
        errors.append(
            "'integration_status' must be a mapping,"
            f" got {type(value).__name__}"
        )
        return
    for alias, status in value.items():
        if not isinstance(alias, str) or not alias.strip():
            errors.append("'integration_status' keys must be non-empty strings")
            continue
        _check_integration_status(
            status, f"integration_status.{alias}", errors
        )


def _check_str(value, label, errors):
    if not isinstance(value, str):
        errors.append(f"'{label}' must be a string, got {type(value).__name__}")


def _check_list_of_str(value, label, errors):
    if not isinstance(value, list):
        errors.append(
            f"'{label}' must be a list, got {type(value).__name__}"
        )
        return
    for i, item in enumerate(value):
        if not isinstance(item, str):
            errors.append(
                f"'{label}[{i}]' must be a string,"
                f" got {type(item).__name__}"
            )


def _check_exclude_patterns(patterns, label, errors):
    if not isinstance(patterns, list):
        errors.append(
            f"'{label}' must be a list of glob patterns,"
            f" got {type(patterns).__name__}"
        )
        return
    for i, p in enumerate(patterns):
        if not isinstance(p, str):
            errors.append(
                f"'{label}[{i}]' must be a string,"
                f" got {type(p).__name__}"
            )
        elif ".." in p:
            errors.append(
                f"'{label}[{i}]': pattern contains '..'"
                " (path traversal)"
            )
        elif p.startswith("/"):
            errors.append(
                f"'{label}[{i}]': pattern is an absolute path"
            )


def _check_extra_orgs(value, errors):
    if not isinstance(value, list):
        errors.append(
            f"'extra_orgs' must be a list,"
            f" got {type(value).__name__}"
        )
        return
    for i, entry in enumerate(value):
        if isinstance(entry, str):
            continue
        if not isinstance(entry, dict):
            errors.append(
                f"'extra_orgs[{i}]' must be a string or dict,"
                f" got {type(entry).__name__}"
            )
            continue
        if "org" not in entry:
            errors.append(f"'extra_orgs[{i}]': missing required 'org'")
        elif not isinstance(entry["org"], str):
            errors.append(f"'extra_orgs[{i}].org' must be a string")
        for k in ("branch", "suffix"):
            if k in entry and not isinstance(entry[k], str):
                errors.append(
                    f"'extra_orgs[{i}].{k}' must be a string"
                )


def _check_extra_repos(value, errors):
    if not isinstance(value, list):
        errors.append(
            f"'extra_repos' must be a list,"
            f" got {type(value).__name__}"
        )
        return
    allowed = {
        "org", "repo", "branch", "suffix", "exclude_files", "protocol",
        "name_prefix",
    }
    for i, entry in enumerate(value):
        if not isinstance(entry, dict):
            errors.append(
                f"'extra_repos[{i}]' must be a dict,"
                f" got {type(entry).__name__}"
            )
            continue
        unknown = set(entry.keys()) - allowed
        if unknown:
            errors.append(
                f"'extra_repos[{i}]': unrecognized keys:"
                f" {', '.join(sorted(unknown))}"
            )
        for req in ("org", "repo"):
            if req not in entry:
                errors.append(
                    f"'extra_repos[{i}]': missing required '{req}'"
                )
            elif not isinstance(entry[req], str):
                errors.append(
                    f"'extra_repos[{i}].{req}' must be a string"
                )
        for opt in ("branch", "suffix", "name_prefix"):
            if opt in entry and not isinstance(entry[opt], str):
                errors.append(
                    f"'extra_repos[{i}].{opt}' must be a string"
                )
        if "name_prefix" in entry:
            prefix = entry["name_prefix"]
            if "/" in prefix or "\\" in prefix or ".." in prefix:
                errors.append(
                    f"'extra_repos[{i}].name_prefix' must be a safe name prefix"
                )
        if "protocol" in entry:
            proto = entry["protocol"]
            if proto not in ("https", "ssh"):
                errors.append(
                    f"'extra_repos[{i}].protocol' must be"
                    f" 'https' or 'ssh', got '{proto}'"
                )
        if "exclude_files" in entry:
            _check_exclude_patterns(
                entry["exclude_files"],
                f"extra_repos[{i}].exclude_files",
                errors,
            )


def _check_include_components(value, errors):
    if not isinstance(value, list):
        errors.append(
            f"'include_components' must be a list,"
            f" got {type(value).__name__}"
        )
        return
    for i, entry in enumerate(value):
        if not isinstance(entry, dict):
            errors.append(
                f"'include_components[{i}]' must be a dict,"
                f" got {type(entry).__name__}"
            )
            continue
        for req in ("key", "repo_org", "repo_name", "type"):
            if req not in entry:
                errors.append(
                    f"'include_components[{i}]':"
                    f" missing required '{req}'"
                )
            elif not isinstance(entry[req], str):
                errors.append(
                    f"'include_components[{i}].{req}'"
                    " must be a string"
                )
        if "integration_status" in entry:
            _check_integration_status(
                entry["integration_status"],
                f"include_components[{i}].integration_status",
                errors,
            )


def _check_post_checkout(value, errors):
    if not isinstance(value, list):
        errors.append(
            f"'post_checkout' must be a list,"
            f" got {type(value).__name__}"
        )
        return
    for i, entry in enumerate(value):
        if not isinstance(entry, dict):
            errors.append(
                f"'post_checkout[{i}]' must be a dict,"
                f" got {type(entry).__name__}"
            )
            continue
        if "repo" not in entry:
            errors.append(
                f"'post_checkout[{i}]': missing required 'repo'"
            )
        elif not isinstance(entry["repo"], str):
            errors.append(
                f"'post_checkout[{i}].repo' must be a string"
            )
        if "exclude_files" not in entry:
            errors.append(
                f"'post_checkout[{i}]':"
                " missing required 'exclude_files'"
            )
        else:
            _check_exclude_patterns(
                entry["exclude_files"],
                f"post_checkout[{i}].exclude_files",
                errors,
            )


def _check_repo_branch_policies(value, errors):
    if not isinstance(value, list):
        errors.append(
            "'repo_branch_policies' must be a list,"
            f" got {type(value).__name__}"
        )
        return

    seen_orgs = set()
    for i, entry in enumerate(value):
        label = f"repo_branch_policies[{i}]"
        if not isinstance(entry, dict):
            errors.append(
                f"'{label}' must be a mapping, got {type(entry).__name__}"
            )
            continue
        unknown = set(entry) - {"org", "require_branch_regex"}
        if unknown:
            errors.append(
                f"'{label}' has unrecognized keys: "
                f"{', '.join(sorted(unknown))}"
            )
        for key in ("org", "require_branch_regex"):
            field = entry.get(key)
            if not isinstance(field, str) or not field.strip():
                errors.append(f"'{label}.{key}' must be a non-empty string")
        org = entry.get("org")
        if isinstance(org, str) and org.strip():
            if org in seen_orgs:
                errors.append(
                    f"'{label}.org' duplicates repo branch policy for {org!r}"
                )
            seen_orgs.add(org)
        expression = entry.get("require_branch_regex")
        if isinstance(expression, str) and expression.strip():
            try:
                re.compile(expression)
            except re.error as exc:
                errors.append(
                    f"'{label}.require_branch_regex' is invalid: {exc}"
                )


def _check_sync_config(value, errors):
    if not isinstance(value, dict):
        errors.append(
            f"'sync_config' must be a dict,"
            f" got {type(value).__name__}"
        )
        return
    allowed = {"org", "repo", "branch", "protocol", "upstream_map"}
    unknown = set(value.keys()) - allowed
    if unknown:
        errors.append(
            f"'sync_config': unrecognized keys:"
            f" {', '.join(sorted(unknown))}"
        )
    for req in ("org", "repo", "upstream_map"):
        if req not in value:
            errors.append(f"'sync_config': missing required '{req}'")
        elif not isinstance(value[req], str):
            errors.append(f"'sync_config.{req}' must be a string")
        elif not value[req].strip():
            errors.append(
                f"'sync_config.{req}' must not be empty"
            )
    for opt in ("branch",):
        if opt in value and not isinstance(value[opt], str):
            errors.append(f"'sync_config.{opt}' must be a string")
    if "protocol" in value:
        proto = value["protocol"]
        if proto not in ("https", "ssh"):
            errors.append(
                f"'sync_config.protocol' must be"
                f" 'https' or 'ssh', got '{proto}'"
            )
    um = value.get("upstream_map", "")
    if isinstance(um, str):
        if ".." in um:
            errors.append(
                "'sync_config.upstream_map' contains '..'"
                " (path traversal)"
            )
        elif um.startswith("/"):
            errors.append(
                "'sync_config.upstream_map' is an absolute path"
            )


def validate_platform(name: str, config: dict) -> list[str]:
    errors: list[str] = []

    if not isinstance(config, dict):
        errors.append("platform value must be a mapping")
        return errors

    unknown = set(config.keys()) - KNOWN_KEYS
    if unknown:
        errors.append(
            f"unrecognized keys: {', '.join(sorted(unknown))}"
        )

    for key in ("suffix", "branch", "version", "reuse_from"):
        if key in config:
            _check_str(config[key], key, errors)

    if config.get("reuse_from") == name:
        errors.append("'reuse_from' must not reference the same platform")
    if isinstance(config.get("reuse_from"), str) and not config["reuse_from"].strip():
        errors.append("'reuse_from' must be a non-empty platform name")

    for key in ("orgs", "exclude_repos", "exclude_components"):
        if key in config:
            _check_list_of_str(config[key], key, errors)

    if "extra_orgs" in config:
        _check_extra_orgs(config["extra_orgs"], errors)

    if "extra_repos" in config:
        _check_extra_repos(config["extra_repos"], errors)

    if "include_components" in config:
        _check_include_components(config["include_components"], errors)

    if "integration_status" in config:
        _check_integration_status_map(config["integration_status"], errors)

    if "component_overrides" in config:
        co = config["component_overrides"]
        if not isinstance(co, dict):
            errors.append(
                "'component_overrides' must be a mapping,"
                f" got {type(co).__name__}"
            )
        else:
            for k, v in co.items():
                if not isinstance(v, dict):
                    errors.append(
                        f"'component_overrides.{k}'"
                        " must be a mapping"
                    )
                elif "integration_status" in v:
                    _check_integration_status(
                        v["integration_status"],
                        f"component_overrides.{k}.integration_status",
                        errors,
                    )

    if "post_checkout" in config:
        _check_post_checkout(config["post_checkout"], errors)

    if "repo_branch_policies" in config:
        _check_repo_branch_policies(config["repo_branch_policies"], errors)
        configured_orgs = set()
        orgs = config.get("orgs", [])
        if isinstance(orgs, list):
            configured_orgs.update(org for org in orgs if isinstance(org, str))
        extra_orgs = config.get("extra_orgs", [])
        if isinstance(extra_orgs, list):
            for entry in extra_orgs:
                if isinstance(entry, str):
                    configured_orgs.add(entry)
                elif isinstance(entry, dict) and isinstance(
                    entry.get("org"), str
                ):
                    configured_orgs.add(entry["org"])
        extra_repos = config.get("extra_repos", [])
        if isinstance(extra_repos, list):
            configured_orgs.update(
                entry["org"]
                for entry in extra_repos
                if isinstance(entry, dict)
                and isinstance(entry.get("org"), str)
            )
        sync_config = config.get("sync_config")
        if isinstance(sync_config, dict) and isinstance(
            sync_config.get("org"), str
        ):
            configured_orgs.add(sync_config["org"])
        branch_policies = config.get("repo_branch_policies", [])
        if not isinstance(branch_policies, list):
            branch_policies = []
        for i, entry in enumerate(branch_policies):
            if (
                isinstance(entry, dict)
                and isinstance(entry.get("org"), str)
                and entry["org"] not in configured_orgs
            ):
                errors.append(
                    f"'repo_branch_policies[{i}].org' is not configured "
                    "under orgs, extra_orgs, extra_repos, or sync_config"
                )

    if "sync_config" in config:
        _check_sync_config(config["sync_config"], errors)

    return errors


def validate_reuse_graph(configurations: dict) -> list[str]:
    """Validate explicit predecessor references without changing live config."""

    errors: list[str] = []
    platforms = {
        name: config
        for name, config in configurations.items()
        if isinstance(name, str) and not name.startswith("_")
    }
    for name, config in platforms.items():
        if not isinstance(config, dict):
            continue
        predecessor = config.get("reuse_from")
        if predecessor is None or not isinstance(predecessor, str):
            continue
        predecessor = predecessor.strip()
        if not predecessor:
            continue
        if predecessor not in platforms:
            errors.append(
                f"{name}: 'reuse_from' references missing platform {predecessor!r}"
            )

    visited: set[str] = set()
    for start in sorted(platforms):
        if start in visited:
            continue
        chain: list[str] = []
        current = start
        while current in platforms and current not in visited:
            if current in chain:
                cycle = chain[chain.index(current):] + [current]
                errors.append("reuse_from cycle detected: " + " -> ".join(cycle))
                break
            chain.append(current)
            config = platforms[current]
            if not isinstance(config, dict):
                break
            predecessor = config.get("reuse_from")
            if not isinstance(predecessor, str) or not predecessor.strip():
                break
            current = predecessor.strip()
        visited.update(chain)
    return errors


def main() -> int:
    if not PLATFORMS_FILE.exists():
        print(f"platforms.yaml not found: {PLATFORMS_FILE}")
        return 1

    try:
        with open(PLATFORMS_FILE, encoding="utf-8") as f:
            data = yaml.safe_load(f)
    except yaml.YAMLError as e:
        print(f"invalid YAML in {PLATFORMS_FILE}: {e}")
        return 1

    if not isinstance(data, dict):
        print("platforms.yaml root must be a YAML mapping")
        return 1

    total_errors = 0
    platforms = sorted(
        k for k in data if not k.startswith("_")
    )

    for name in platforms:
        errors = validate_platform(name, data[name])
        if errors:
            print(f"{name}:")
            for e in errors:
                print(f"  - {e}")
            total_errors += len(errors)

    graph_errors = validate_reuse_graph(data)
    for error in graph_errors:
        print(f"reuse graph: {error}")
    total_errors += len(graph_errors)

    if total_errors:
        print(
            f"\n{total_errors} error(s) in"
            f" {len(platforms)} platform(s)"
        )
        return 1

    print(
        f"All {len(platforms)} platform(s) passed validation."
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
