#!/usr/bin/env bash
# compare-tokens.sh
# Compare token usage between go-llm-lens MCP tools and Glob/Grep for a given task.
#
# Usage: compare-tokens.sh [--model MODEL] [--runs N] [--no-memory] [--target DIR] [--keep] "<task description>"
# Example: compare-tokens.sh --target ~/projects/mylib "find all types that implement the Handler interface"
#
# Note: --target must be a Go project with go-llm-lens configured as an MCP server.
# MCP tool permissions must be allowed during the session — if they are denied the benchmark is invalid.

set -euo pipefail
export LC_ALL=C  # ensure awk uses '.' as decimal separator regardless of system locale

# ── Defaults ──────────────────────────────────────────────────────────────────
MODEL="claude-opus-4-6"
RUNS=1
NO_MEMORY=false
KEEP=false
TARGET="."

# ── Argument parsing ──────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
    case "$1" in
        --model)      MODEL="$2";  shift 2 ;;
        --runs|-n)    RUNS="$2";   shift 2 ;;
        --no-memory)  NO_MEMORY=true; shift ;;
        --target|-t)  TARGET="$2"; shift 2 ;;
        --keep|-k)    KEEP=true;   shift   ;;
        --)           shift; break ;;
        -*)           echo "Unknown flag: $1" >&2; exit 1 ;;
        *)            break ;;
    esac
done

