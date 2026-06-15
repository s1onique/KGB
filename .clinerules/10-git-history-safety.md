# Git History Safety: Absolute Rule

Never execute, request approval for, suggest, or auto-retry any command that can rewrite or delete remote Git history.

## Forbidden Commands and Patterns

- `git push --force`
- `git push -f`
- `git push --force-with-lease`
- `git push --force-if-includes`
- `git push --mirror`
- `git push --delete`
- `git push --no-verify`
- `git push origin :branch`
- `git push origin +branch`
- Any push refspec beginning with `+`
- Any alias or wrapper that expands to these commands

## When a Normal Push Fails

If a normal `git push` is rejected because the remote has moved:

1. **Stop immediately**
2. Fetch: `git fetch origin`
3. Inspect the divergence
4. Rebase or merge locally
5. Push normally
6. **Do not force-push** — ask the human if stuck

## Emergency Exceptions

Force-push operations are **human-only emergency operations**. They are:
- Performed outside agent control
- Performed outside auto-approval
- Documented and communicated to collaborators
- Never attempted by Cline

## Enforcement

This rule is absolute and non-negotiable. Violations may cause:
- Data loss
- Broken collaboration
- Compromised audit trails

The policy is enforced by:
1. GitHub server-side rulesets (authoritative, cannot be bypassed)
2. Local pre-push hooks (early warning)
3. Cline rules (agent behavior control)

See `docs/doctrine/git-history-safety.md` for full policy details.
