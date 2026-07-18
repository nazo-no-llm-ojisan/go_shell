# go_shell

A small multi-OS command runtime for coding agents and humans.

`go_shell` separates **intent**, **OS-specific command mapping**, **argument translation**, and **process execution** into four distinct layers. On Windows it uses PowerShell 7 (`pwsh`) by default — **not** Git Bash.

Status: experimental / pre-1.0.

## Why

Most coding agents assume a POSIX shell. On Windows this leads to one of two bad outcomes:

1. The agent invokes `bash` / `sh` directly, triggering Git Bash with its quoting, encoding, and path-translation quirks.
2. The agent writes PowerShell cmdlets by hand, mixing OS and shell concerns, and hallucinates parameters.

`go_shell` gives agents (and humans) a single small vocabulary that hides both problems:

- The agent writes `go_shell -win -ls -a`, never `Get-ChildItem -Force`.
- The agent writes `go_shell -win git status`, never `pwsh -Command "git status"`.
- Unknown commands pass through unchanged to the OS-appropriate backend.
- Unknown **functions** are rejected — no silent shell fallback for hallucinated calls.

## Install

```sh
go install github.com/nazo-no-llm-ojisan/go_shell@latest
```

Or build from source:

```sh
git clone https://github.com/nazo-no-llm-ojisan/go_shell
cd go_shell
go build -o go_shell .
```

Requires Go 1.21+ and, on Windows, [PowerShell 7](https://github.com/PowerShell/PowerShell) (`pwsh`).

## Usage

Two invocation shapes:

```text
go_shell [meta...] -<os> -<command> [args...]   # OS mode
go_shell [meta...] -<function> [args...]         # function mode
```

### OS mode

The first `-` token selects the OS direction. Mapped commands are translated; unmapped commands pass straight through to the OS backend.

```sh
go_shell -win -ls -a                # → Get-ChildItem -Force
go_shell -win -mkdir output         # → New-Item -ItemType Directory output
go_shell -win -rm -r old_dir        # → Remove-Item -Recurse old_dir (requires --yes)
go_shell -win git status            # passthrough: pwsh -Command "git status"
go_shell -win dotnet test           # passthrough
go_shell -linux -ls -la             # → ls -la
go_shell -macos -cat file.txt       # → cat file.txt
```

### Function mode

If the first token is not an OS specifier, it is treated as a registered function name. Functions run inside the Go process — no shell is involved.

```sh
go_shell -write_file path.txt "content"
go_shell -copy_file src.txt dst.txt
go_shell -create_hermes_subagent nemo COMMAND.md   # stub
```

Unknown function names produce an error rather than being forwarded to the shell:

```text
go_shell: unknown function: -create_nonexistent
```

### Meta flags

| Flag | Purpose |
|---|---|
| `--json` | Emit structured result JSON instead of raw stdout/stderr |
| `--yes` | Allow destructive operations (`rm`, `rmdir`) |
| `--cwd DIR` | Set working directory for the command |
| `--timeout DUR` | Timeout (e.g. `30s`, `2m`). Default 60s. Exit 124 on timeout. |
| `--env K=V` | Override an environment variable (repeatable) |
| `--allow-windows-powershell` | Permit fallback to Windows PowerShell 5.1 if `pwsh` is absent |

### JSON result

With `--json`, the result is always a single JSON object:

```json
{
  "ok": true,
  "exit_code": 0,
  "stdout": "...",
  "stderr": "",
  "backend": "pwsh",
  "os_mode": "win",
  "resolved_command": "Get-ChildItem -Force",
  "duration": "927.8127ms"
}
```

Command stdout and stderr are captured up to 16 MiB per stream. If either
limit is exceeded, the retained output is returned and the JSON result sets
`stdout_truncated` or `stderr_truncated` to `true`. Non-JSON mode prints an
equivalent warning to stderr.

Destructive operations without `--yes` return a `dry_run` result instead of executing:

```json
{
  "ok": true,
  "exit_code": 0,
  "stdout": "[dry-run] destructive operation blocked without --yes\n  resolved: Remove-Item -Recurse testrm\n  args: [-Recurse testrm]\n",
  "dry_run": true
}
```

## Architecture

Four layers, in order:

| Layer | File | Responsibility |
|---|---|---|
| 0 — OS | `os_layer.go` | Pick OS direction and execution backend (`pwsh` / `sh` / `zsh` / `wsl` / `native`) |
| 1 — Command | `command_layer.go` | Map abstract commands (`ls`, `rm`, …) to OS-specific equivalents; passthrough if unmapped |
| 2 — Argument | `arg_layer.go` | Translate flags for mapped commands only; **passthrough args are never modified** |
| 3 — Execution | `exec_layer.go` | Run via the chosen backend with timeout, stdout/stderr separation, UTF-8 forcing, and execution log |

Function calls bypass the pipeline and run Go code directly (`func_layer.go`).

### Backends

| OS | Backend | Shell |
|---|---|---|
| `-win` | `pwsh` | PowerShell 7 (fallback to 5.1 only with `--allow-windows-powershell`) |
| `-linux` | `sh` | `/bin/sh -c` |
| `-macos` | `zsh` | `/bin/zsh -c` |
| `-wsl` | `wsl` | `wsl.exe -e sh -c` |
| `-native` | `native` | direct `exec`, no shell |
| `-auto` | — | resolves from `runtime.GOOS` |

Git Bash is **never** invoked.

## Security

`go_shell` executes local commands and may modify or delete files. Treat any agent that can invoke `go_shell` as having local execution capability.

- `rm` and `rmdir` require `--yes`. This is a **UX guard against accidental mapped-command misuse, not a security boundary or sandbox.** Passthrough commands (`Remove-Item`, `cmd /c del`, `sh -c "rm -rf ..."`) are not intercepted.
- Windows mapped commands treat user-supplied path operands literally. Wildcard-like values such as `[abc].txt` are passed through `-LiteralPath` where the PowerShell cmdlet supports it; passthrough commands retain their native PowerShell semantics.
- Do not expose `go_shell` directly to untrusted remote input.
- Do not pass secrets as command-line arguments — they are visible in process listings and in `~/.go_shell/log.jsonl`.
- Prefer environment variables (`--env`) or explicit secret-management integration.
- Unimplemented registered functions return exit code 78 (failure), never a false positive.

Every execution is appended to `~/.go_shell/log.jsonl` with timestamp, resolved command, backend, exit code, and output sizes.

## Agent integration

A minimal spec to give to a coding agent:

```md
Use go_shell for all host command execution.

Windows examples:
  go_shell -win git status
  go_shell -win -ls -a
  go_shell -win -mkdir output
  go_shell -win dotnet test

Do not invoke Git Bash, bash, sh, cmd.exe, or pwsh.exe directly.
Do not use Bash syntax.
Use registered go_shell functions (-write_file, -copy_file, etc.) for agent operations.
Unknown function names will be rejected.
```

## License

MIT. See [LICENSE](LICENSE).
