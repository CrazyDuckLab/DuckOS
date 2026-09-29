# Engineering rules for continuation

- Continue the current roadmap from its first unfinished acceptance gate. No
  broad redesign without a demonstrated blocker in the existing implementation.
- Record necessary architectural changes in an ADR, including evidence,
  compatibility, migration and recovery implications. Never change them silently.
- Avoid cosmetic refactoring and speculative infrastructure while functional
  integration remains incomplete. Preserve working code and unrelated changes.
- Prefer complete implementation/test/fix blocks. Prioritize real end-to-end
  behavior over additional disconnected scaffolding or progress reports.
- Never replace production observations, enforcement or recovery with fixtures,
  optimistic success responses or stubs. Test doubles remain test-only.
- Preserve API → Engine → authenticated privileged Agent boundaries. No direct
  UI/CLI host writes, arbitrary root commands, shell snippets or unchecked paths.
- Security/regression checks for firewall, isolation, authentication, secrets,
  privileges, tunnels, updates, Safe Apply and recovery are mandatory. Speed or
  usage optimization is not permission to weaken or omit them.
- Keep secrets out of ordinary reads, logs, audit details, QR assets, support
  bundles, diffs, command arguments and repository content. Audit secret access
  using identifiers and actions, not payloads.
- Attribute generated objects; preview effects and dependencies; retain safe
  rollback and management access. No silent automation or implicit WAN exposure.
- Mark results truthfully: specified, implemented, locally tested, cross-built,
  native-tested and release-qualified are different states. A completed item
  includes its actual test status and remaining limitations.
- Run applicable tests, security regressions, builds and diff review before
  committing/pushing. Use normal engineering commit messages; never force-push
  main or rewrite history to remove development provenance.
- Keep raw conversations/prompts, temporary session notes and tool-specific
  instructions out of product documentation. These explicitly requested handoff
  files are the maintained continuation guide, not a transcript archive.
- Work in the existing local checkout. Do not recreate it, discard changes or
  alter system components merely to make tests pass. Linux targets receive exact
  committed code from GitHub; do not access `duck-dev-01` without authorization.
- Do not create public Alpha releases, change repository visibility, select a
  license/domain, publish announcements, or enable indexing implicitly.
- Ask one concrete question only when an unresolved material decision or unsafe
  destructive action genuinely needs approval. Otherwise follow accepted
  contracts and record a reasonable scoped engineering decision.
