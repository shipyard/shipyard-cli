#!/usr/bin/env bash
#
# Smoke test for `shipyard mcp serve`.
# Drives the server over stdio the way Claude Code, Cursor and Codex do, and
# validates the handshake without any network access.
#
# Every check here stands for a way the server can look broken to an editor
# while the CLI itself works fine: anything but JSON-RPC on stdout, a request
# dropped when the client closes stdin, a process that outlives its client, or a
# base-protocol method answered with an error.
#
# Usage: ./scripts/mcp-smoke-test.sh <path-to-binary>
#
set -uo pipefail

BINARY="${1:-}"
if [[ -z "$BINARY" ]]; then
    echo "Usage: $0 <path-to-binary>"
    exit 1
fi

if [[ ! -x "$BINARY" ]]; then
    echo "FAIL: $BINARY is not executable or does not exist"
    exit 1
fi

if ! command -v python3 >/dev/null 2>&1; then
    echo "FAIL: python3 is required to check the JSON-RPC responses"
    exit 1
fi

PASS=0
FAIL=0

pass() {
    echo "PASS: $1"
    PASS=$((PASS + 1))
}

fail() {
    echo "FAIL: $1"
    [[ -n "${2:-}" ]] && echo "  got: $2"
    FAIL=$((FAIL + 1))
}

# run_timeout <seconds> <command...>
#
# `timeout` is GNU coreutils and is not on the macOS runners; coreutils installs
# it as gtimeout when it is there at all. perl is on every runner image, and
# alarm(2) survives exec, so it is the portable last resort. Set
# MCP_SMOKE_TIMEOUT_IMPL to timeout, gtimeout or perl to pin one (used by this
# script's own tests).
run_timeout() {
    local secs="$1"
    shift
    local impl="${MCP_SMOKE_TIMEOUT_IMPL:-}"

    if [[ -z "$impl" ]]; then
        if command -v timeout >/dev/null 2>&1; then
            impl=timeout
        elif command -v gtimeout >/dev/null 2>&1; then
            impl=gtimeout
        else
            impl=perl
        fi
    fi

    case "$impl" in
        timeout|gtimeout) "$impl" "$secs" "$@" ;;
        perl) perl -e 'alarm shift; exec @ARGV or die "exec: $!\n"' "$secs" "$@" ;;
        *) echo "unknown MCP_SMOKE_TIMEOUT_IMPL: $impl" >&2; return 2 ;;
    esac
}

# timed_out <exit-code>: 124 is timeout(1), 142 is SIGALRM from the perl path.
timed_out() {
    [[ "$1" == "124" || "$1" == "142" ]]
}

INIT='{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"smoke-test","version":"1"}}}'

echo "=== Shipyard MCP Smoke Tests ==="
echo "Binary: $BINARY"
echo ""

# 1. A first run must not open the stream with the config-creation notice: the
#    client parses stdout as JSON-RPC and fails on anything else.
TEMP_HOME=$(mktemp -d)
FIRST_LINE=$(echo "$INIT" | run_timeout 20 env HOME="$TEMP_HOME" SHIPYARD_API_TOKEN=dummy "$BINARY" mcp serve 2>/dev/null | head -1)
rm -rf "$TEMP_HOME"
if [[ "$FIRST_LINE" == '{"jsonrpc"'* ]]; then
    pass "fresh install: first stdout line is the JSON response"
else
    fail "fresh install: first stdout line is the JSON response" "${FIRST_LINE:0:80}"
fi

# 2. Verbose logging belongs on stderr for the same reason.
STRAY=$(echo "$INIT" | run_timeout 20 "$BINARY" mcp serve -v 2>/dev/null | grep -cv '^{')
if [[ "$STRAY" == "0" ]]; then
    pass "verbose mode: stdout carries only JSON"
else
    fail "verbose mode: stdout carries only JSON" "$STRAY non-JSON line(s)"
fi

# 3. A client that closes stdin right after writing must still get its answer.
ANSWERED=0
for _ in $(seq 1 12); do
    BYTES=$(echo "$INIT" | run_timeout 15 "$BINARY" mcp serve 2>/dev/null | wc -c | tr -d ' ')
    [[ "$BYTES" != "0" ]] && ANSWERED=$((ANSWERED + 1))
done
if [[ "$ANSWERED" == "12" ]]; then
    pass "stdin closed early: 12/12 requests answered"
else
    fail "stdin closed early: 12/12 requests answered" "$ANSWERED/12"
fi

# 4. A disconnected client must end the process, or editors that restart their
#    MCP servers leak one per restart.
echo "$INIT" | run_timeout 15 "$BINARY" mcp serve >/dev/null 2>&1
EXIT_CODE=$?
if timed_out "$EXIT_CODE"; then
    fail "client disconnect: process exits" "still running when the timeout fired"
else
    pass "client disconnect: process exits"
fi

