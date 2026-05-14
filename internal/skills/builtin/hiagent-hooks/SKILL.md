---
name: hiagent-hooks
description: Use when the user wants to add, write, debug, or configure a HiAgent hook - gating or blocking tool calls, approving or rewriting tool input before execution, injecting context into tool results, or troubleshooting hook behavior in hiagent.json.
---

# HiAgent Hooks

Hooks are user-defined commands in `hiagent.json` that fire at specific points
during execution, giving deterministic control over tool behavior. They run
before permission checks and only on the top-level agent's tool calls.
Sub-agent calls such as `task` and `agentic_fetch` are not intercepted,
though the sub-agent tool call itself is.

For the full reference, see `docs/hooks/README.md`. This skill covers what you
need to author correct hooks.

## Supported Events

Only `PreToolUse` is currently supported. Event names are case-insensitive and
accept snake_case (`PreToolUse`, `pretooluse`, `pre_tool_use` all work).

## Configuration

```jsonc
{
  "hooks": {
    "PreToolUse": [
      {
        "matcher": "^bash$",
        "command": "./hooks/my-hook.sh",
        "timeout": 10
      }
    ]
  }
}
```

Project-level hooks take precedence over global. Matching hooks are deduped by
`command`, run in parallel, and aggregated in config order rather than finish order.

## Language

`command` is a shell command, so hooks can be written in any language by
invoking the interpreter: `node ./hooks/h.js`, `python3 ./hooks/h.py`,
`./hooks/h.sh`, inline `echo '...'`, and so on. The rest of this skill shows
bash, but the input and output contract is identical regardless of language.

## Input

Environment variables:

| Variable                     | Description                          |
| ---------------------------- | ------------------------------------ |
| `HIAGENT_EVENT`                | Event name such as `PreToolUse`      |
| `HIAGENT_TOOL_NAME`            | Tool being called such as `bash`     |
| `HIAGENT_SESSION_ID`           | Current session ID                   |
| `HIAGENT_CWD`                  | Working directory                    |
| `HIAGENT_PROJECT_DIR`          | Project root directory               |
| `HIAGENT_TOOL_INPUT_COMMAND`   | For `bash` calls: the shell command  |
| `HIAGENT_TOOL_INPUT_FILE_PATH` | For file tools: the target file path |

JSON on stdin:

```json
{
  "event": "PreToolUse",
  "session_id": "313909e",
  "cwd": "/home/user/project",
  "tool_name": "bash",
  "tool_input": {"command": "rm -rf /"}
}
```

## Output

Communicate back via exit code with stderr, or JSON on stdout.

| Exit Code | Meaning                                                   |
| --------- | --------------------------------------------------------- |
| 0         | Success. Stdout is parsed as the JSON envelope below.     |
| 2         | Block this tool call. Stderr becomes the deny reason.     |
| 49        | Halt the whole turn. Stderr becomes the halt reason.      |
| Other     | Non-blocking error. Logged and ignored; tool call proceeds. |

Exit `2` blocks one tool call, while exit `49` ends the whole turn. Default to
deny; only use halt when retrying itself would be a problem.

JSON envelope on exit `0`:

```json
{
  "version": 1,
  "decision": "allow",
  "halt": false,
  "reason": "...",
  "context": "Extra info for the model",
  "updated_input": {"command": "rewritten"}
}
```

- `decision`: `"allow"`, `"deny"`, or omitted. `"allow"` is affirmative pre-approval and bypasses the permission prompt.
- `halt: true`: ends the turn, same effect as exit `49`.
- `reason`: shown to the model on deny, and to both model and user on halt.
- `context`: string or array of strings appended to what the model sees.
- `updated_input`: shallow-merge patch against `tool_input`, not a full replacement.

## Aggregation (Multiple Hooks)

Composed in config order:

- `deny` beats `allow`, which beats no opinion.
- `halt` is sticky: any hook halting ends the turn.
- `reason` and `context` are concatenated in config order.
- `updated_input` patches are shallow-merged in order; later patches win on key collisions.

## Canonical Examples

### Block destructive commands

```bash
#!/usr/bin/env bash
set -euo pipefail

if echo "$HIAGENT_TOOL_INPUT_COMMAND" | grep -qE 'rm\s+-(rf|fr)\s+/'; then
  echo "Refusing to run rm -rf against root" >&2
  exit 2
fi
```

Config:

```json
{"matcher": "^bash$", "command": "./hooks/no-rm-rf.sh"}
```

### Auto-approve read-only tools

```jsonc
{"matcher": "^(view|ls|grep|glob)$", "command": "echo '{\"decision\":\"allow\"}'"}
```

### Inject context without auto-approving

Emit only `context`; omit `decision` so the normal permission flow still runs.

```bash
#!/usr/bin/env bash
set -euo pipefail

if [[ "$HIAGENT_TOOL_INPUT_FILE_PATH" == *.go ]]; then
  echo '{"context": "Remember: run gofumpt after editing Go files."}'
else
  echo '{}'
fi
```

Config:

```json
{"matcher": "^(edit|write|multiedit)$", "command": "./hooks/go-context.sh"}
```

### Rewrite tool input

```bash
#!/usr/bin/env bash
set -euo pipefail

read -r input
rewritten=$(echo "$input" | jq -r '.tool_input.command' | some-rewriter)

cat <<EOF
{
  "context": "Rewrote command",
  "updated_input": {"command": "$rewritten"}
}
EOF
```

If the original call was `{"command": "npm test", "timeout": 60000}`, the tool
runs with the rewritten command and the original `timeout` is preserved.

## Authoring Checklist

1. Add `#!/usr/bin/env bash` and `set -euo pipefail` for shell scripts.
2. `chmod +x` the script.
3. Add the entry under `hooks.PreToolUse` in `hiagent.json` with the right matcher.
4. Decide intent: inject context, auto-approve, block, or halt.
5. If rewriting input, remember `updated_input` is a shallow merge, so include only the keys you want to change.

## Debugging

- Timeouts kill the hook silently and the tool call proceeds. Increase `timeout` if needed.
- Non-zero exit codes other than `2` and `49` are logged but do not block.
- Use `echo "debug info" >&2` for logging without corrupting stdout JSON.
- `matcher` is a regex against the tool name. Use `^bash$` if you want an exact match.

## Claude Code Compatibility

HiAgent also accepts Claude Code's `hookSpecificOutput` envelope. One intentional
difference is that HiAgent treats `updated_input` as shallow-merge, while Claude
Code replaces the full tool input. Existing Claude Code hooks should still work
for matcher and decision behavior.

