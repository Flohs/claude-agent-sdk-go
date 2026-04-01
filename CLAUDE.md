# Claude Code Project Instructions

## Development Workflow

All development work (features, bugfixes, code changes) MUST follow the workflow defined in `.claude/skills/dev-workflow.md`. This includes:

- Creating a GitHub issue before any code change
- Working on a dedicated issue branch (`feat/`, `fix/`, `chore/`)
- Updating `CHANGELOG.md` under `[Unreleased]` for every code change
- Referencing the issue number in branch names, commits, changelog entries, and PR descriptions
- Running `go build ./...`, `go vet ./...`, and `go test -race ./...` before committing
