"""Phase 2b: Discover components via breadcrumb exploration."""

import copy
import fnmatch
import json
import os
import urllib.parse
from datetime import datetime
from pathlib import Path

from git import Repo
from github import Auth as GithubAuth
from github import Github

from lib.agent_runner import (
    run_agents_concurrently,
)
from lib.fetch import load_platform_config


async def _classify_checkouts(
    args,
    checkouts,
    architecture_dir,
    phase="PHASE - checkout classification"
):

    # cache this data ...
    cachedir = Path(".cache") / args.platform
    cachefile = cachedir / "checkouts_classifications.json"
    cachedir.mkdir(parents=True, exist_ok=True)
    requested_checkouts = {str(Path(path).resolve()) for path in checkouts}

    if cachefile.exists():
        try:
            with open(cachefile, "r") as f:
                results = json.loads(f.read())
            if requested_checkouts.issubset(results):
                return {
                    checkout: results[checkout]
                    for checkout in sorted(requested_checkouts)
                }
        except Exception as e:
            print(e)

    # run a classifier on a list of checkouts
    jobs = []
    output_folder = Path(architecture_dir) / args.platform / ".discovery/classification"
    output_folder.mkdir(parents=True, exist_ok=True)
    output_files = {}
    for checkout in sorted(checkouts):
        checkout_name = f"{checkout.parent.name}__{checkout.name}"
        output_file = output_folder / (checkout_name + '.json')
        output_files[checkout] = output_file
        jobs.append({
            "name": checkout_name,
            "cwd": ".",
            "prompt": f"/classify-checkout {str(checkout)} --output={output_file}",
        })

    # Create logs directory
    log_dir = Path(getattr(args, "log_dir", "logs/classify-checkout"))
    log_dir.mkdir(parents=True, exist_ok=True)
    print(f"Logs will be written to: {log_dir}\n")

    harness = getattr(args, "harness", "claude")
    results = await run_agents_concurrently(
        jobs,
        log_dir,
        args.model,
        args.max_concurrent,
        enable_skills=True,
        phase_label=phase,
         harness=harness,
    )

    missing = [
        checkout.name for checkout, output_file in output_files.items()
        if not output_file.exists()
    ]
    if missing:
        raise FileNotFoundError(
            f"Classification failed for {len(missing)} component(s): "
            + ", ".join(sorted(missing))
        )

    classifications = {}
    for output_file in output_files.values():
        with open(output_file, "r") as f:
            ds = json.loads(f.read())
            classifications[ds["checkout"]] = copy.deepcopy(ds)
            classifications[ds["checkout"]].pop("reasoning")

    # save the data
    with open(cachefile, "w") as f:
        f.write(json.dumps(classifications))

    return classifications


async def _get_provenance(args, checkouts_dirs):
    """Assemble the repository lineage for all checkouts"""

    # cache this data ...
    cachedir = Path(".cache") / args.platform
    cachefile = cachedir / "provenance.json"
    cachedir.mkdir(parents=True, exist_ok=True)
    requested_checkouts = {
        str(Path(checkout_dir).resolve()) for checkout_dir in checkouts_dirs
    }

    if cachefile.exists():
        try:
            with open(cachefile, "r") as f:
                provenance = json.loads(f.read())
            cached_checkouts = {
                str(Path(item["checkout"]).resolve())
                for item in provenance.values()
                if isinstance(item, dict) and isinstance(item.get("checkout"), str)
            }
            if requested_checkouts.issubset(cached_checkouts):
                return {
                    fullname: item
                    for fullname, item in provenance.items()
                    if str(Path(item["checkout"]).resolve())
                    in requested_checkouts
                }
        except Exception as e:
            print(e)

    provenance = {}

    # map out details for each checkout
    for checkout_dir in checkouts_dirs:
        codename = Path(checkout_dir).name
        try:
            repo = Repo(checkout_dir)
        except Exception as exc:
            detail = str(exc).strip() or "no exception message"
            raise RuntimeError(
                f"Cannot open checkout '{checkout_dir}' as a Git repository "
                f"({type(exc).__name__}: {detail}). Check the fetch log for "
                "a failed or incomplete clone."
            ) from exc
        #repo_url = repo.remote().url.rstrip("/.git").rstrip(".git")
        repo_url = repo.remote().url.rstrip("/").removesuffix(".git")
        if "git@" in repo_url:
            repo_url = repo_url.replace(":", "/", 1)
            repo_url = repo_url.replace("git@", "https://")
        repo_ps = urllib.parse.urlsplit(repo_url)
        repo_name = Path(repo_url).name
        repo_org = repo_ps.path.removesuffix(repo_name).strip("/")
        repo_fullname = repo_org + "/" + repo_name

        provenance[repo_fullname] = {
            "codename": codename,
            "checkout": str(checkout_dir),
            "repo_url": repo_url,
            "repo": repo_name,
            "org": repo_org,
            "repo_fullname": repo_org + "/" + repo_name,
            "lineage": [],
        }

    # Resolve fork ancestry when credentials are available. Fetch historically
    # uses GITHUB_TOKEN, while older discovery setups may use GH_TOKEN.
    gh_token = os.environ.get("GH_TOKEN") or os.environ.get("GITHUB_TOKEN")
    if not gh_token:
        print(
            "WARNING: GH_TOKEN and GITHUB_TOKEN are unset; skipping GitHub "
            "parent-lineage lookups. Repository lineage will remain empty."
        )
    else:
        g = Github(auth=GithubAuth.Token(gh_token))

        # Recurse through parent relationships in the GitHub API data.
        for pkey, pdata in provenance.items():
            print(f"get lineage for {pdata['repo_fullname']}")
            lineage = [pdata["repo_fullname"]]
            prepo = g.get_repo(pdata["repo_fullname"])
            parent = prepo.parent
            while True:
                if not parent or not parent.full_name:
                    break
                lineage.append(parent.full_name)
                print(f"\t{lineage}")
                parent = parent.parent
            provenance[pkey]["lineage"] = lineage[::-1]

    with open(cachefile, "w") as f:
        f.write(json.dumps(provenance))

    return provenance


