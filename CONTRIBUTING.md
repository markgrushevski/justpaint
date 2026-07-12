# Contributing

How work flows through justpaint — branches, commits, and the local gates. The _why_ behind these
choices lives in [`docs/DECISIONS.md`](docs/DECISIONS.md); the coding conventions and commands in
[`CLAUDE.md`](CLAUDE.md); the per-change quality bar in [`docs/REVIEW.md`](docs/REVIEW.md).

## Prerequisites

- **Node ≥ 22** and npm (the repo is an npm-workspace monorepo — one root `npm install` wires
  `packages/*` + `apps/*`).
- **Go ≥ 1.26** (`server/go.mod` declares `go 1.26`).
- **Docker** for local Postgres (`docker compose up -d` at the repo root → postgres:17-alpine).
- **goose** (migrations) and **sqlc** (query codegen) as external CLIs — installed separately, not
  Go module deps.

## Branching

`main` is the trunk. The deciding factor for whether to branch is **commit count, not size**:

- **One commit** — even a big self-contained change — goes **straight to `main`**: commit there once
  the gates are green (no branch, no merge bubble).
- **Several commits** grouped under one unit of work get a **type-prefixed branch** (`feat/…`,
  `fix/…`, `docs/…`, `refactor/…`, `build/…`, `chore/…`), integrated with a **`--no-ff` merge** so
  `main`'s first-parent history records each unit as a single merge commit — then delete the branch.

```bash
git switch -c feat/my-thing main
# … commits …
git switch main && git merge --no-ff feat/my-thing
git branch -d feat/my-thing
```

**Parallel work:** the backend (`server/`) and the frontend (`packages/` + `apps/`) share no files
and both validate against the frozen contract ([`docs/DOCUMENT-FORMAT.md`](docs/DOCUMENT-FORMAT.md)),
so they can run on **parallel branches**. Integration — review, `--no-ff` merge, and the
ROADMAP-status update — stays **serialized** through one orchestrator. Within a single domain,
isolate parallel work with git worktrees or sequential commits to avoid same-file conflicts.

## Commits

[Conventional Commits](https://www.conventionalcommits.org): `feat` / `fix` / `refactor` / `build` /
`docs` / `chore` …, `!` for breaking. **Present tense, one logical change per commit** — group
related edits rather than shipping many tiny commits. Git author is **Leonid**.

**Update [`docs/ROADMAP.md`](docs/ROADMAP.md) in the same change that lands a deliverable or flips a
phase.** The ROADMAP is the durable status tracker; a stale ROADMAP is a bug, not a nit.

## Local gates (no CI yet)

There is no GitHub Actions gate (that's an [`docs/IDEAS.md`](docs/IDEAS.md) item) — run the checks
locally before committing:

```sh
# TypeScript side (repo root)
npm run types        # vue-tsc / tsc --noEmit across packages/* + apps/*
npm run test         # Vitest across workspaces
npm run build        # package dist/ + vite build

# Go side (in server/)
gofmt -l .           # must print nothing
go vet ./...
go test ./...
go build ./...
```

Then self-review the diff against [`docs/REVIEW.md`](docs/REVIEW.md) — the judgment layer the
commands can't assert (contract parity, the trust boundary, scope discipline). A criterion
deliberately not met is fine **only if** recorded in [`docs/DECISIONS.md`](docs/DECISIONS.md);
otherwise it's a finding.

## No release machinery

justpaint is an **app, not a published library** — there is no SemVer/changesets/publish flow. `main`
is simply the current state of the project. (Contrast with the sibling oriui library, which does
publish.)
