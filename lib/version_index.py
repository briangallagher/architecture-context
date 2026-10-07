"""Deterministic navigation indexes for versioned architecture directories."""

from __future__ import annotations

import json
import os
import re
import tempfile
from dataclasses import dataclass
from pathlib import Path
from typing import Any
from urllib.parse import quote

import yaml

from lib.structured_component_publication import (
    accepted_publications,
    has_publication_state,
)
from lib.structured_component_synthesis import GoStructuredAssembler

INDEX_FILENAME = "INDEX.md"
RESERVED_DOCUMENTS = frozenset({INDEX_FILENAME, "PLATFORM.md", "README.md"})
INTEGRATION_STATUSES = frozenset({"current", "planned", "not-integrated", "unknown"})
TOPIC_KEYWORDS = {
    "Authentication": (
        "authentication",
        "authorization",
        "oauth",
        "oidc",
        "rbac",
        "security",
    ),
    "Serving": ("serving", "inference"),
    "Training": ("training", "trainer", "fine tuning", "fine-tuning"),
    "Pipelines": ("pipeline", "workflow"),
    "Lifecycle": (
        "lifecycle",
        "configuration",
        "deployment",
        "installation",
        "upgrade",
    ),
}
FENCE_RE = re.compile(r"^\s*(```+|~~~+)")
HEADING_RE = re.compile(r"^(#{1,6})\s+(.+?)\s*$")
MARKDOWN_LINK_RE = re.compile(r"\[([^]]+)]\([^)]*\)")
COMMIT_IN_REF_RE = re.compile(r"@([0-9a-fA-F]{7,40})$")


class VersionIndexError(ValueError):
    """Raised when index inputs cannot be rendered safely."""


@dataclass(frozen=True)
class IndexGenerationResult:
    """Result of one atomic version-index write."""

    path: Path
    changed: bool
    component_count: int


def _safe_alias(value: object) -> str:
    if not isinstance(value, str) or not value.strip():
        raise VersionIndexError("component aliases must be non-empty strings")
    alias = value.strip()
    if alias in {".", ".."} or "/" in alias or "\\" in alias or "\0" in alias:
        raise VersionIndexError(f"unsafe component alias: {alias!r}")
    return alias


def _component_entries(
    component_map: dict[str, Any],
) -> list[tuple[str, dict[str, Any]]]:
    raw = component_map.get("components")
    if isinstance(raw, dict):
        entries = list(raw.items())
    elif isinstance(raw, list):
        entries = []
        for position, value in enumerate(raw):
            if not isinstance(value, dict):
                raise VersionIndexError(
                    f"component-map component {position} must be an object"
                )
            alias = value.get("key") or value.get("repo_name")
            entries.append((alias, value))
    else:
        raise VersionIndexError("component-map components must be an object or list")

    result: list[tuple[str, dict[str, Any]]] = []
    seen: set[str] = set()
    for raw_alias, value in entries:
        alias = _safe_alias(raw_alias)
        if not isinstance(value, dict):
            raise VersionIndexError(f"component {alias!r} must be an object")
        filename = f"{alias}.md"
        if filename in RESERVED_DOCUMENTS:
            continue
        if alias in seen:
            raise VersionIndexError(f"duplicate component alias: {alias}")
        seen.add(alias)
        result.append((alias, value))
    return sorted(result, key=lambda item: (item[0].casefold(), item[0]))


def _load_json_object(path: Path, label: str) -> dict[str, Any]:
    try:
        payload = json.loads(path.read_text())
    except OSError as error:
        raise VersionIndexError(f"cannot read {label} {path}: {error}") from error
    except json.JSONDecodeError as error:
        raise VersionIndexError(f"invalid JSON in {label} {path}: {error}") from error
    if not isinstance(payload, dict):
        raise VersionIndexError(f"{label} root must be an object: {path}")
    return payload


def load_platform_configuration(path: Path, platform: str) -> dict[str, Any]:
    """Load one platform entry; unconfigured legacy versions get an empty map."""

    if not path.is_file():
        return {}
    try:
        payload = yaml.safe_load(path.read_text())
    except (OSError, yaml.YAMLError) as error:
        raise VersionIndexError(f"cannot parse platform configuration {path}: {error}")
    if payload is None:
        return {}
    if not isinstance(payload, dict):
        raise VersionIndexError("platform configuration root must be an object")
    value = payload.get(platform, {})
    if value is None:
        return {}
    if not isinstance(value, dict):
        raise VersionIndexError(
            f"platform configuration for {platform} must be an object"
        )
    return value


