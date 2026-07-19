#!/usr/bin/env bash
set -euo pipefail

# Default mode: staged changes (current index)
MODE="staged"
OUT=""
RANGE_ARG=""
declare -a FILE_ARGS=()

while [[ $# -gt 0 ]]; do
  case "$1" in
    --staged) MODE="staged"; shift ;;
    --unstaged) MODE="unstaged"; shift ;;
    --dirty) MODE="dirty"; shift ;;
    --range) MODE="range"; RANGE_ARG="$2"; shift 2 ;;
    --output) OUT="$2"; shift 2 ;;
    --) shift; break ;;
    -*) echo "ERROR: unknown flag $1" >&2; exit 1 ;;
    *) FILE_ARGS+=("$1"); shift ;;
  esac
done
for arg in "$@"; do FILE_ARGS+=("$arg"); done

[[ -z "$OUT" ]] && { echo "ERROR: --output is required" >&2; exit 1; }
command -v git >/dev/null 2>&1 || { echo "ERROR: git not found" >&2; exit 1; }

repo_root="$(git rev-parse --show-toplevel)"
cd "$repo_root"

# Determine files based on mode
declare -a FILES=()
if [[ ${#FILE_ARGS[@]} -gt 0 ]]; then
  FILES=("${FILE_ARGS[@]}")
else
  case "$MODE" in
    staged)   mapfile -t FILES < <(git diff --cached --name-only) ;;
    unstaged) mapfile -t FILES < <(git diff --name-only) ;;
    range)
      [[ -z "$RANGE_ARG" ]] && { echo "ERROR: --range requires a commit range argument" >&2; exit 1; }
      mapfile -t FILES < <(git diff --name-only "$RANGE_ARG") ;;
    dirty)
      mapfile -t STAGED_FILES < <(git diff --cached --name-only)
      mapfile -t UNSTAGED_FILES < <(git diff --name-only)
      mapfile -t UNTRACKED_FILES < <(git ls-files --others --exclude-standard)
      declare -A SEEN
      for f in "${STAGED_FILES[@]}" "${UNSTAGED_FILES[@]}" "${UNTRACKED_FILES[@]}"; do
        [[ -n "$f" && -z "${SEEN[$f]:-}" ]] || continue; SEEN[$f]=1; ALL_FILES+=("$f")
      done
      FILES=("${ALL_FILES[@]}")
      ;;
  esac
fi

if [[ ${#FILES[@]} -eq 0 ]]; then
  echo "No changed files found in mode: $MODE" >"$OUT"; echo "$OUT"; exit 0
fi

is_tracked()   { git ls-files --error-unmatch "$1" >/dev/null 2>&1; }
has_staged()   { git diff --cached --quiet -- "$1" 2>/dev/null && return 1 || return 0; }
has_unstaged() { git diff --quiet -- "$1" 2>/dev/null && return 1 || return 0; }

# name_status_cached returns the git name-status character (A/M/D/R/C/T)
# for a file from `git diff --cached --name-status`. Empty if absent.
name_status_cached() {
  local file="$1" line
  line=$(git diff --cached --name-status -- "$file" 2>/dev/null | head -n1)
  [[ -z "$line" ]] && return 1
  printf '%s' "$line" | awk '{print $1}'
}

diff_cmd() {
  case "$MODE" in
    staged) git diff --cached "$@" ;;
    unstaged) git diff "$@" ;;
    range) git diff "$RANGE_ARG" -- "$@" ;;
    dirty) echo "# ERROR: diff_cmd called in dirty mode, use staged_diff or unstaged_diff" >&2; return 1 ;;
  esac
}
staged_diff()   { git diff --cached "$@"; }
unstaged_diff() { git diff "$@"; }

