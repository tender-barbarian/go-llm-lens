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
- Project memory tools persist codebase knowledge across sessions — no repeated orientation at the start of every conversation

**Where it doesn't help much:**

- Simple lookups in small files — Read on a known file is comparable
- Tasks that require understanding surrounding context (comments, neighboring functions)
- The tool call itself + its response still consume tokens, so very tiny queries have overhead

The bigger win is probably fewer round trips — less searching in the dark, fewer "read this file, now read that file" chains. That means less context accumulation overall, which is where token costs really compound.

## Benchmarks

### go-llm-lens vs Glob/Grep — Token Usage Benchmark

- **Task:** Describe the sample codebase (github.com/tender-barbarian/gniot)
- **Model:** claude-opus-4-6
- **Runs:** 3 (cold baseline — no prior project memories)
- **Date:** 2026-02-23

### Results

| Metric                         |          Glob/Grep |        go-llm-lens |
|--------------------------------|-------------------:|-------------------:|
| Effective tokens (mean ± sd) * | 42,900 ± 3,976     | 33,356 ± 965       |
| **Cost USD (mean ± sd)**       | **$0.2545 ± $0.0208** | **$0.1967 ± $0.0028** |

\* `input + output + cache_read × 0.1 + cache_creation × 1.25`  (reflects Opus 4.6 billing weights)

### Verdict

**go-llm-lens used ~22% fewer effective tokens and cost ~23% less (~$0.06/run saved) in a cold session.**

The consistency gap is equally striking: lens has a coefficient of variation of ~3% vs ~9% for Glob/Grep. The structured tool approach takes a predictable path — a handful of targeted calls, compact structured results, done. Glob/Grep lets the model improvise a search strategy each time, so costs swing with how many files it decides to read.

### Memory amortisation

The numbers above reflect a single session with no prior project knowledge. The `write_memory` / `list_memories` tools change the picture significantly across repeated sessions: the first session pays to explore and writes its findings; subsequent sessions read the notes and skip re-discovery entirely.

Observed across three consecutive benchmark executions on the same codebase (Glob/Grep held steady at ~42,000 effective tokens throughout):

| Session | Lens eff. tokens (mean) | vs Glob/Grep |
|---------|------------------------:|:-------------|
| 1 — cold | 33,356 | −22% |
| 2 — warm | ~25,700 | ~−39% |
| 3 — warmer | ~14,600 | ~−66% |

By the third session, individual runs were completing the same "describe the codebase" task in as few as **~8,000 effective tokens** — roughly a 5× reduction from a cold Glob/Grep session.

Run with and without `--no-memory` to see this amortisation effect on your own codebase (see below).

### Notes

- Results may vary by task type; simple symbol lookups on small codebases are where Grep is most competitive and can match go-llm-lens
- The advantage of go-llm-lens compounds on larger codebases and multi-step exploration tasks where Glob/Grep requires reading many files to build context

### Running your own benchmark

`tests/benchmark/compare-tokens.sh` runs two back-to-back `claude -p` sessions — one constrained to Glob/Grep and one to go-llm-lens — on the same task, then prints a side-by-side token and cost comparison.

```bash
# Single comparison, keep raw JSON output:
./tests/benchmark/compare-tokens.sh --target ~/projects/mylib --keep "describe the codebase structure"

# Run 3 times each, report mean ± stddev (memories accumulate between lens runs):
./tests/benchmark/compare-tokens.sh --target ~/projects/mylib --runs 3 "describe the codebase structure"

# Same, but with memory tools disabled — isolates structural tool savings:
./tests/benchmark/compare-tokens.sh --target ~/projects/mylib --runs 3 --no-memory "describe the codebase structure"
```

| Flag | Default | Description |
|------|---------|-------------|
| `--model` | `claude-opus-4-6` | Model to use for both sessions |
| `--runs`/`-n` | `1` | Number of runs per method; reports mean ± stddev when > 1 |
| `--no-memory` | off | Exclude memory tools from the lens session; useful for isolating structural tool savings from memory amortisation |
| `--target`/`-t` | `.` | Go project directory to benchmark against |
| `--keep`/`-k` | off | Keep raw JSON output files instead of deleting them |

Requirements: `claude` CLI in PATH, `jq`, and go-llm-lens configured as an MCP server in the current project.

## Security