def _release_labels(platform: str) -> set[str]:
    labels = {platform}
    version = platform
    for prefix in ("rhoai-", "odh-"):
        if platform.startswith(prefix):
            version = platform[len(prefix) :]
            break
    labels.add(version)
    match = re.match(r"^(\d+\.\d+)", version)
    if match:
        labels.add(match.group(1))
    if version == "next" or platform.endswith(".next"):
        labels.add("next")
    return labels


def _frontmatter(path: Path) -> dict[str, Any]:
    try:
        text = path.read_text()
    except OSError as error:
        raise VersionIndexError(f"cannot read overlay {path}: {error}") from error
    lines = text.splitlines()
    if not lines or lines[0].strip() != "---":
        raise VersionIndexError(f"overlay lacks YAML frontmatter: {path}")
    try:
        end = next(
            i for i, line in enumerate(lines[1:], start=1) if line.strip() == "---"
        )
    except StopIteration as error:
        raise VersionIndexError(
            f"overlay frontmatter is not terminated: {path}"
        ) from error
    try:
        payload = yaml.safe_load("\n".join(lines[1:end]))
    except yaml.YAMLError as error:
        raise VersionIndexError(
            f"invalid overlay frontmatter {path}: {error}"
        ) from error
    if not isinstance(payload, dict):
        raise VersionIndexError(f"overlay frontmatter must be an object: {path}")
    return payload


def _integration_status(value: object, source: str) -> str | None:
    if value is None:
        return None
    status = str(value).strip().lower()
    if status not in INTEGRATION_STATUSES:
        expected = ", ".join(sorted(INTEGRATION_STATUSES))
        raise VersionIndexError(
            f"invalid integration_status {value!r} in {source}; expected {expected}"
        )
    return status


def load_active_overlays(overlays_dir: Path, platform: str) -> list[dict[str, Any]]:
    """Load active, release-applicable overlay metadata without reading prose."""

    if not overlays_dir.is_dir():
        return []
    labels = _release_labels(platform)
    result: list[dict[str, Any]] = []
    for path in sorted(overlays_dir.glob("*.md")):
        if path.name == "README.md":
            continue
        data = _frontmatter(path)
        if data.get("status") != "active":
            continue
        releases = data.get("release")
        affects = data.get("affects")
        if not isinstance(releases, list) or not all(
            isinstance(item, (str, int, float)) for item in releases
        ):
            raise VersionIndexError(f"overlay release must be a list: {path}")
        if not isinstance(affects, list) or not all(
            isinstance(item, str) and item.strip() for item in affects
        ):
            raise VersionIndexError(f"overlay affects must be a string list: {path}")
        release_values = {str(item).strip() for item in releases}
        if "all" not in release_values and release_values.isdisjoint(labels):
            continue
        title = data.get("title")
        if not isinstance(title, str) or not title.strip():
            raise VersionIndexError(f"overlay title must be a string: {path}")
        result.append(
            {
                "path": path,
                "title": title.strip(),
                "affects": sorted({item.strip() for item in affects}),
                "integration_status": _integration_status(
                    data.get("integration_status"), str(path)
                ),
            }
        )
    return result


def _parse_document(text: str) -> tuple[list[str], str | None]:
    headings: list[str] = []
    lines = text.splitlines()
    in_fence = False
    purpose_start: int | None = None
    for position, line in enumerate(lines):
        if FENCE_RE.match(line):
            in_fence = not in_fence
            continue
        if in_fence:
            continue
        match = HEADING_RE.match(line)
        if not match:
            continue
        level = len(match.group(1))
        heading = match.group(2).strip().rstrip("#").strip()
        if level in {2, 3}:
            headings.append(heading)
        if level == 2 and heading.casefold() == "purpose":
            purpose_start = position + 1

    purpose: str | None = None
    if purpose_start is not None:
        paragraph: list[str] = []
        in_fence = False
        for line in lines[purpose_start:]:
            if FENCE_RE.match(line):
                in_fence = not in_fence
                continue
            if in_fence:
                continue
            if HEADING_RE.match(line):
                break
            stripped = line.strip()
            if not stripped:
                if paragraph:
                    break
                continue
            paragraph.append(stripped)
        if paragraph:
            purpose = " ".join(paragraph)
    return headings, purpose


