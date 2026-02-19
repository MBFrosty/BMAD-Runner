# BMAD Runner

Orchestrate BMAD workflow phases (create-story → dev-story → code-review) using `cursor-agent`, `claude-code`, or `gemini-cli`.

## Prerequisites

- **Go 1.24+** — [Install Go](https://go.dev/doc/install)
- **One agent backend** (in your PATH or via `--agent-path`):
  - `cursor-agent` CLI (usually in `~/.local/bin/agent`)
  - `claude` CLI ([Claude Code](https://code.claude.com))
  - `gemini` CLI ([Gemini CLI](https://github.com/google-gemini/gemini-cli))

## Running

### Option 1: Run from GitHub (no install)

Like `npx` or `uvx` — Go downloads and runs the tool without installing it:

```bash
go run github.com/MBFrosty/BMAD-Runner/cmd/bmad-runner@latest status
go run github.com/MBFrosty/BMAD-Runner/cmd/bmad-runner@latest run create-story
```

- `@latest` — latest tagged release
- `@main` — latest from main branch
- `@v1.0.0` — specific version (when tagged)

### Option 2: Run locally (development)

From the repo root:

```bash
go run ./cmd/bmad-runner status
go run ./cmd/bmad-runner run create-story
```

### Option 3: Build and install

```bash
cd BMAD-Runner
go build -o bin/bmad-runner ./cmd/bmad-runner
```

Then run `./bin/bmad-runner` or add `bin/` to your PATH.

## Usage

### Show current sprint status
```bash
./bin/bmad-runner status
```

### Run full pipeline
```bash
./bin/bmad-runner run
```

### Run individual phases
```bash
./bin/bmad-runner run create-story
./bin/bmad-runner run dev-story
./bin/bmad-runner run code-review
```

### Run auto (loop through all pending stories and epics)
```bash
./bin/bmad-runner run auto
```
Runs the full pipeline (create-story → dev-story → code-review) for each pending story, triggers retrospective when an epic completes, and stops when all work is done. Use `--max-iterations` to set a safety limit (default 50).

- **Between stories**: Prints "Story X complete — continuing to next" and continues automatically.
- **Stall handling**: If `sprint-status.yaml` is unchanged after a story (workflow didn't update it), the runner warns and continues. After 2 consecutive stalls for the same story, it exits. Use `--ignore-stall` to never exit on stall.
- **After retrospective**: Prompts "Press Enter to continue to next epic" (interactive terminal only). Use `--no-pause-after-retro` for scripts/CI to skip the prompt.

### Configuration

You can customize the runner using global flags (works with any run method — `go run`, built binary, or from GitHub):
- `--status-file`: Path to `sprint-status.yaml`
- `--agent-path`: Path to agent binary (lookup in PATH by default)
- `--agent-type`: Agent backend — `cursor-agent` (default), `claude-code`, or `gemini-cli`
- `--project-root`: Root of the project to run workflows in
- `--model`: Model to use (default: `composer-1.5` for cursor-agent, `sonnet` for claude-code, `gemini-3-pro-preview` for gemini-cli)
- `--no-live-status`: Disable last-lines display in spinner (e.g. for CI/scripts). Live status is also disabled when stdout is not a TTY.

Examples:
```bash
# Use cursor-agent with custom model
./bin/bmad-runner --model sonnet-4-thinking run dev-story

# Use claude-code (default model: sonnet)
./bin/bmad-runner --agent-type claude-code run dev-story

# Use claude-code with opus
./bin/bmad-runner --agent-type claude-code --model opus run dev-story

# Use gemini-cli (default model: gemini-3-pro-preview)
./bin/bmad-runner --agent-type gemini-cli run dev-story

# Use gemini-cli with flash
./bin/bmad-runner --agent-type gemini-cli --model gemini-2.5-flash run dev-story
```