# print_file_metadata: tracked vs untracked + presence of staged/unstaged
print_file_metadata() {
  local file="$1" staged_yes="no" unstaged_yes="no"
  if is_tracked "$file"; then
    has_staged "$file"   && staged_yes="yes"
    has_unstaged "$file" && unstaged_yes="yes"
    echo "Metadata: tracked, staged present: $staged_yes, unstaged present: $unstaged_yes"
  else
    echo "Metadata: untracked, staged present: no, unstaged present: yes"
  fi
}
is_file_untracked() { ! is_tracked "$1"; }

# print_file_entry: per-file classification including name-status character.
print_file_entry() {
  local file="$1" ns=""
  ns=$(name_status_cached "$file" || true)
  if is_tracked "$file"; then
    local staged_yes="no" unstaged_yes="no"
    has_staged "$file"   && staged_yes="yes"
    has_unstaged "$file" && unstaged_yes="yes"
    printf '%s  [tracked, name-status=%s, staged present: %s, unstaged present: %s]\n' \
      "$file" "${ns:-?}" "$staged_yes" "$unstaged_yes"
  else
    printf '%s  [untracked, name-status=%s, staged present: no, unstaged present: yes]\n' \
      "$file" "${ns:-A}"
  fi
}

{
  echo "# Targeted digest"
  echo
  echo "Generated at: $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
  echo "Repo: $repo_root"
  echo "Mode: $MODE"
  [[ -n "$RANGE_ARG" ]] && echo "Range: $RANGE_ARG"
  [[ ${#FILE_ARGS[@]} -gt 0 ]] && echo "File filter: ${FILE_ARGS[*]}"
  echo

  # Name-status summary: count A/M/D/R/C/T across all FILES, plus total.
  echo "## Name-status summary"
  for file in "${FILES[@]}"; do name_status_cached "$file" || true; done | awk '
    BEGIN { a=m=d=r=c=t=0 }
    /^A$/  { a++ } /^M$/  { m++ } /^T$/  { t++ }
    /^D$/  { d++ }
    /^R[0-9]+/  { r++ }
    /^C[0-9]+/  { c++ }
    END {
      mod=m+t
      ren=r+c
      printf "added_files=%d\n", a
      printf "modified_files=%d\n", mod
      printf "deleted_files=%d\n", d
      printf "renamed_or_copied_files=%d\n", ren
      printf "total_files=%d\n", a+mod+d+ren
    }'
  echo

  echo "## Changed files"
  for file in "${FILES[@]}"; do print_file_entry "$file"; done
  echo

  if [[ "$MODE" == "dirty" ]]; then
    echo "## Diffs"
    for file in "${FILES[@]}"; do
      echo; echo "=== $file ==="; print_file_metadata "$file"; echo
      if is_file_untracked "$file"; then
        echo "--- untracked file preview ---"
        [[ -f "$file" ]] && cat "$file" || echo "(file not present)"
        continue
      fi
      if has_staged "$file"; then echo "--- staged diff ---"; staged_diff --unified=3 -- "$file"; echo; fi
      if has_unstaged "$file"; then echo "--- unstaged diff ---"; unstaged_diff --unified=3 -- "$file"; fi
    done
  else
    echo "## Diff stat"; diff_cmd --stat -- "${FILES[@]}"; echo
    echo "## Diffs"
    for file in "${FILES[@]}"; do echo; echo "=== $file ==="; diff_cmd --unified=3 -- "$file" || true; done
  fi

  echo; echo "## Workflow anchors"
  for file in "${FILES[@]}"; do
    [[ -f "$file" ]] || continue
    case "$file" in
      frontend/src/App.tsx|frontend/src/__tests__/app.test.tsx|frontend/src/index.css)
        echo; echo "### ANCHORS IN: $file"
        grep -nE 'WORKFLOW_LANES|Diagnose now|Diagnose Now|Work next checks|Work Next Checks|Improve the system|Improve the System|ExecutionHistoryPanel|ReviewEnrichmentPanel|ProviderExecutionPanel|LLMActivityPanel|LLMPolicyPanel|Proposal' "$file" || true
        ;;
    esac
  done
} >"$OUT"

echo "$OUT"
