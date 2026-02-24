# Project Context

This project uses Exogram for task context and BMAD for sprint tracking. Agents running BMAD workflows MUST run the following commands at the specified trigger points to keep the Exogram task graph synchronized with BMAD sprint-status and story state.

## Exogram Task Sync Protocol

### Trigger: After create-story completes

When `sprint-status.yaml` is updated to `ready-for-dev` for a new story:

```bash
exogram epic import
```

**Rationale:** Syncs new epic/story state from `sprint-status.yaml` into the Exogram SQLite DB so task graph reflects the latest planning artifacts.

### Trigger: At dev-story start

When beginning implementation of a story, identify available work:

```bash
exogram task next --story <story_id>
```

**Rationale:** Identifies the next unblocked task(s) for the current story, using topological sort to respect task dependencies.

### Trigger: During dev-story (before significant file edits)

Before editing any file that may have downstream dependents:

```bash
exogram impact <file_path>
```

**Rationale:** Assesses blast radius — lists files that import the target file — so the agent understands which other files may need updating.

### Trigger: During dev-story (linking work to tasks)

As files are created or modified for a task:

```bash
exogram task link <task_id> <file_path>
```

**Rationale:** Links concrete file artifacts to their originating task, enabling `exogram file tasks <file_path>` and `exogram impact` to surface task context.

### Trigger: For out-of-scope items discovered during implementation

When identifying work that is out of scope for the current story:

```bash
exogram task add "Future: <short title>"
```

**Rationale:** Captures deferred work in the task graph without blocking current story progress. Use the `Future:` prefix to distinguish speculative/deferred tasks.

### Trigger: At dev-story completion

When a task is fully implemented and tests pass:

```bash
exogram task update <task_id> --status done
```

**Rationale:** Marks completed tasks in the graph, which unblocks dependent tasks and keeps `exogram task next` accurate for future work.

### Trigger: After code-review completes

When `sprint-status.yaml` is updated to `done` for a reviewed story:

```bash
exogram epic import
```

**Rationale:** Syncs completed story status into the SQLite DB, ensuring the task graph reflects the finalized sprint state.

## Quick Reference

| Trigger | Command | When |
|---------|---------|------|
| After create-story (→ `ready-for-dev`) | `exogram epic import` | After sprint-status.yaml update |
| At dev-story start | `exogram task next --story <story_id>` | Before first task |
| Before file edits | `exogram impact <file_path>` | Before any significant edit |
| Linking work | `exogram task link <task_id> <file_path>` | As files are created/modified |
| Out-of-scope items | `exogram task add "Future: <title>"` | When deferring work |
| Task complete | `exogram task update <task_id> --status done` | After tests pass |
| After code-review (→ `done`) | `exogram epic import` | After sprint-status.yaml update |