async def _assemble_component_map(args, classifications, provenance):
    """Combine data into the component map format."""
    cm = {
        "meta": {
            "platform": args.platform,
            "discovered_at": datetime.now().isoformat()
        },
        "components": {}
    }

    missing = sorted(
        rdata["checkout"]
        for rdata in provenance.values()
        if rdata["checkout"] not in classifications
    )
    if missing:
        raise ValueError(
            f"No classification found for {len(missing)} checkout(s): "
            + ", ".join(missing)
        )

    for full_name, rdata in provenance.items():
        codename = rdata["codename"]
        cm["components"][codename] = {
            "key": codename,
            "repo_org": rdata["org"],
            "repo_name": rdata["repo"],
            "checkout_path": rdata["checkout"],
            "checkout_branch": None,
            "ref": None,
            "has_architecture": False,
            "discovered_via": None,
            "referenced_by": None,
            "tier": None,
            "type": classifications[rdata["checkout"]]["classification"],
            "architecturally_signficant": True,
            "confidence": "high",
            "lineage": rdata["lineage"][:]
        }

    return cm


def _load_branch_policy_exclusions(
    checkouts_dirs: list[Path], platform: str, policies: list[dict]
) -> set[Path]:
    """Read fetch-stage branch decisions and return ineligible checkout paths."""
    if not policies:
        return set()

    expected = {}
    for index, entry in enumerate(policies):
        if (
            not isinstance(entry, dict)
            or not isinstance(entry.get("org"), str)
            or not isinstance(entry.get("require_branch_regex"), str)
        ):
            raise RuntimeError(
                f"Malformed repo_branch_policies[{index}] for {platform}"
            )
        expected[entry["org"]] = entry["require_branch_regex"]
    found = set()
    excluded = set()
    for checkout_dir in checkouts_dirs:
        report_path = checkout_dir / "repo-branch-policy.json"
        if not report_path.is_file():
            continue
        try:
            report = json.loads(report_path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError) as exc:
            raise RuntimeError(
                f"Cannot read branch policy report {report_path}: {exc}"
            ) from exc
        if not isinstance(report, dict) or report.get("platform") != platform:
            raise RuntimeError(
                f"Branch policy report has the wrong platform: {report_path}"
            )
        org = report.get("org")
        if org not in expected:
            raise RuntimeError(
                f"Unexpected org in branch policy report {report_path}: {org!r}"
            )
        if report.get("require_branch_regex") != expected[org]:
            raise RuntimeError(
                f"Branch policy report for {org} uses a stale regex: "
                f"{report_path}"
            )
        if org in found:
            raise RuntimeError(
                f"Multiple branch policy reports found for {org!r}"
            )
        found.add(org)
        repositories = report.get("repositories")
        if not isinstance(repositories, list):
            raise RuntimeError(
                f"Branch policy report has no repository list: {report_path}"
            )
        reported_repositories = set()
        for item in repositories:
            if not isinstance(item, dict):
                raise RuntimeError(
                    f"Malformed repository result in {report_path}"
                )
            name = item.get("repo")
            eligible = item.get("eligible")
            if (
                not isinstance(name, str)
                or not name
                or Path(name).name != name
                or name in {".", ".."}
                or not isinstance(eligible, bool)
            ):
                raise RuntimeError(
                    f"Malformed repository result in {report_path}: {item!r}"
                )
            matches = item.get("matching_branches")
            expected_reason = (
                "matched_required_branch_regex"
                if eligible else "no_remote_branch_matched_regex"
            )
            if (
                not isinstance(matches, list)
                or any(not isinstance(branch, str) for branch in matches)
                or bool(matches) != eligible
                or item.get("reason") != expected_reason
                or name in reported_repositories
            ):
                raise RuntimeError(
                    f"Inconsistent repository result in {report_path}: {item!r}"
                )
            reported_repositories.add(name)
            if not eligible:
                excluded.add((checkout_dir / name).resolve())

        current_repositories = {
            path.name for path in checkout_dir.iterdir() if path.is_dir()
        }
        if current_repositories != reported_repositories:
            raise RuntimeError(
                f"Branch policy report is stale or incomplete: {report_path}"
            )

    missing = set(expected) - found
    if missing:
        raise RuntimeError(
            "Missing fetch-stage branch policy report(s) for: "
            + ", ".join(sorted(missing))
        )
    return excluded


