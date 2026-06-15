# Git History Safety Doctrine

Git history rewriting is a destructive operation that can cause data loss, break collaboration, and compromise audit trails. KGB enforces strict protection against force pushes and history manipulation.

## Policy

### Absolute Prohibition

The following operations are **forbidden** for all agents, automated systems, and non-emergency human workflows:

- Force pushes (`git push --force`, `git push -f`)
- Force-with-lease pushes (`git push --force-with-lease`)
- Force-if-includes pushes (`git push --force-if-includes`)
- Mirror pushes (`git push --mirror`)
- Branch/tag deletions via push (`git push --delete`, `git push origin :branch`)
- Non-fast-forward updates (`git push +branch`)
- Bypassing hooks (`git push --no-verify`)

### Rationale

1. **Server-side GitHub protection** is the authoritative control — it cannot be bypassed
2. **Local hooks** provide early warning but can be bypassed with `--no-verify`
3. **Cline rules** are the innermost tripwire for agent behavior

The layered approach ensures that even if one layer is circumvented, others remain as defense.

## Enforcement Layers

### Layer 1: GitHub Repository Rulesets (Authoritative)

GitHub rulesets provide the hard stop that no client-side configuration can bypass.

**Recommended ruleset configuration:**

```
Name: No history rewriting
Target: All branches
Enforcement: Active
Bypass actors: None
Rules:
  - Block force pushes (non_fast_forward)
  - Restrict deletions
```

Tags should also be protected against rewrites where the GitHub plan allows.

### Layer 2: Local pre-push Hook

The `scripts/git_no_history_rewrite_pre_push.sh` hook inspects actual ref updates rather than parsing command-line flags.

**It rejects:**
- Branch deletions (local_oid is all zeroes)
- Non-fast-forward branch updates (verified via `git merge-base --is-ancestor`)
- Tag deletions
- Tag rewrites

**It allows:**
- Normal fast-forward pushes
- New branch creation
- New tag creation

### Layer 3: Cline Rules (Innermost Tripwire)

See `.clinerules/10-git-history-safety.md` for the absolute agent ban.

## What To Do When a Normal Push Fails

If a normal `git push` is rejected because the remote has moved:

1. **Stop**. Do not attempt to force-push.
2. Fetch and inspect: `git fetch origin`
3. Review the divergence: `git log origin/main..main`
4. Rebase or merge locally, then push normally
5. If truly stuck, escalate to human operators with justification

## Emergency Exception Process

Force-push operations are **human-only emergency operations** performed outside agent control and outside auto-approval. The emergency must be documented and communicated to affected collaborators before or immediately after the force-push.

## Scope

This policy applies to:
- All KGB repositories
- All branches and tags
- All push channels (SSH, HTTPS, GitHub CLI)
- All actors (humans, bots, deploy keys, GitHub Actions)

## References

- [GitHub Rulesets Documentation](https://docs.github.com/repositories/configuring-branches-and-merges-in-your-repository/managing-protected-branches/about-protected-branches)
- [Git Hooks Documentation](https://git-scm.com/book/en/v2/Customizing-Git-Git-Hooks)
- [GitHub Rules API](https://docs.github.com/en/rest/repos/rules)
