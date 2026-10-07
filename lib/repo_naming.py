"""Shared naming helpers for repository checkout aliases."""


def checkout_name(repo: str, name_prefix: str = "") -> str:
    """Return the local checkout name while keeping the repository name intact."""
    if not name_prefix or repo.startswith(name_prefix):
        return repo
    return f"{name_prefix}{repo}"


def extra_repo_checkout_name(platform_config: dict, org: str, repo: str) -> str:
    """Resolve a configured extra repository's local checkout name."""
    for entry in platform_config.get("extra_repos", []):
        if entry.get("org") == org and entry.get("repo") == repo:
            return checkout_name(repo, entry.get("name_prefix", ""))
    return repo


def extra_repo_aliases(platform_config: dict) -> dict[tuple[str, str], str]:
    """Return canonical ``(org, repo)`` identities mapped to local aliases."""
    aliases = {}
    for entry in platform_config.get("extra_repos", []):
        prefix = entry.get("name_prefix", "")
        if prefix:
            aliases[(entry["org"], entry["repo"])] = checkout_name(
                entry["repo"], prefix,
            )
    return aliases