def _plain_summary(value: object, limit: int = 180) -> str | None:
    if not isinstance(value, str):
        return None
    summary = MARKDOWN_LINK_RE.sub(r"\1", value)
    summary = re.sub(r"\s+", " ", summary).strip()
    if not summary:
        return None
    if len(summary) > limit:
        summary = summary[: limit - 1].rstrip() + "…"
    return summary


def _normalize_repository(value: object) -> str | None:
    if not isinstance(value, str) or not value.strip():
        return None
    return value.strip().rstrip("/").removesuffix(".git")


def _analyzer_metadata(
    analyzer_path: Path,
    alias: str,
    component: dict[str, Any],
) -> tuple[str, dict[str, Any] | None]:
    if not analyzer_path.is_file():
        return "unavailable", None
    try:
        payload = json.loads(analyzer_path.read_text())
    except (OSError, json.JSONDecodeError):
        return "invalid", None
    if not isinstance(payload, dict):
        return "invalid", None

    mismatches: list[str] = []
    analyzer_component = payload.get("component")
    if isinstance(analyzer_component, str) and analyzer_component != alias:
        mismatches.append("component")
    expected_repository = _normalize_repository(component.get("repo_url"))
    actual_repository = _normalize_repository(
        payload.get("repo") or payload.get("repository")
    )
    if (
        expected_repository
        and actual_repository
        and expected_repository != actual_repository
    ):
        mismatches.append("repository")
    ref = component.get("ref")
    commit = payload.get("commit_sha")
    if isinstance(ref, str) and isinstance(commit, str):
        match = COMMIT_IN_REF_RE.search(ref)
        if match and not commit.lower().startswith(match.group(1).lower()):
            mismatches.append("revision")
    state = "mismatch:" + ",".join(mismatches) if mismatches else "available"
    return state, payload


def _config_status(
    alias: str, platform_config: dict[str, Any]
) -> tuple[str | None, str | None]:
    explicit = platform_config.get("integration_status")
    if isinstance(explicit, dict) and alias in explicit:
        return _integration_status(
            explicit[alias], "platform integration_status"
        ), "platforms.yaml"
    overrides = platform_config.get("component_overrides")
    if isinstance(overrides, dict):
        value = overrides.get(alias)
        if isinstance(value, dict) and "integration_status" in value:
            return (
                _integration_status(
                    value["integration_status"], f"component_overrides.{alias}"
                ),
                "platforms.yaml",
            )
    includes = platform_config.get("include_components")
    if isinstance(includes, list):
        for value in includes:
            if not isinstance(value, dict) or value.get("key") != alias:
                continue
            if "integration_status" in value:
                return (
                    _integration_status(
                        value["integration_status"], f"include_components.{alias}"
                    ),
                    "platforms.yaml",
                )
    return None, None


def _validate_platform_index_config(platform_config: dict[str, Any]) -> None:
    explicit = platform_config.get("integration_status")
    if explicit is not None:
        if not isinstance(explicit, dict):
            raise VersionIndexError(
                "platform integration_status must be an alias-to-status object"
            )
        for alias, status in explicit.items():
            _safe_alias(alias)
            _integration_status(status, f"platform integration_status.{alias}")

    overrides = platform_config.get("component_overrides")
    if isinstance(overrides, dict):
        for alias, value in overrides.items():
            if not isinstance(value, dict) or "integration_status" not in value:
                continue
            _safe_alias(alias)
            _integration_status(
                value["integration_status"],
                f"component_overrides.{alias}",
            )

    includes = platform_config.get("include_components")
    if isinstance(includes, list):
        for position, value in enumerate(includes):
            if not isinstance(value, dict) or "integration_status" not in value:
                continue
            alias = value.get("key")
            _safe_alias(alias)
            _integration_status(
                value["integration_status"],
                f"include_components[{position}]",
            )


