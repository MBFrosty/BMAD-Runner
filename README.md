# BMAD Runner

Orchestrate BMAD workflow phases (`create-story` → `dev-story` → `code-review` → `retrospective`) using AI agent CLIs like `cursor-agent`, `claude-code`, or `gemini-cli`.

## Features

- **Multi-Agent Support**: Run workflows with Cursor, Claude Code, or Gemini CLI.
- **Workflow Orchestration**: Supports individual phases (`create-story`, `dev-story`, `code-review`) and automated full-pipeline execution.
- **Retrospectives**: Automatically runs an epic retrospective when all stories in an epic are completed.
- **Auto-Looping**: Continually works through pending stories and epics until all work is done.
- **Automated Epic Planning**: When all stories are complete, automatically plans new epics (hardening, features, tech debt) guided by a project-level **prime directive** you edit. Up to 5 new epics are staged into `sprint-status.yaml` so the loop can continue without manual intervention.
- **Stall Detection**: Detects if the agent fails to update the sprint status and safely halts to prevent infinite loops.
- **Smart Model Defaults**: Uses different models optimized for different phases of work (e.g., cheaper/faster models for typical dev loops, smarter models for reviews and planning).

## Prerequisites

- **Go 1.24+** — [Install Go](https://go.dev/doc/install)
- **One or more agent backends** (in your PATH or via `--agent-path`):
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
go run ./cmd/bmad-runner run auto
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

### Run individual phases
```bash
./bin/bmad-runner run create-story
./bin/bmad-runner run dev-story
./bin/bmad-runner run code-review
```

### Run next pending task 
```bash
./bin/bmad-runner run
```
Runs the next available workflow phase based on the current `sprint-status.yaml`.

### Run auto (loop through all pending stories and epics)
```bash
./bin/bmad-runner run auto
```
Runs the full pipeline (`create-story` → `dev-story` → `code-review`) for each pending story, triggers `retrospective` when an epic completes, and stops when all work is done. Use `--max-iterations` to set a safety limit (default 50).

- **Between stories**: Prints "Story X complete — continuing to next" and continues automatically.
- **Stall handling**: If `sprint-status.yaml` is unchanged after a story (workflow didn't update it), the runner warns and continues. After 2 consecutive stalls for the same story, it exits. Use `--ignore-stall` to never exit on stall.
- **After retrospective**: Prompts "Press Enter to continue to next epic" (interactive terminal only). Use `--no-pause-after-retro` for scripts/CI to skip the prompt.

### Run auto with automated epic planning
```bash
./bin/bmad-runner run auto --enable-epic-planning
```
When all current stories are complete, instead of stopping the runner will automatically plan new epics and stage them into `sprint-status.yaml` so development can continue.

**How it works:**

1. When `NextWork()` finds no remaining stories, the runner checks for a **prime directive** file at `_bmad-output/prime-directive.md` (relative to project root).
2. If the file doesn't exist, it's **created with a default template** — the runner then exits so you can review and fill it in.
3. On the next run, the runner passes the prime directive to the agent, which:
   - Analyzes existing project artifacts (PRD, architecture, epics, retrospective notes)
   - Generates up to 5 new epics focused on hardening, new features, technical debt, performance, and testing
   - Writes a new epics document (`_bmad-output/planning-artifacts/epics-phase-N.md`)
   - Appends the new epics and stories to `sprint-status.yaml`
4. The auto loop continues working through the newly planned stories.

**Prime Directive (`_bmad-output/prime-directive.md`):**

This file is your strategic guide to the AI planner. Edit it to describe:
- Your project's vision and north star
- Current focus areas and priorities
- Constraints and guardrails
- Specific goals for the next development phase

The richer and more specific this file, the more aligned the generated epics will be with your intentions.

**Options for `run auto --enable-epic-planning`:**

| Flag | Description | Default |
|------|-------------|---------|
| `--enable-epic-planning` | Enable automated epic planning when no work remains | off |
| `--prime-directive <path>` | Path to the prime directive file | `<project-root>/_bmad-output/prime-directive.md` |
| `--max-new-epics <N>` | Maximum number of new epics to generate | `5` |

### Plan epics manually
```bash
./bin/bmad-runner run plan-epics
```
Runs the epic planning phase on its own — useful when you want to plan new work without triggering a full auto run. Respects `--prime-directive` and `--max-new-epics`.

## Configuration Options

You can customize the runner using global flags (works with any run method):

- `--status-file`: Path to `sprint-status.yaml` (default: `_bmad-output/implementation-artifacts/sprint-status.yaml`)
- `--project-root`: Root of the project to run workflows in
- `--no-live-status`: Disable last-lines display in spinner (e.g., for CI/scripts). Live status is also disabled when stdout is not a TTY.

### Selecting Your Agent Type
Use the `--agent-type` (or `-t`) flag to specify which agent CLI backend to use.
Valid options are:
- `cursor-agent` (default)
- `claude-code`
- `gemini-cli`

If the agent is not in your PATH, you can explicitly set its location using `--agent-path` (or `-a`).

### Models & Defaults
Use the `--model` (or `-m`) flag to override the model used by the agent. If you don't provide a model, the runner intelligently selects the best default model based on the chosen agent and the current workflow phase.

**Default Models by Agent and Phase:**

| Agent | `create-story` | `dev-story` | `code-review` / `retrospective` | `plan-epics` | Fallback / Default |
|-------|----------------|-------------|---------------------------------|--------------|---------------------|
| **`cursor-agent`** | `claude-4.6-sonnet-medium` | `composer-1.5` | `gemini-3-flash` | `claude-4.6-sonnet-medium` | `composer-1.5` |
| **`claude-code`**| `opus` | `haiku` | `sonnet` | `opus` | `sonnet` |
| **`gemini-cli`** | `gemini-3-pro` | `gemini-3-flash`| `gemini-3-pro` | `gemini-3-pro` | `gemini-3-pro` |

### Examples

```bash
# Use cursor-agent (default) with its default models for the phase
./bin/bmad-runner run auto

# Use cursor-agent with a custom model override for all phases
./bin/bmad-runner --model sonnet-4-thinking run auto

# Use claude-code with its smart default models based on the phase
./bin/bmad-runner --agent-type claude-code run auto

# Use claude-code explicitly with sonnet for a specific phase
./bin/bmad-runner --agent-type claude-code --model sonnet run code-review

# Use gemini-cli with its default phase-specific models
./bin/bmad-runner --agent-type gemini-cli run auto

# Enable automated epic planning when all stories are done
./bin/bmad-runner --agent-type claude-code run auto --enable-epic-planning

# Cap the planner at 3 epics and use a custom prime directive location
./bin/bmad-runner run auto --enable-epic-planning --max-new-epics 3 \
  --prime-directive /path/to/my-goals.md

# Plan new epics manually (standalone, no auto loop)
./bin/bmad-runner --agent-type claude-code run plan-epics
```
