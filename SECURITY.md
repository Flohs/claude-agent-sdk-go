# Security Policy

## Supported Versions

| Version | Supported          |
| ------- | ------------------ |
| 0.2.x   | :white_check_mark: |
| < 0.2   | :x:                |

## Reporting a Vulnerability

If you discover a security vulnerability, please report it responsibly.

**Do not open a public issue.**

Instead, please use [GitHub's private vulnerability reporting](https://github.com/Flohs/claude-agent-sdk-go/security/advisories/new) to submit your report.

You should receive a response within 72 hours. We will work with you to understand the issue and coordinate a fix before any public disclosure.

## Scope

This SDK communicates with the Claude Code CLI via subprocess stdio. Security concerns may include:

- Command injection through unsanitized inputs
- Unsafe handling of subprocess I/O
- Information disclosure through error messages or logs