# 5. A full session: handshake, base protocol, and every advertised capability.
SESSION=$( { echo "$INIT"
    echo '{"jsonrpc":"2.0","method":"notifications/initialized"}'
    echo '{"jsonrpc":"2.0","id":2,"method":"ping"}'
    echo '{"jsonrpc":"2.0","id":3,"method":"prompts/list"}'
    echo '{"jsonrpc":"2.0","id":4,"method":"prompts/get","params":{"name":"shipyard_verify","arguments":{}}}'
    echo '{"jsonrpc":"2.0","id":5,"method":"tools/list"}'
    echo '{"jsonrpc":"2.0","id":6,"method":"resources/list"}'; } | run_timeout 40 "$BINARY" mcp serve 2>/dev/null )

SESSION_RESULT=$(printf '%s' "$SESSION" | python3 -c '
import sys, json

seen = {}
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    try:
        msg = json.loads(line)
    except ValueError:
        print("FAIL\tsession: stdout is valid JSON-RPC\t" + line[:80])
        continue
    seen[msg.get("id")] = msg

def check(ok, name, detail=""):
    print(("PASS\t" if ok else "FAIL\t") + name + "\t" + ("" if ok else detail))

# A notification carries no id and must never be answered, not even with an error.
check(sorted(k for k in seen if k is not None) == [1, 2, 3, 4, 5, 6] and None not in seen,
      "session: every request answered, notification ignored",
      "ids seen: " + str(sorted(str(k) for k in seen)))

init = seen.get(1, {}).get("result", {})
check(init.get("protocolVersion") == "2025-06-18",
      "initialize: negotiates 2025-06-18", str(init.get("protocolVersion")))
check(bool(init.get("instructions")), "initialize: sends server instructions")
check(set(init.get("capabilities", {})) == {"tools", "resources", "prompts"},
      "initialize: advertises tools, resources and prompts",
      str(sorted(init.get("capabilities", {}))))

ping = seen.get(2, {})
check("result" in ping and ping.get("error") is None,
      "ping: answered without an error", str(ping.get("error")))

names = [p.get("name") for p in seen.get(3, {}).get("result", {}).get("prompts", [])]
check("shipyard_verify" in names, "prompts/list: includes shipyard_verify", str(names))

messages = seen.get(4, {}).get("result", {}).get("messages", [])
text = messages[0].get("content", {}).get("text", "") if messages else ""
check(len(text) > 1000, "prompts/get: returns the verify instructions",
      "%d chars" % len(text))

tools = [t.get("name") for t in seen.get(5, {}).get("result", {}).get("tools", [])]
check(len(tools) >= 15 and "get_environments" in tools and "get_logs" in tools,
      "tools/list: returns the tool set", "%d tools" % len(tools))

resources = seen.get(6, {}).get("result", {}).get("resources", [])
check(len(resources) >= 1, "resources/list: returns at least one resource",
      "%d resources" % len(resources))
')

while IFS=$'\t' read -r verdict name detail; do
    [[ -z "$verdict" ]] && continue
    if [[ "$verdict" == "PASS" ]]; then
        pass "$name"
    else
        fail "$name" "$detail"
    fi
done <<< "$SESSION_RESULT"

# 6. Version negotiation: supported revisions echo back, anything else falls
#    back to the newest this server speaks rather than going silent.
negotiated() {
    echo "{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\",\"params\":{\"protocolVersion\":\"$1\",\"capabilities\":{},\"clientInfo\":{\"name\":\"smoke-test\",\"version\":\"1\"}}}" \
        | run_timeout 20 "$BINARY" mcp serve 2>/dev/null \
        | python3 -c 'import sys, json
line = sys.stdin.readline()
try:
    print(json.loads(line)["result"]["protocolVersion"])
except Exception:
    print("<no response>")'
}

for VERSION in 2025-06-18 2025-03-26 2024-11-05; do
    GOT=$(negotiated "$VERSION")
    if [[ "$GOT" == "$VERSION" ]]; then
        pass "negotiation: $VERSION echoed back"
    else
        fail "negotiation: $VERSION echoed back" "$GOT"
    fi
done

GOT=$(negotiated "2099-01-01")
if [[ "$GOT" == "2025-06-18" ]]; then
    pass "negotiation: unknown revision falls back to the latest"
else
    fail "negotiation: unknown revision falls back to the latest" "$GOT"
fi

# 7. With no token a tool call must return an actionable JSON-RPC error rather
#    than hanging: an agent can act on the message, it cannot act on a stall.
EMPTY_HOME=$(mktemp -d)
TOKEN_ERROR=$( { echo "$INIT"
    echo '{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"get_environments","arguments":{}}}'; } \
    | run_timeout 60 env -u SHIPYARD_API_TOKEN -u SHIPYARD_ORG HOME="$EMPTY_HOME" "$BINARY" mcp serve 2>/dev/null \
    | python3 -c 'import sys, json
for line in sys.stdin:
    if not line.strip():
        continue
    msg = json.loads(line)
    if msg.get("id") == 2:
        print((msg.get("error") or {}).get("message", "<no error field>"))' )
rm -rf "$EMPTY_HOME"
if [[ "$TOKEN_ERROR" == *"token is missing"* ]]; then
    pass "missing token: actionable error, no hang"
else
    fail "missing token: actionable error, no hang" "${TOKEN_ERROR:0:120}"
fi

echo ""
echo "=== Results: $PASS passed, $FAIL failed ==="

if [[ "$FAIL" -gt 0 ]]; then
    exit 1
fi
