# AGENTS.md

<!-- EXOGRAM_MANAGED_START -->
## Exogram Task Guidance (Managed)

Use Exogram to maintain task context during development and keep user-visible state accurate.

- At session start, run `exogram context` to establish project state (unblocked tasks, in-progress work, index status).
- Add follow-up tasks for out-of-scope requests with `exogram task add`.
- When a user says "later", capture the deferral by creating/updating a task.
- Query `exogram task get <task_id>` for task details when needed.
- Query `exogram impact <file_path>` to assess and explain dependency impact.
- Do not automatically run `exogram task next` on session start.
- Provide periodic summaries of task operations and rationale to the user.
<!-- EXOGRAM_MANAGED_END -->
