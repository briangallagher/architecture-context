#!/usr/bin/env python3
"""Fetch CodeRabbit review comments from a GitHub pull request.

Usage:
    uv run python scripts/fetch_coderabbit_comments.py <PR-URL>
    uv run python scripts/fetch_coderabbit_comments.py <owner/repo> <number>
    uv run python scripts/fetch_coderabbit_comments.py <owner/repo> <number> --json

Requires GH_TOKEN in .env or environment.
"""

import argparse
import json
import os
import re
import sys
import urllib.request
from pathlib import Path


def _load_token() -> str:
    token = os.environ.get("GH_TOKEN")
    if token:
        return token
    env_file = Path(__file__).resolve().parent.parent / ".env"
    if env_file.is_file():
        for line in env_file.read_text().splitlines():
            if line.startswith("GH_TOKEN="):
                return line.split("=", 1)[1].strip()
    print("Error: GH_TOKEN not found in environment or .env", file=sys.stderr)
    sys.exit(1)


def _gh_api(url: str, token: str) -> list | dict:
    results = []
    while url:
        req = urllib.request.Request(url, headers={
            "Authorization": f"token {token}",
            "Accept": "application/vnd.github+json",
            "X-GitHub-Api-Version": "2022-11-28",
        })
        with urllib.request.urlopen(req) as resp:
            data = json.loads(resp.read())
            if isinstance(data, list):
                results.extend(data)
            else:
                return data
            link = resp.headers.get("Link", "")
            match = re.search(r'<([^>]+)>;\s*rel="next"', link)
            url = match.group(1) if match else None
    return results


def _parse_pr_url(url: str) -> tuple[str, str, int]:
    m = re.match(
        r"https?://github\.com/([^/]+)/([^/]+)/pull/(\d+)", url,
    )
    if m:
        return m.group(1), m.group(2), int(m.group(3))
    raise ValueError(f"Cannot parse PR URL: {url}")


def fetch_coderabbit_comments(
    owner: str, repo: str, pr_number: int, token: str,
) -> dict:
    base = f"https://api.github.com/repos/{owner}/{repo}"

    review_comments = _gh_api(
        f"{base}/pulls/{pr_number}/comments?per_page=100", token,
    )
    coderabbit_inline = [
        c for c in review_comments
        if c["user"]["login"] == "coderabbitai[bot]"
    ]

    issue_comments = _gh_api(
        f"{base}/issues/{pr_number}/comments?per_page=100", token,
    )
    coderabbit_general = [
        c for c in issue_comments
        if c["user"]["login"] == "coderabbitai[bot]"
    ]

    reviews = _gh_api(
        f"{base}/pulls/{pr_number}/reviews?per_page=100", token,
    )
    coderabbit_reviews = [
        r for r in reviews
        if r["user"]["login"] == "coderabbitai[bot]"
    ]

    return {
        "pr": f"{owner}/{repo}#{pr_number}",
        "inline_comments": [
            {
                "path": c["path"],
                "line": c.get("line"),
                "diff_hunk": c.get("diff_hunk", ""),
                "body": c["body"],
                "created_at": c["created_at"],
                "html_url": c["html_url"],
            }
            for c in coderabbit_inline
        ],
        "review_summaries": [
            {
                "state": r["state"],
                "body": r["body"],
                "submitted_at": r["submitted_at"],
                "html_url": r["html_url"],
            }
            for r in coderabbit_reviews
            if r.get("body")
        ],
        "general_comments": [
            {
                "body": c["body"],
                "created_at": c["created_at"],
                "html_url": c["html_url"],
            }
            for c in coderabbit_general
        ],
    }


def _print_text(data: dict) -> None:
    pr = data["pr"]
    inline = data["inline_comments"]
    summaries = data["review_summaries"]
    general = data["general_comments"]

    print(f"CodeRabbit comments for {pr}")
    print("=" * 60)

    if inline:
        print(f"\n## Inline comments ({len(inline)})\n")
        for c in inline:
            loc = c["path"]
            if c["line"]:
                loc += f":{c['line']}"
            print(f"### {loc}")
            print(c["body"])
            print()

    if summaries:
        print(f"\n## Review summaries ({len(summaries)})\n")
        for r in summaries:
            print(f"### {r['state']} — {r['submitted_at']}")
            print(r["body"])
            print()

    if general:
        print(f"\n## General comments ({len(general)})\n")
        for c in general:
            print(f"### {c['created_at']}")
            print(c["body"])
            print()

    if not inline and not summaries and not general:
        print("\nNo CodeRabbit comments found on this PR.")


def main():
    parser = argparse.ArgumentParser(
        description="Fetch CodeRabbit review comments from a GitHub PR",
    )
    parser.add_argument(
        "pr_url_or_repo",
        help="Full PR URL or owner/repo",
    )
    parser.add_argument(
        "pr_number",
        nargs="?",
        type=int,
        help="PR number (when passing owner/repo instead of URL)",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        dest="output_json",
        help="Output as JSON",
    )
    args = parser.parse_args()

    if args.pr_number is not None:
        parts = args.pr_url_or_repo.split("/")
        if len(parts) != 2:
            parser.error("Expected owner/repo format")
        owner, repo = parts
        pr_number = args.pr_number
    else:
        owner, repo, pr_number = _parse_pr_url(args.pr_url_or_repo)

    token = _load_token()
    data = fetch_coderabbit_comments(owner, repo, pr_number, token)

    if args.output_json:
        json.dump(data, sys.stdout, indent=2)
        print()
    else:
        _print_text(data)


if __name__ == "__main__":
    main()
