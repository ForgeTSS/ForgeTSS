# How to Contribute

We welcome contributions. The documentation site you're reading is built from `docs/` and pushed to the `docs/documentation-site` branch (main is protected; PRs are merged after review).

## Finding an issue

Browse [open issues](https://github.com/ForgeTSS/ForgeTSS/issues) — look for the `good first issue` label if you're new to the codebase. Before opening a new issue, search existing ones; many gaps (like the environment variable mismatch, or the `USER`/`adduser` ordering in the Dockerfile) are already documented.

## Branch naming

Use descriptive prefixes:

| Prefix | Use case | Example |
|--------|----------|---------|
| `feat/` | New feature | `feat/sse-streaming` |
| `fix/` | Bug fix | `fix/channel-lease-race` |
| `docs/` | Documentation | `docs/api-reference` |

## Commit format

Follow the established convention (see `README.md`):

```
feat(api): wire store into handlers — SSE polling
test(submission): add engine tests covering retry
docs: fix typo in architecture section
```

No `Co-Authored-By` or AI signature lines are added to commits. Keep the message clean: `docs: add <section>/<page>` for documentation commits.

## Pull request checklist

- [ ] Tests pass: `go test ./...`
- [ ] Code builds: `go build ./...`
- [ ] `golangci-lint` is clean (when installed)
- [ ] One logical change per commit
- [ ] PR title follows the commit format
- [ ] For documentation changes: every SUMMARY.md link resolves, no orphaned pages, no broken cross-references

## Documentation discipline

Every page in `docs/` must be complete — no placeholder paragraphs, no "TODO" markers, no empty sections. Each section mentioned in `docs/SUMMARY.md` must have a file. Before pushing a docs PR, verify links with `grep -r '` across the docs directory. The build check includes link verification; broken links will block the PR.

## Code conventions

Read `internal/config/config.go` before adding environment variables; it is the single source of truth. Read `README.md` and `docs/` before rewriting descriptions of behavior — the documentation is kept aligned with the code, not ahead of it.
