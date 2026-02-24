---
description: ExogramRelay context-fabric usage for autonomous agent workflows
alwaysApply: true
---

# ExogramRelay Agent Protocol

Use ExogramRelay to query project context and maintain the task graph during development.

## Session Context

- At session start, run `exogram context` to establish project state (unblocked tasks, in-progress work, index status).
- Use `exogram task next` on demand when the user asks for prioritization.

## Task-Aware Context (On-Demand)

- Use `exogram task get <task_id>` when a task needs clarification.
- Use `exogram impact <file_path>` when assessing dependency impact.
- Do not run `exogram task next` automatically; use it when the user asks for reprioritization.

## Context Gardener (Proactive Graph Maintenance)

- When work is out of scope, suggest and capture follow-up with:
  - `exogram task add "Future: <short title>"`
- When you introduce technical debt (`TODO`, `FIXME`, temporary workaround), create a task immediately.
- When the user defers work ("later"), create a placeholder task that records the deferral context.

## Trigger Reference

| Trigger | Command | Rationale |
|---------|---------|-----------|
| Session start | `exogram context` | Passive project snapshot: unblocked tasks, in-progress, index status |
| Before significant file edits | `exogram impact <file_path>` | Reveals dependent files and tasks affected by the change |
| Out-of-scope item identified | `exogram task add "Future: <title>"` | Captures deferred items so nothing is lost between sessions |
| Work deferred / blocked | `exogram task update <task_id> --status blocked` | Records deferral; keeps task graph accurate for `task next` |
| Task completed | `exogram task update <task_id> --status done` | Advances topological sort so next priorities are correct |
| After sprint-status changes | `exogram epic import` | Syncs BMAD epic/story hierarchy into Exogram task graph |

## User Communication

- Periodically provide short user-facing summaries of tasks created/updated and why.
- When task state changes matter, explicitly mention the impacted task IDs and statuses.

## Troubleshooting

- If `exogram task add` or `exogram task update` fails with "readonly database", the sandbox may block writes to `~/.exogram`. Run the command in the user's terminal or request full permissions.

## BMAD Workflow Integration

When running BMAD workflows (create-story, dev-story, code-review), sync Exogram at each trigger point. Full protocol: [docs/project-context.md](../../docs/project-context.md).

| BMAD Trigger | Exogram Command | Rationale |
|---|---|---|
| After create-story (sprint-status → ready-for-dev) | `exogram epic import` | Syncs story into task graph |
| At dev-story start | `exogram task next --story <story_id>` | Focus on unblocked story tasks |
| Before significant file edits | `exogram impact <file_path>` | Blast radius awareness |
| Linking files to tasks | `exogram task link <task_id> <file_path>` | Bidirectional traceability |
| Out-of-scope items | `exogram task add "Future: <title>"` | Capture deferral |
| Task complete | `exogram task update <task_id> --status done` | Advance task graph |
| After code-review (sprint-status → done) | `exogram epic import` | Sync final status |
