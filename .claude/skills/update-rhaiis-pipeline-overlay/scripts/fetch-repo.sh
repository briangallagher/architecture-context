#!/usr/bin/env bash
# Locate or clone the rhaiis/pipeline repository.
# Prints the path to the repository root on stdout.
set -euo pipefail

LOCAL_CHECKOUT="../rhaiis/pipeline"
REMOTE_URL="https://gitlab.com/redhat/rhel-ai/rhaiis/pipeline.git"
CLONE_DIR="./tmp/rhaiis-pipeline"
DEFAULT_BRANCH="main"

# Normalize a git remote URL to a canonical "host/path" form so that HTTPS,
# scp-style SSH (git@host:path), ssh:// URLs, and an optional ".git" suffix all
# compare equal. glab and the GitLab UI hand out several of these forms.
normalize_remote() {
    local url="$1"
    url="${url%.git}"          # drop trailing .git
    url="${url%/}"             # drop trailing slash
    # Strip known secure schemes; http:// is intentionally excluded
    case "${url}" in
        ssh://*)   url="${url#ssh://}" ;;
        https://*) url="${url#https://}" ;;
    esac
    # Strip userinfo (user@) ONLY from the authority component (the part before
    # the first path separator). The old ${url#*@} matches any @ anywhere in the
    # URL and lets an attacker embed the real host in a path component.
    local authority path_rest
    if [[ "${url}" == */* ]]; then
        authority="${url%%/*}"   # everything before the first /
        path_rest="${url#*/}"    # everything after the first /
        if [[ "${authority}" == *@* ]]; then
            authority="${authority##*@}"
        fi
        url="${authority}/${path_rest}"
    else
        # No path separator — entire value is the authority (e.g. git@host:path)
        if [[ "${url}" == *@* ]]; then
            url="${url##*@}"
        fi
    fi
    url="${url/://}"           # scp-style host:path -> host/path (first colon)
    printf '%s' "${url}"
}

EXPECTED_REMOTE="$(normalize_remote "${REMOTE_URL}")"

is_allowed_remote() {
    local url="$1"
    # Reject plain HTTP — unencrypted transport is not an approved form
    if [[ "${url}" == http://* ]]; then
        return 1
    fi
    # Require an explicit remote URL form: an https:// or ssh:// scheme, or
    # scp-style [user@]host:path (a colon before the first slash). A scheme-less
    # "host/path" is a LOCAL folder path to git, so it must NOT satisfy the
    # allowlist even though it normalizes to the expected host/path string.
    if [[ "${url}" != https://* && "${url}" != ssh://* && "${url%%/*}" != *:* ]]; then
        return 1
    fi
    [[ "$(normalize_remote "$url")" == "${EXPECTED_REMOTE}" ]]
}

# Returns 0 if the checkout is a clean, up-to-date main; 1 (with a warning) otherwise.
# Never mutates the caller's working tree.
is_fresh_main() {
    local dir="$1"
    git -C "${dir}" fetch --quiet origin "${DEFAULT_BRANCH}" 2>/dev/null || {
        echo "WARNING: could not fetch origin/${DEFAULT_BRANCH} for ${dir}; falling back to a clone." >&2
        return 1
    }
    local branch
    branch="$(git -C "${dir}" rev-parse --abbrev-ref HEAD 2>/dev/null || echo '(detached)')"
    if [[ "${branch}" != "${DEFAULT_BRANCH}" ]]; then
        echo "WARNING: ${dir} is on branch '${branch}', not '${DEFAULT_BRANCH}'; falling back to a clone." >&2
        return 1
    fi
    local behind ahead
    behind="$(git -C "${dir}" rev-list --count "HEAD..FETCH_HEAD" 2>/dev/null || echo 0)"
    ahead="$(git -C "${dir}" rev-list --count "FETCH_HEAD..HEAD" 2>/dev/null || echo 0)"
    if [[ "${behind}" -gt 0 ]] || [[ "${ahead}" -gt 0 ]]; then
        echo "WARNING: ${dir} diverges from origin/${DEFAULT_BRANCH} (ahead=${ahead}, behind=${behind}); falling back to a clone." >&2
        return 1
    fi
    local dirty
    dirty="$(git -C "${dir}" status --porcelain 2>/dev/null | head -c 1)"
    if [[ -n "${dirty}" ]]; then
        echo "WARNING: ${dir} has uncommitted local changes; falling back to a clone." >&2
        return 1
    fi
    return 0
}

if [[ -d "${LOCAL_CHECKOUT}/.git" ]]; then
    ACTUAL_REMOTE="$(git -C "${LOCAL_CHECKOUT}" remote get-url origin 2>/dev/null || true)"
    if is_allowed_remote "${ACTUAL_REMOTE}" && is_fresh_main "${LOCAL_CHECKOUT}"; then
        echo "Using local checkout at ${LOCAL_CHECKOUT}" >&2
        echo "${LOCAL_CHECKOUT}"
        exit 0
    fi
    echo "WARNING: local ${LOCAL_CHECKOUT} is not an acceptable source; falling back to a clone." >&2
fi

mkdir -p "$(dirname "${CLONE_DIR}")"

if [[ -d "${CLONE_DIR}/.git" ]]; then
    ACTUAL_REMOTE="$(git -C "${CLONE_DIR}" remote get-url origin 2>/dev/null || true)"
    if ! is_allowed_remote "${ACTUAL_REMOTE}"; then
        echo "ERROR: ${CLONE_DIR} remote is '${ACTUAL_REMOTE}', expected '${REMOTE_URL}'" >&2
        exit 1
    fi
    echo "Updating existing clone at ${CLONE_DIR}..." >&2
    git -C "${CLONE_DIR}" pull --ff-only
else
    echo "Cloning ${REMOTE_URL} to ${CLONE_DIR}..." >&2
    git clone --depth=1 "${REMOTE_URL}" "${CLONE_DIR}"
fi

echo "${CLONE_DIR}"