def _component_overlays(
    alias: str,
    component: dict[str, Any],
    overlays: list[dict[str, Any]],
) -> list[dict[str, Any]]:
    identities = {alias}
    repo_name = component.get("repo_name")
    if isinstance(repo_name, str) and repo_name:
        identities.add(repo_name)
    return [
        overlay
        for overlay in overlays
        if "platform" in overlay["affects"]
        or identities.intersection(overlay["affects"])
    ]


def _resolve_integration(
    alias: str,
    component: dict[str, Any],
    platform_config: dict[str, Any],
    overlays: list[dict[str, Any]],
) -> tuple[str, str]:
    component_overlays = _component_overlays(alias, component, overlays)
    overlay_statuses = {
        overlay["integration_status"]
        for overlay in component_overlays
        if overlay["integration_status"] is not None
    }
    if len(overlay_statuses) == 1:
        return next(iter(overlay_statuses)), "active overlay"
    if len(overlay_statuses) > 1:
        return "unknown", "conflicting active overlays"
    config_status, config_source = _config_status(alias, platform_config)
    if config_status is not None and config_source is not None:
        return config_status, config_source
    raw_status = _integration_status(
        component.get("integration_status"), f"component-map.{alias}"
    )
    if raw_status == "planned":
        raise VersionIndexError(
            f"planned integration for {alias!r} must come from platforms.yaml "
            "or an active release-applicable overlay"
        )
    if raw_status is not None:
        return raw_status, "component-map.json"
    return "unknown", "no explicit version-scoped integration evidence"


def _heading_anchor(heading: str) -> str:
    value = heading.strip().casefold()
    value = re.sub(r"[^\w\- ]", "", value)
    return re.sub(r"[ -]+", "-", value).strip("-")


def _topic_heading(headings: list[str], keywords: tuple[str, ...]) -> str | None:
    for heading in headings:
        normalized = re.sub(r"\s+", " ", heading.casefold().replace("_", " "))
        if any(keyword in normalized for keyword in keywords):
            return heading
    return None


def _relative_link(from_dir: Path, target: Path, fragment: str | None = None) -> str:
    relative = Path(os.path.relpath(target, start=from_dir))
    parts = [
        part if part == ".." else quote(part, safe=".-_") for part in relative.parts
    ]
    link = "/".join(parts)
    if fragment:
        link += "#" + quote(fragment, safe="-_")
    return link


def _escape(value: object) -> str:
    text = str(value)
    text = text.replace("\\", "\\\\")
    text = text.replace("|", "\\|")
    text = text.replace("[", "\\[").replace("]", "\\]")
    return re.sub(r"\s+", " ", text).strip()


def _link(label: str, target: str) -> str:
    return f"[{_escape(label)}]({target})"


def _coverage_status(
    platform_dir: Path,
    alias: str,
    analyzer_path: Path,
    analyzer: dict[str, Any] | None,
) -> str:
    sidecar = platform_dir / alias / ".generation" / "SURFACE_COVERAGE.json"
    values: list[str] = []
    if sidecar.is_file():
        try:
            payload = json.loads(sidecar.read_text())
        except (OSError, json.JSONDecodeError):
            values.append("coverage sidecar invalid")
        else:
            if isinstance(payload, dict):
                version = payload.get("schema_version")
                label = f"coverage {version}" if version else "coverage sidecar"
                values.append(_link(label, _relative_link(platform_dir, sidecar)))
            else:
                values.append("coverage sidecar invalid")
    else:
        values.append("coverage telemetry unavailable")
    if analyzer_path.is_file() and analyzer is not None:
        label = (
            "analyzer limitations"
            if isinstance(analyzer.get("data_coverage"), dict)
            else "analyzer metadata"
        )
        values.append(_link(label, _relative_link(platform_dir, analyzer_path)))
    elif analyzer_path.exists():
        values.append("analyzer metadata invalid")
    else:
        values.append("analyzer metadata unavailable")
    return "; ".join(values)