**Seems like since the introduction of AI-assisted coding security became an afterthought. A lot of people are rightfully afraid of introducing AI slop into their workflows. As such I took special care to use proper security measures in this project.**

`go-llm-lens` is designed to be safe to run alongside an AI assistant:

- **Minimal write surface.** The server only writes to `.llm-lens/memories.json` within the project root (project memory tools). It never executes shell commands or makes network calls. All other operations are read-only.
- **No network surface.** Transport is stdio only. There is no HTTP server and no open port.
- **Scoped to `--root`.** The indexer only processes source files that physically reside under the directory you specify. Files outside that tree are never read.
- **Minimal token footprint.** Tools return structured JSON containing only the fields the LLM needs — signatures, types, locations, doc comments — rather than raw source files. Unexported symbols and function bodies are omitted by default (`include_unexported` / `include_bodies` opt in). This keeps context window usage predictable and small regardless of codebase size.
- **Input length limits.** String arguments to codebase-query tools are capped at 2 048 bytes before any handler logic runs, preventing resource exhaustion from oversized inputs. Memory tool values are uncapped to allow storing longer notes.
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

## LLM Integration

`go-llm-lens` is an MCP server so it can work with any AI coding tool, but it was developed and tested with Claude Code, so here's how to set it up.

### Add to Claude Code

```
claude mcp add --scope user --transport stdio go-llm-lens -- /path/to/go-llm-lens
```

### Encourage Claude to use it

Claude won't prefer these tools over Glob/Grep/Read by default. Add the following to your `CLAUDE.md` (global `~/.claude/CLAUDE.md` or project-level):

```markdown
## Codebase exploration
**ALWAYS use `go-llm-lens` MCP tools for Go symbol lookup. NEVER use Glob/Grep/Read to explore Go code structure.**

- `list_packages` — list all indexed packages
- `get_package_symbols` — browse all symbols in a package
- `get_file_symbols` — list symbols defined in a specific file
- `find_symbol` — locate any function/type/var/const by name (supports prefix/contains match)
- `get_function` — read full function/method definition including body
- `get_type` — read full struct or interface definition
- `find_implementations` — find all concrete types implementing an interface

Call `list_memories` at the start of every session and `write_memory` proactively to persist codebase knowledge across sessions.

Only fall back to Glob/Grep/Read for non-Go files or when the MCP server is unavailable.
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
| `include_bodies`     | bool   | no       | Include function bodies (default: false)       |

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
| `include_bodies`     | bool   | no       | Include function bodies (default: false)         |

**Output:** `{ funcs: [...], types: [...], vars: [...] }` scoped to the given file.

### `find_implementations`

Finds all concrete types in the indexed codebase that implement a given interface.

| Field       | Type   | Required | Description                              |
|-------------|--------|----------|------------------------------------------|
| `package`   | string | yes      | Package import path of the interface     |
| `interface` | string | yes      | Interface type name                      |

**Output:** Array of `{ name, package, location, implements_via }` where `implements_via` is `"value"` or `"pointer"`.

Uses `types.Implements` from `go/types` for precise, type-system-accurate results.

### Project memory

These four tools provide a persistent key/value notepad stored in `.llm-lens/memories.json` at the project root. The file is plain JSON — human-readable, editable, and safe to commit to Git so the whole team shares the same accumulated knowledge.

**Recommended usage:** call `list_memories` at the start of every session, and call `write_memory` proactively whenever you learn something reusable about the codebase.

### `list_memories`

Returns all memory notes for this project as key/value pairs.

No parameters.

**Output:** `{ "key": "value", ... }`

### `write_memory`

Creates or updates a named memory note.

| Field   | Type   | Required | Description  |
|---------|--------|----------|--------------|
| `key`   | string | yes      | Note name    |
| `value` | string | yes      | Note content |

**Output:** `"ok"`

### `read_memory`

Retrieves a single memory note by key. Returns an error if the key does not exist.

| Field | Type   | Required | Description |
|-------|--------|----------|-------------|
| `key` | string | yes      | Note name   |

**Output:** The stored string value.

### `delete_memory`

Removes a memory note. Returns an error if the key does not exist.

| Field | Type   | Required | Description |
|-------|--------|----------|-------------|
| `key` | string | yes      | Note name   |

**Output:** `"ok"`

## License

See [LICENSE](LICENSE).
