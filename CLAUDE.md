# CLAUDE.md

<!-- EXOGRAM_MANAGED_START -->
## Exogram Agent Workflow (Managed)

Use Exogram as the source of truth for task capture and task-state management.

- At session start, run `exogram context` to establish project state (unblocked tasks, in-progress work, index status).
- Capture out-of-scope follow-ups with `exogram task add "Future: <title>"`.
- When work is deferred, update/create a task and clearly record that deferral.
- Use `exogram task get <task_id>` to clarify task details before implementation.
- Use `exogram impact <file_path>` when communicating likely change impact.
- Do not run `exogram task next` automatically; run it only when the user requests reprioritization.
- Periodically report task graph changes to the user (created/updated/deferred tasks and status).
<!-- EXOGRAM_MANAGED_END -->