def build_index_model(
    platform_dir: Path,
    *,
    platform_config: dict[str, Any] | None = None,
    overlays: list[dict[str, Any]] | None = None,
    platforms_file: Path | None = None,
) -> dict[str, Any]:
    """Build a version-wide navigation model from bounded local artifacts."""

    if not platform_dir.is_dir():
        raise VersionIndexError(f"version directory does not exist: {platform_dir}")
    component_map_path = platform_dir / "component-map.json"
    if not component_map_path.is_file():
        raise VersionIndexError(
            f"required component map not found: {component_map_path}"
        )
    component_map = _load_json_object(component_map_path, "component map")
    platform_config = platform_config or {}
    _validate_platform_index_config(platform_config)
    overlays = overlays or []
    components: list[dict[str, Any]] = []
    topics: dict[str, list[dict[str, str]]] = {topic: [] for topic in TOPIC_KEYWORDS}

    for alias, raw in _component_entries(component_map):
        document_path = platform_dir / f"{alias}.md"
        document_available = document_path.is_file()
        headings: list[str] = []
        purpose: str | None = None
        if document_available:
            headings, purpose = _parse_document(document_path.read_text())
        analyzer_path = (
            platform_dir / alias / ".analyzer" / "component-architecture.json"
        )
        analyzer_state, analyzer = _analyzer_metadata(analyzer_path, alias, raw)
        description = _plain_summary(raw.get("description"))
        description_source = "component-map.json" if description else None
        if description is None:
            description = _plain_summary(purpose)
            description_source = "document Purpose" if description else None
        if description is None and analyzer_state == "available" and analyzer:
            for key in ("description", "purpose", "summary"):
                description = _plain_summary(analyzer.get(key))
                if description:
                    description_source = f"analyzer {key}"
                    break
        integration, integration_source = _resolve_integration(
            alias, raw, platform_config, overlays
        )
        relevant_overlays = _component_overlays(alias, raw, overlays)
        component = {
            "alias": alias,
            "description": description or "unavailable",
            "description_source": description_source or "unavailable",
            "type": str(raw.get("type") or "unavailable"),
            "document_path": document_path,
            "document_status": "available" if document_available else "pending",
            "inventory_signal": (
                "shipped=true"
                if raw.get("shipped") is True
                else "shipped=false"
                if raw.get("shipped") is False
                else "shipped=unavailable"
            ),
            "integration": integration,
            "integration_source": integration_source,
            "analyzer_state": analyzer_state,
            "coverage": _coverage_status(platform_dir, alias, analyzer_path, analyzer),
            "overlays": relevant_overlays,
        }
        components.append(component)
        if document_available:
            for topic, keywords in TOPIC_KEYWORDS.items():
                heading = _topic_heading(headings, keywords)
                if heading:
                    topics[topic].append(
                        {
                            "alias": alias,
                            "heading": heading,
                            "link": _relative_link(
                                platform_dir,
                                document_path,
                                _heading_anchor(heading),
                            ),
                        }
                    )

    platform_document = platform_dir / "PLATFORM.md"
    return {
        "platform": platform_dir.name,
        "platform_dir": platform_dir,
        "component_map_path": component_map_path,
        "platform_document": (
            platform_document if platform_document.is_file() else None
        ),
        "platforms_file": platforms_file,
        "components": components,
        "topics": topics,
        "overlays": overlays,
    }


def _overlay_link(platform_dir: Path, overlay: dict[str, Any]) -> str:
    return _link(overlay["title"], _relative_link(platform_dir, overlay["path"]))


