# Contributing to Claude Agent SDK for Go

Thank you for your interest in contributing! This document provides guidelines for contributing to this project.

## Getting Started

1. Fork the repository
2. Clone your fork: `git clone https://github.com/<your-username>/claude-agent-sdk-go.git`
3. Create a branch: `git checkout -b my-feature`
4. Make your changes
5. Push and open a pull request

## Prerequisites

- Go 1.26+
- Claude Code CLI (`npm install -g @anthropic-ai/claude-code`) for running integration tests

## Development

```bash
# Run tests
go test ./...

# Run tests with race detection
go test -race ./...

# Run linter
go vet ./...
```

## Pull Requests

- Keep PRs focused on a single change
- Include tests for new functionality
- Ensure all tests pass and `go vet` reports no issues
- Update documentation (README, CHANGELOG) if applicable
- Write clear commit messages

## Reporting Bugs

Please use [GitHub Issues](https://github.com/Flohs/claude-agent-sdk-go/issues) and include:

- Go version (`go version`)
- Claude Code CLI version (`claude --version`)
- Steps to reproduce
- Expected vs actual behavior

## Feature Requests

Open an issue describing the use case and proposed API. Discussion before implementation helps avoid wasted effort.

## Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- Keep the public API surface minimal
- Prefer clear, simple code over clever abstractions

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