if [[ $# -lt 1 ]]; then
    echo "Usage: $0 [--model MODEL] [--runs|-n N] [--no-memory] [--target|-t DIR] [--keep] \"<task description>\"" >&2
    exit 1
fi

TARGET=$(cd "$TARGET" && pwd)  # resolve to absolute path

TASK="$*"
BENCH_TMPDIR=$(mktemp -d)

cleanup() {
    if [[ "$KEEP" == "true" ]]; then
        echo "  Raw outputs kept in: $BENCH_TMPDIR" >&2
    else
        rm -rf "$BENCH_TMPDIR"
    fi
}
trap cleanup EXIT

# ── Tool lists and constraints ─────────────────────────────────────────────────
GLOB_TOOLS="Glob,Grep,Read"

GLOB_CONSTRAINT="You MUST use only Glob and Grep tools for all code exploration. \
Do NOT use any MCP tools (go-llm-lens or otherwise). Do NOT use Bash."

if [[ "$NO_MEMORY" == "true" ]]; then
    LENS_TOOLS="mcp__go-llm-lens__find_symbol,mcp__go-llm-lens__get_function,mcp__go-llm-lens__get_type,mcp__go-llm-lens__find_implementations,mcp__go-llm-lens__get_package_symbols,mcp__go-llm-lens__list_packages,Read"
    LENS_CONSTRAINT="You MUST use only go-llm-lens MCP tools for all code exploration: \
find_symbol, get_function, get_type, find_implementations, get_package_symbols, list_packages. \
Do NOT use Glob, Grep, or Bash."
else
    LENS_TOOLS="mcp__go-llm-lens__find_symbol,mcp__go-llm-lens__get_function,mcp__go-llm-lens__get_type,mcp__go-llm-lens__find_implementations,mcp__go-llm-lens__get_package_symbols,mcp__go-llm-lens__list_packages,mcp__go-llm-lens__write_memory,mcp__go-llm-lens__list_memories,mcp__go-llm-lens__read_memory,mcp__go-llm-lens__delete_memory,Read"
    LENS_CONSTRAINT="You MUST use only go-llm-lens MCP tools for all code exploration: \
find_symbol, get_function, get_type, find_implementations, get_package_symbols, list_packages, \
list_memories, read_memory, write_memory, delete_memory. \
Do NOT use Glob, Grep, or Bash."
fi

# ── run_session <label> <constraint> <allowed_tools> <outfile> ────────────────
run_session() {
    local label="$1"
    local constraint="$2"
    local allowed_tools="$3"
    local outfile="$4"
    local logfile="${outfile%.json}.stderr.log"

    echo "    Running $label..." >&2
    ( cd "$TARGET" && claude -p "${constraint}

Task: ${TASK}" \
        --output-format json \
        --model "$MODEL" \
        --allowedTools "$allowed_tools" ) \
        > "$outfile" 2>"$logfile"

    if [[ ! -s "$outfile" ]]; then
        echo "  ERROR: $label produced no output. Check $logfile" >&2
        return 1
    fi
    if ! jq -e . "$outfile" > /dev/null 2>&1; then
        echo "  ERROR: $label produced invalid JSON. Check $logfile" >&2
        return 1
    fi
    local out_tok
    out_tok=$(jq -r '.usage.output_tokens // 0' "$outfile")
    if [[ "$out_tok" -eq 0 ]]; then
        echo "  WARNING: $label had 0 output tokens — task may not have completed. Check $logfile" >&2
    fi
}

# ── extract_tokens <file> → "input output cache_read cache_creation cost denials" ──
extract_tokens() {
    local file="$1"
    local input output cache_read cache_creation cost denials
    input=$(         jq -r '.usage.input_tokens                 // 0' "$file" 2>/dev/null || echo 0)
    output=$(        jq -r '.usage.output_tokens                // 0' "$file" 2>/dev/null || echo 0)
    cache_read=$(    jq -r '.usage.cache_read_input_tokens      // 0' "$file" 2>/dev/null || echo 0)
    cache_creation=$(jq -r '.usage.cache_creation_input_tokens  // 0' "$file" 2>/dev/null || echo 0)
    cost=$(          jq -r '.total_cost_usd                     // 0' "$file" 2>/dev/null || echo 0)
    denials=$(       jq -r '.permission_denials | length        // 0' "$file" 2>/dev/null || echo 0)
    echo "$input $output $cache_read $cache_creation $cost $denials"
}

# effective = input + output + cache_read×0.1 + cache_creation×1.25
compute_eff() {
    echo "$1 $2 $3 $4" | awk '{printf "%d", $1 + $2 + $3*0.1 + $4*1.25}'
}

# mean_sd [fmt]: reads one number per line from stdin → prints "mean sd"
# fmt defaults to "%.1f" (tokens); pass "%.4f" for costs
mean_sd() {
    local fmt="${1:-%.1f}"
    awk -v fmt="$fmt" \
        'BEGIN { n=0; s=0; ss=0 }
         { n++; s+=$1; ss+=$1*$1 }
         END {
             mean = s / n
             sd   = (n > 1) ? sqrt((ss - s*s/n) / (n-1)) : 0
             printf fmt" "fmt, mean, sd
         }'
}

# ── Cache warmup ───────────────────────────────────────────────────────────────
# Prime the system-prompt cache so neither session pays cache_creation cost
# for the shared Claude Code context, making per-run ordering irrelevant.
echo "" >&2
echo "Model:  $MODEL  |  Target: $TARGET  |  Memory: $( [[ "$NO_MEMORY" == "true" ]] && echo "off" || echo "on" )" >&2
echo "Task:   $TASK" >&2
echo "" >&2
echo "  Warming cache..." >&2
( cd "$TARGET" && claude -p "Say hi." --output-format json --model "$MODEL" ) \
    > "$BENCH_TMPDIR/warmup.json" 2>/dev/null || true

TIMESTAMP=$(date -u '+%Y-%m-%dT%H:%M:%SZ')
g_eff_list=()
l_eff_list=()
g_cost_list=()
l_cost_list=()
# Last-run raw values used for single-run detail display
g_in=0; g_out=0; g_cr=0; g_cc=0; g_cost=0; g_eff=0
l_in=0; l_out=0; l_cr=0; l_cc=0; l_cost=0; l_eff=0

echo "  Runs: $RUNS" >&2

for run in $(seq 1 "$RUNS"); do
    echo "--- Run $run / $RUNS ---" >&2
    g_file="$BENCH_TMPDIR/glob_run${run}.json"
    l_file="$BENCH_TMPDIR/lens_run${run}.json"

    # Randomise execution order to reduce cache warm-up bias
    if (( RANDOM % 2 == 0 )); then
        run_session "Glob/Grep"   "$GLOB_CONSTRAINT" "$GLOB_TOOLS" "$g_file"
        run_session "go-llm-lens" "$LENS_CONSTRAINT" "$LENS_TOOLS" "$l_file"
    else
        run_session "go-llm-lens" "$LENS_CONSTRAINT" "$LENS_TOOLS" "$l_file"
        run_session "Glob/Grep"   "$GLOB_CONSTRAINT" "$GLOB_TOOLS" "$g_file"
    fi

    read -r g_in  g_out  g_cr  g_cc  g_cost  g_deny <<< "$(extract_tokens "$g_file")"
    read -r l_in  l_out  l_cr  l_cc  l_cost  l_deny <<< "$(extract_tokens "$l_file")"

    if [[ "$g_deny" -gt 0 ]]; then
        echo "  ERROR: Glob/Grep had $g_deny permission denial(s) in run $run — benchmark is invalid" >&2
        exit 1
    fi
    if [[ "$l_deny" -gt 0 ]]; then
        echo "  ERROR: go-llm-lens had $l_deny permission denial(s) in run $run — benchmark is invalid" >&2
        exit 1
    fi

    g_eff=$(compute_eff "$g_in" "$g_out" "$g_cr" "$g_cc")
    l_eff=$(compute_eff "$l_in" "$l_out" "$l_cr" "$l_cc")

    g_eff_list+=("$g_eff")
    l_eff_list+=("$l_eff")
    g_cost_list+=("$g_cost")
    l_cost_list+=("$l_cost")

    echo "  Glob/Grep eff=$g_eff cost=\$$g_cost  |  go-llm-lens eff=$l_eff cost=\$$l_cost" >&2
done

# ── Compute aggregate stats ────────────────────────────────────────────────────
read -r g_eff_mean  g_eff_sd  <<< "$(printf '%s\n' "${g_eff_list[@]}"  | mean_sd)"
read -r l_eff_mean  l_eff_sd  <<< "$(printf '%s\n' "${l_eff_list[@]}"  | mean_sd)"
read -r g_cost_mean g_cost_sd <<< "$(printf '%s\n' "${g_cost_list[@]}" | mean_sd "%.4f")"
read -r l_cost_mean l_cost_sd <<< "$(printf '%s\n' "${l_cost_list[@]}" | mean_sd "%.4f")"

# ── Print report ───────────────────────────────────────────────────────────────
echo ""
echo "═══════════════════════════════════════════════════════════"
echo "  Token usage comparison"
printf "  Task:      %s\n" "$TASK"
printf "  Model:     %s\n" "$MODEL"
printf "  Runs:      %s\n" "$RUNS"
printf "  Memory:    %s\n" "$( [[ "$NO_MEMORY" == "true" ]] && echo "off (--no-memory)" || echo "on" )"
printf "  Timestamp: %s\n" "$TIMESTAMP"
echo "═══════════════════════════════════════════════════════════"

if [[ $RUNS -eq 1 ]]; then
    printf "  %-26s %12s %12s\n" "" "Glob/Grep" "go-llm-lens"
    printf "  %-26s %12s %12s\n" "──────────────────────────" "──────────" "───────────"
    printf "  %-26s %12d %12d\n" "Input tokens"              "$g_in"     "$l_in"
    printf "  %-26s %12d %12d\n" "Output tokens"             "$g_out"    "$l_out"
    printf "  %-26s %12d %12d\n" "Cache read tokens"         "$g_cr"     "$l_cr"
    printf "  %-26s %12d %12d\n" "Cache creation tokens"     "$g_cc"     "$l_cc"
    printf "  %-26s %12s %12s\n" "──────────────────────────" "──────────" "───────────"
    printf "  %-26s %12d %12d\n" "Effective tokens*"         "$g_eff"    "$l_eff"
    printf "  %-26s %12s %12s\n" "Cost (USD)"                "\$$g_cost" "\$$l_cost"
else
    printf "  %-30s %20s %20s\n" "" "Glob/Grep" "go-llm-lens"
    printf "  %-30s %20s %20s\n" "──────────────────────────────" "────────────────────" "────────────────────"
    printf "  %-30s %20s %20s\n" "Eff. tokens (mean ± sd)*" \
        "${g_eff_mean} ± ${g_eff_sd}" "${l_eff_mean} ± ${l_eff_sd}"
    printf "  %-30s %20s %20s\n" "Cost USD (mean ± sd)" \
        "\$${g_cost_mean} ± ${g_cost_sd}" "\$${l_cost_mean} ± ${l_cost_sd}"
fi

echo "═══════════════════════════════════════════════════════════"
echo "  * Effective = input + output + cache_read×0.1 + cache_creation×1.25"
echo ""

diff=$(echo "$g_eff_mean $l_eff_mean" | awk '{printf "%d", $1 - $2}')
if [[ "$diff" -gt 0 ]]; then
    pct=$(echo "$diff $g_eff_mean" | awk '{printf "%d", $1 * 100 / ($2 + 1)}')
    echo "  go-llm-lens used fewer effective tokens by ~${diff} (~${pct}%)"
elif [[ "$diff" -lt 0 ]]; then
    neg=$(echo "$diff" | awk '{printf "%d", -$1}')
    pct=$(echo "$neg $l_eff_mean" | awk '{printf "%d", $1 * 100 / ($2 + 1)}')
    echo "  Glob/Grep used fewer effective tokens by ~${neg} (~${pct}%)"
else
    echo "  No difference in effective token usage."
fi
echo ""
