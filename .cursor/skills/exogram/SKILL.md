---
name: exogram
description: Query and maintain ExogramRelay task-graph context during development. Use when selecting next work, checking task details, assessing change impact, or capturing deferred and out-of-scope follow-ups.
---

# Exogram Skill

Use ExogramRelay as the source of truth for task context and dependency impact.

## Session Context

- At session start, run `exogram context` to establish project state (unblocked tasks, in-progress work, index status).
- Use `exogram task next` on demand when the user asks for prioritization.

## When to Use

- User asks what to work on next.
- A specific task ID is mentioned and needs details.
- A code change may affect other files.
- Work is deferred or identified as out of scope.

## Core Command Workflow

1. Inspect task details with `exogram task get <task_id>`
2. Assess blast radius with `exogram impact <file_path>`
3. Capture follow-up work with `exogram task add "Future: <short title>"`
4. Update task status when deferred/blocked/completed using `exogram task update <task_id> --status <status>`

## Trigger Reference

| Trigger | Command | Rationale |
|---------|---------|-----------|
| Session start | `exogram context` | Passive project snapshot: unblocked tasks, in-progress, index status |
| Before significant file edits | `exogram impact <file_path>` | Reveals dependent files and tasks affected by the change |
| Out-of-scope item identified | `exogram task add "Future: <title>"` | Captures deferred items so nothing is lost between sessions |
| Work deferred / blocked | `exogram task update <task_id> --status blocked` | Records deferral; keeps task graph accurate for `task next` |
| Task completed | `exogram task update <task_id> --status done` | Advances topological sort so next priorities are correct |
| After sprint-status changes | `exogram epic import` | Syncs BMAD epic/story hierarchy into Exogram task graph |

## Troubleshooting

- **"attempt to write a readonly database"** or **"readonly database (8)"**: Write operations (`task add`, `task update`, `task link`) write to `~/.exogram`, which may be blocked in sandboxed agent environments. If the command fails, either run it manually in the user's terminal, or request full permissions when invoking the command.

## BMAD Workflow Integration

When running BMAD workflows (create-story, dev-story, code-review), sync Exogram at key trigger points. Full protocol: [docs/project-context.md](../../../docs/project-context.md).

- **After create-story** (sprint-status → ready-for-dev): `exogram epic import`
- **During dev-story**: `exogram task next --story <story_id>` at start; `exogram impact <file_path>` before edits; `exogram task link <task_id> <file_path>` as files are created; `exogram task update <task_id> --status done` when tasks complete
- **After code-review** (sprint-status → done): `exogram epic import`