def render_version_index(model: dict[str, Any]) -> str:
    """Render a stable Markdown navigation document."""

    platform_dir = model["platform_dir"]
    components = model["components"]
    available = sum(item["document_status"] == "available" for item in components)
    pending = len(components) - available
    lines = [
        f"# {_escape(model['platform'])} Architecture Index",
        "",
        (
            "This deterministic index points to existing architecture evidence. "
            "Topic entries are navigation hints and do not prove component behavior."
        ),
        "",
        "## Platform navigation",
        "",
        (
            "- Platform architecture: "
            + (
                _link("PLATFORM.md", "PLATFORM.md")
                if model["platform_document"] is not None
                else "unavailable"
            )
        ),
        "- Component inventory: " + _link("component-map.json", "component-map.json"),
        f"- Documentation: {available} available; {pending} pending.",
        (
            "- Integration status requires an explicit version-scoped "
            "`integration_status`. Inventory inclusion and the `shipped` field are "
            "shown separately and do not establish platform integration."
        ),
        "",
        "## Components",
        "",
        (
            "| Component | Description | Type | Documentation | Inventory signal | "
            "Integration | Analyzer metadata | Coverage / limitations | Context |"
        ),
        "|---|---|---|---|---|---|---|---|---|",
    ]
    for component in components:
        alias = component["alias"]
        if component["document_status"] == "available":
            documentation = _link(
                alias, _relative_link(platform_dir, component["document_path"])
            )
        else:
            documentation = "pending"
        integration = f"{component['integration']} ({component['integration_source']})"
        context = (
            ", ".join(
                _overlay_link(platform_dir, overlay)
                for overlay in component["overlays"]
            )
            or "none"
        )
        lines.append(
            "| "
            + " | ".join(
                [
                    _escape(alias),
                    _escape(component["description"]),
                    _escape(component["type"]),
                    documentation,
                    _escape(component["inventory_signal"]),
                    _escape(integration),
                    _escape(component["analyzer_state"]),
                    component["coverage"],
                    context,
                ]
            )
            + " |"
        )

    lines.extend(["", "## Topic navigation", ""])
    lines.append(
        "Links below come only from headings present in available component "
        "documents. They are navigation hints, not capability or relationship claims."
    )
    for topic, entries in model["topics"].items():
        lines.extend(["", f"### {topic}", ""])
        if entries:
            lines.extend(
                f"- {_link(entry['alias'] + ' — ' + entry['heading'], entry['link'])}"
                for entry in entries
            )
        else:
            lines.append("- No matching document heading is available.")

    lines.extend(["", "## Active overlay context", ""])
    if model["overlays"]:
        for overlay in model["overlays"]:
            affects = ", ".join(_escape(item) for item in overlay["affects"])
            integration = overlay["integration_status"]
            suffix = f"; explicit integration: `{integration}`" if integration else ""
            lines.append(
                f"- {_overlay_link(platform_dir, overlay)} — affects {affects}{suffix}."
            )
    else:
        lines.append("- No active release-applicable overlay metadata is available.")

    lines.extend(
        [
            "",
            "## Interpretation limits",
            "",
            (
                "- Missing coverage sidecars mean coverage telemetry is unavailable; "
                "they do not show that behavior is missing."
            ),
            (
                "- Pending documentation has no link. The index does not inspect "
                "source, fetch repositories, invoke an agent, or synthesize a "
                "replacement."
            ),
            (
                "- Planned integration appears only when structured platform or "
                "active overlay metadata explicitly records it. Free-form overlay "
                "prose is not interpreted."
            ),
            "",
        ]
    )
    return "\n".join(lines)


def _atomic_write_if_changed(path: Path, content: str) -> bool:
    encoded = content.encode()
    if path.is_file() and path.read_bytes() == encoded:
        return False
    temporary: Path | None = None
    try:
        with tempfile.NamedTemporaryFile(
            mode="wb",
            prefix=f".{path.name}.",
            suffix=".tmp",
            dir=path.parent,
            delete=False,
        ) as stream:
            temporary = Path(stream.name)
            stream.write(encoded)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    finally:
        if temporary is not None and temporary.exists():
            temporary.unlink()
    return True


def generate_version_index(
    platform_dir: Path,
    *,
    platforms_file: Path = Path("platforms.yaml"),
    overlays_dir: Path = Path("overlays"),
    assembler: GoStructuredAssembler | None = None,
) -> IndexGenerationResult:
    """Validate local inputs and atomically create ``INDEX.md``."""

    if not platform_dir.is_dir():
        raise VersionIndexError(f"version directory does not exist: {platform_dir}")
    has_structured = has_publication_state(platform_dir)
    if has_structured:
        if assembler is None:
            raise VersionIndexError(
                "accepted snapshot validation requires the arch-analyzer renderer"
            )
        accepted_publications(platform_dir, assembler, repair_markdown=True)
    platform_config = load_platform_configuration(platforms_file, platform_dir.name)
    overlays = load_active_overlays(overlays_dir, platform_dir.name)
    model = build_index_model(
        platform_dir,
        platform_config=platform_config,
        overlays=overlays,
        platforms_file=platforms_file,
    )
    output = platform_dir / INDEX_FILENAME
    changed = _atomic_write_if_changed(output, render_version_index(model))
    return IndexGenerationResult(
        path=output,
        changed=changed,
        component_count=len(model["components"]),
    )
