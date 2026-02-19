# Contributing

## Reporting bugs

Open a [GitHub issue](https://github.com/tender-barbarian/go-llm-lens/issues/new) with a description of the problem and steps to reproduce it.

For security vulnerabilities, follow the [security policy](SECURITY.md) instead.

## Submitting changes

1. Fork the repository and create a branch from `main`.
2. Make your changes. All code must pass the checks below before review.
3. Open a pull request against `main`. The description should explain what the change does and why.

A maintainer will review and merge the PR. At least one approving review is required.

## Local checks

```bash
make check   # vet + lint + test
make build   # verify the binary compiles
```

Requirements:
- Go 1.25+
- [golangci-lint](https://golangci-lint.run/welcome/install/) v2+

## Code style

Follow the [Uber Go Style Guide](https://github.com/uber-go/guide/blob/master/style.md). Key points:

- Keep functions focused; prefer early returns over deep nesting.
- No `else` after `return`, `break`, or `continue`.
- Wrap errors with `fmt.Errorf("doing X: %w", err)`; never return raw errors.
- Add tests for new behaviour. Table-driven tests are preferred for multiple cases.
- Do not add dependencies without discussion.