async def run_discover_components_phase(args) -> None:
    """Run Phase 2b: Discover components via breadcrumb exploration."""
    print("\n" + "=" * 60)
    print("PHASE 2B: Discovering platform components")
    print("=" * 60 + "\n")

    architecture_dir = str(
        Path(getattr(args, 'architecture_dir', 'architecture')).resolve()
    )
    map_file = Path(architecture_dir) / args.platform / "component-map.json"
    force = getattr(args, 'force', False)

    try:
        platform_policy_config = load_platform_config(args.platform)
    except (FileNotFoundError, KeyError) as e:
        print(f"Cannot load platform config for discovery: {e}")
        return

    if (
        map_file.exists()
        and not force
        and not platform_policy_config.get("repo_branch_policies")
    ):
        print(f"Component map already exists: {map_file}")
        print("Skipping discovery (use --force to re-run)\n")
        print("=" * 60)
        return

    platform_config = None
    checkouts_dirs = []

    if getattr(args, 'checkouts_dir', None):
        checkouts_dirs = [args.checkouts_dir]
    else:
        try:
            platform_config = platform_policy_config
            suffix = platform_config.get("suffix", "head")

            for org in platform_config.get("orgs", []):
                checkouts_dirs.append(f"checkouts/{org}.{suffix}")

            for entry in platform_config.get("extra_orgs", []):
                org_name = entry.get("org") if isinstance(entry, dict) else entry
                org_suffix = (
                    entry.get("suffix")
                    if isinstance(entry, dict)
                    else None
                ) or suffix
                checkouts_dirs.append(f"checkouts/{org_name}.{org_suffix}")

            for entry in platform_config.get("extra_repos", []):
                repo_suffix = entry.get("suffix") or suffix
                org_dir = f"checkouts/{entry['org']}.{repo_suffix}"
                if org_dir not in checkouts_dirs:
                    checkouts_dirs.append(org_dir)

            if checkouts_dirs:
                print("Resolved checkout directories from platforms.yaml:")
                for d in checkouts_dirs:
                    print(f"  - {d}")
            else:
                print(
                    f"Error: no orgs defined for platform"
                    f" '{args.platform}' in platforms.yaml"
                )
                return
        except (FileNotFoundError, KeyError) as e:
            print(
                f"Error: --checkouts-dir is required"
                f" (could not resolve from platforms.yaml: {e})"
            )
            return

    print(f"Platform: {args.platform}")
    if getattr(args, 'entry_repo', None):
        print(f"Entry point: {args.entry_repo}")
    print()

    checkouts_dirs = [Path(d).resolve() for d in checkouts_dirs]

    # make a list of checkouts
    full_checkouts = set()
    for checkout_dir in checkouts_dirs:
        for root,dirs,files in checkout_dir.walk():
            if root != checkout_dir:
                continue
            for subdir in dirs:
                full_path = root / subdir
                full_checkouts.add(full_path)

    branch_policies = platform_policy_config.get("repo_branch_policies", [])
    branch_exclusions = _load_branch_policy_exclusions(
        checkouts_dirs, args.platform, branch_policies
    )
    if branch_exclusions:
        before = len(full_checkouts)
        full_checkouts.difference_update(branch_exclusions)
        print(
            "Filtered {} checkout(s) by remote branch policy before "
            "classification.".format(before - len(full_checkouts))
        )

    # remove checkouts that are statically excluded
    if platform_config:
        if config_excludes := platform_config.get("exclude_repos", []):
            for pattern in config_excludes:
                full_checkouts = [
                    x for x in full_checkouts
                    if not fnmatch.fnmatchcase(str(x), pattern)
                ]

    provenance = await _get_provenance(args, full_checkouts)
    classifications = await _classify_checkouts(args, full_checkouts, architecture_dir)
    component_map = await _assemble_component_map(args, classifications, provenance)
    platform_dir = Path(architecture_dir) / args.platform
    platform_dir.mkdir(parents=True, exist_ok=True)

    # do a final pass to remove excluded components
    if args.platform:
        platform_config = platform_policy_config
        for exclude_component in platform_config['exclude_components']:
            if exclude_component in component_map['components']:
                component_map['components'].pop(exclude_component)
        for exclude_repo in platform_config['exclude_repos']:
            keys = list(component_map['components'].keys())
            for key in keys:
                if component_map['components'][key]['repo_name'] == exclude_repo:
                    component_map['components'].pop(key)

    map_file = platform_dir / "component-map.json"
    with open(map_file, "w") as f:
        f.write(json.dumps(component_map))
