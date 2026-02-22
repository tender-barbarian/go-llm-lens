# Go LLM Lens

[![CI](https://github.com/tender-barbarian/go-llm-lens/actions/workflows/ci.yml/badge.svg)](https://github.com/tender-barbarian/go-llm-lens/actions/workflows/ci.yml)
[![Release](https://github.com/tender-barbarian/go-llm-lens/actions/workflows/release.yml/badge.svg)](https://github.com/tender-barbarian/go-llm-lens/releases/latest)
[![OpenSSF Scorecard](https://api.securityscorecards.dev/projects/github.com/tender-barbarian/go-llm-lens/badge)](https://securityscorecards.dev/viewer/?uri=github.com/tender-barbarian/go-llm-lens)
[![OpenSSF Baseline](https://www.bestpractices.dev/projects/11985/baseline)](https://www.bestpractices.dev/projects/11985)
[![Go Report Card](https://goreportcard.com/badge/github.com/tender-barbarian/go-llm-lens)](https://goreportcard.com/report/github.com/tender-barbarian/go-llm-lens)

An MCP (Model Context Protocol) server that enables LLMs like Claude to navigate and understand Go codebases through full type-checked AST analysis.

Instead of reading raw source files line by line, an LLM can call structured tools to explore packages, find symbols, inspect function signatures and type definitions, and discover interface implementations.

The main goal is to save tokens on constant re-learning of the codebase.

## How it works

The indexer uses `golang.org/x/tools/go/packages` to perform full type-checked loading of the entire codebase at startup, then builds an in-memory index of all packages, functions, types, variables, and constants. The index is queried by MCP tools without re-parsing source files.

**Where it saves tokens:**

- Instead of reading entire files to find a function, `get_function` returns just that function's source
- Instead of grepping + reading multiple files to understand a type, `get_type` returns the definition directly
- `find_implementations` replaces multi-step grep → read → parse workflows
- Structured results are more compact than raw file content with line numbers

**Where it doesn't help much:**

- Simple lookups in small files — Read on a known file is comparable
- Tasks that require understanding surrounding context (comments, neighboring functions)
- The tool call itself + its response still consume tokens, so very tiny queries have overhead

The bigger win is probably fewer round trips — less searching in the dark, fewer "read this file, now read that file" chains. That means less context accumulation overall, which is where token costs really compound.

## Benchmarks

### go-llm-lens vs Glob/Grep — Token Usage Benchmark

**Task:** Describe the sample codebase (github.com/tender-barbarian/gniot)
**Model:** claude-opus-4-6
**Date:** 2026-02-20

### Results

| Metric                  |   Glob/Grep | go-llm-lens |
|-------------------------|------------:|------------:|
| Input tokens            |          10 |          14 |
| Output tokens           |       6,004 |       6,483 |
| Cache read tokens       |     338,868 |     364,173 |
| Cache creation tokens   |      73,719 |      35,612 |
| **Effective tokens** *  | **132,050** |  **87,430** |
| **Cost (USD)**          |  **$0.781** |  **$0.606** |
| Permission denials      |           0 |           0 |

\* `input + output + cache_read × 0.1 + cache_creation × 1.25`  (reflects Opus 4.6 billing weights)

### Verdict

**go-llm-lens used ~34% fewer effective tokens and cost 22% less ($0.17 saved).**

The primary driver was cache creation tokens — the most expensive token type at 1.25× input price. `go-llm-lens` produced roughly half the cache creation (35k vs 73k) because it returns targeted structured data rather than raw file contents, keeping less new material in the context cache.

### Notes

- `go-llm-lens` session used Haiku for MCP tool orchestration (886 in / 1,263 out via `claude-haiku-4-5`) — this overhead is cheap and confirms tools actually ran
- Results may vary by task type; symbol lookup tasks likely favour `go-llm-lens` more than broad narrative tasks like this one

## Security

**Seems like since the introduction of AI-assisted coding security became an afterthought. A lot of people are rightfully afraid of introducing AI slop into their workflows. As such I took special care to use proper security measures in this project.**

`go-llm-lens` is designed to be safe to run alongside an AI assistant:

- **Read-only.** The server never writes to disk, executes shell commands, or makes network calls. It only reads Go source files via the standard `go/packages` loader.
- **No network surface.** Transport is stdio only. There is no HTTP server and no open port.
- **Scoped to `--root`.** The indexer only processes source files that physically reside under the directory you specify. Files outside that tree are never read.
- **Input length limits.** All string arguments sent by the LLM are capped at 2 048 bytes before any handler logic runs, preventing resource exhaustion from oversized inputs.
- **Minimal token footprint.** Tools return structured JSON containing only the fields the LLM needs — signatures, types, locations, doc comments — rather than raw source files. Unexported symbols are omitted by default. This keeps context window usage predictable and small regardless of codebase size.
- **Dependency vulnerability scanning.** CI runs [`govulncheck`](https://pkg.go.dev/golang.org/x/vuln/cmd/govulncheck) on every push to catch known CVEs in dependencies.
- **Security linting.** [`gosec`](https://github.com/securego/gosec) is enabled in the golangci-lint configuration.
- **Pinned CI actions.** Every GitHub Actions step is pinned to an immutable commit SHA to prevent supply-chain attacks via mutable tags.
- **Signed and attested releases.** All release artifacts are signed with [Sigstore](https://www.sigstore.dev/) keyless signing and include [SLSA provenance attestations](https://slsa.dev/). Verify a download:

  ```bash
  # Verify signature
  cosign verify-blob \
    --certificate-identity-regexp='github.com/tender-barbarian/go-llm-lens' \
    --certificate-oidc-issuer='https://token.actions.githubusercontent.com' \
    --bundle checksums.txt.bundle \
    checksums.txt

  # Verify SLSA provenance
  gh attestation verify checksums.txt \
    --repo tender-barbarian/go-llm-lens
  ```

## Limitations

- **Codebase must build.** Full type checking requires the code to compile. Broken packages are skipped with a warning.
- **Dependencies must be available.** Run `go mod download` in the target codebase before starting the server.
- **Index is built at startup.** Changes to the codebase require restarting the server.
- **One codebase per server instance.** Use multiple server instances for multiple codebases.
- **Standard library not indexed.** Only packages under the module root (`./...`) are indexed.

## Prerequisites

- Go 1.25+
- The target codebase must compile cleanly (`go build ./...` passes)
- Dependencies must be downloaded (`go mod download` has been run)

## Installation

**Download a pre-built binary** from the [latest release](https://github.com/tender-barbarian/go-llm-lens/releases/latest) for your platform (Linux, macOS, Windows — amd64 and arm64).

**Or install with Go:**

```bash
go install github.com/tender-barbarian/go-llm-lens/cmd/server@latest
```

**Or build from source:**

```bash
git clone https://github.com/tender-barbarian/go-llm-lens
cd go-llm-lens
go build -o go-llm-lens ./cmd/server
```

## Usage

```bash
go-llm-lens --root /path/to/your/go/repo
```

The server communicates over **stdio** using the MCP protocol.

### Flags

| Flag     | Default | Description                                |
|----------|---------|--------------------------------------------|
| `--root` | `.`     | Root directory of the Go codebase to index |

### Add to Claude Code

```
claude mcp add --scope user --transport stdio go-llm-lens -- /path/to/go-llm-lens --root /path/to/your/go/repo
```

## MCP Tools

All tools return JSON-encoded results.

### `list_packages`

Lists all indexed packages with summary statistics.

| Field    | Type   | Required | Description                          |
|----------|--------|----------|--------------------------------------|
| `filter` | string | no       | Optional prefix filter on import path |

**Output:** Array of `{ import_path, name, dir, file_count, func_count, type_count }`

### `get_package_symbols`

Returns all symbols in a package: functions, types, variables, and constants.

| Field                | Type   | Required | Description                                    |
|----------------------|--------|----------|------------------------------------------------|
| `package`            | string | yes      | Package import path                            |
| `include_unexported` | bool   | no       | Include unexported symbols (default: false)    |

**Output:** `{ funcs: [...], types: [...], vars: [...] }` each with signature and doc comment.

### `find_symbol`

Searches for a symbol by name across the entire indexed codebase.

| Field   | Type   | Required | Description                                                          |
|---------|--------|----------|----------------------------------------------------------------------|
| `name`  | string | yes      | Symbol name to search for                                            |
| `kind`  | string | no       | Filter by kind: `func`, `method`, `type`, `var`, `const` (empty = all) |
| `match` | string | no       | Match mode: `exact` (default), `prefix`, or `contains`              |

**Output:** Array of matches with package, kind, signature, receiver (for methods), and location.

### `get_function`

Returns full details for a specific function or method.

| Field     | Type   | Required | Description                                     |
|-----------|--------|----------|-------------------------------------------------|
| `package` | string | yes      | Package import path                             |
| `name`    | string | yes      | Function name, or `TypeName.MethodName` for methods |

**Output:** Full signature, parameter names and types, return types, doc comment, implementation body, `is_promoted` (true for methods promoted from embedded types), file and line.

### `get_type`

Returns full definition of a type (struct or interface).

| Field     | Type   | Required | Description         |
|-----------|--------|----------|---------------------|
| `package` | string | yes      | Package import path |
| `name`    | string | yes      | Type name           |

**Output:**
- For structs: fields with types, struct tags, and comments; all methods (with `is_promoted` flag for methods from embedded types); embedded types
- For interfaces: method signatures with parameter and return types; embedded interfaces
- Doc comment, file and line

### `get_file_symbols`

Returns all symbols defined in a specific file.

| Field                | Type   | Required | Description                                      |
|----------------------|--------|----------|--------------------------------------------------|
| `file`               | string | yes      | File path (absolute or relative)                 |
| `include_unexported` | bool   | no       | Include unexported symbols (default: false)      |

**Output:** `{ funcs: [...], types: [...], vars: [...] }` scoped to the given file.

### `find_implementations`

Finds all concrete types in the indexed codebase that implement a given interface.

| Field       | Type   | Required | Description                              |
|-------------|--------|----------|------------------------------------------|
| `package`   | string | yes      | Package import path of the interface     |
| `interface` | string | yes      | Interface type name                      |

**Output:** Array of `{ name, package, location, implements_via }` where `implements_via` is `"value"` or `"pointer"`.

Uses `types.Implements` from `go/types` for precise, type-system-accurate results.

## License

See [LICENSE](LICENSE).
