#!/usr/bin/env bash
# install_git_safety_hooks.sh
# Installs the git history safety pre-push hook.
#
# Usage:
#   ./scripts/install_git_safety_hooks.sh           # Interactive install
#   ./scripts/install_git_safety_hooks.sh --force   # Overwrite existing hook

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "${SCRIPT_DIR}/.." && pwd)"
HOOK_SOURCE="${REPO_ROOT}/scripts/git_no_history_rewrite_pre_push.sh"
HOOK_DEST="${REPO_ROOT}/.git/hooks/pre-push"
MANAGED_MARKER="# MANAGED BY KGB - DO NOT EDIT"

usage() {
    echo "Usage: $0 [--force]"
    echo ""
    echo "Installs the KGB git history safety pre-push hook."
    echo ""
    echo "Options:"
    echo "  --force    Overwrite existing pre-push hook if present"
    echo ""
    echo "The hook prevents:"
    echo "  - Force pushes (non-fast-forward updates)"
    echo "  - Branch deletions"
    echo "  - Tag rewrites"
    echo "  - Tag deletions"
    exit 1
}

FORCE=0
for arg in "$@"; do
    case "$arg" in
        --force) FORCE=1 ;;
        --help|-h) usage ;;
    esac
done

# Verify we're in a git repo
if [ ! -d ".git" ]; then
    echo "ERROR: Not in a git repository root" >&2
    exit 1
fi

# Verify hook source exists
if [ ! -f "$HOOK_SOURCE" ]; then
    echo "ERROR: Hook source not found: $HOOK_SOURCE" >&2
    exit 1
fi

# Check if hook already exists
if [ -f "$HOOK_DEST" ]; then
    if [ "$FORCE" -eq 1 ]; then
        echo "WARNING: Overwriting existing pre-push hook (--force specified)"
    else
        # Check if it's already managed by us
        if grep -q "$MANAGED_MARKER" "$HOOK_DEST" 2>/dev/null; then
            echo "Hook already installed (managed by KGB)"
            echo "Installed path: $HOOK_DEST"
            exit 0
        else
            echo "ERROR: A pre-push hook already exists" >&2
            echo "Path: $HOOK_DEST" >&2
            echo "" >&2
            echo "Use --force to overwrite, or manually integrate the hook." >&2
            exit 1
        fi
    fi
fi

# Ensure hooks directory exists
mkdir -p "$(dirname "$HOOK_DEST")"

# Copy and make executable, preserving shebang and prepending marker as line 2
{
    head -n 1 "$HOOK_SOURCE"
    echo "$MANAGED_MARKER"
    tail -n +2 "$HOOK_SOURCE"
} > "$HOOK_DEST"
chmod +x "$HOOK_DEST"

echo "KGB git safety hook installed successfully"
echo "Installed path: $HOOK_DEST"
echo ""
echo "The hook will prevent:"
echo "  - Force pushes (non-fast-forward updates)"
echo "  - Branch deletions"
echo "  - Tag rewrites and deletions"
echo ""
echo "To uninstall, remove: $HOOK_DEST"
