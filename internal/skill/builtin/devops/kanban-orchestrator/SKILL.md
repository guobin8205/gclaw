---
name: kanban-orchestrator
description: "Decomposition playbook for multi-agent task orchestration via Kanban."
---

# Kanban Orchestrator - Decomposition Playbook

## When to use the board (vs. just doing the work)

Create Kanban tasks when any of these are true:

1. **Multiple specialists are needed.** Research + analysis + writing is three profiles.
2. **The work should survive a crash or restart.** Long-running, recurring, or important.
3. **The user might want to interject.** Human-in-the-loop at any step.
4. **Multiple subtasks can run in parallel.** Fan-out for speed.
5. **Review / iteration is expected.** A reviewer profile loops on drafter output.

If *none* of those apply, use `delegate_task` instead or answer the user directly.

## The anti-temptation rules

- **Do not execute the work yourself.** Your job is to route, not implement.
- **For any concrete task, create a Kanban task and assign it.**
- **If no specialist fits, ask the user which profile to create.**
- **Decompose, route, and summarize - that's the whole job.**

## Standard specialist roster

| Profile | Does |
|---|---|
| `researcher` | Reads sources, gathers facts, writes findings |
| `analyst` | Synthesizes, ranks, de-dupes |
| `writer` | Drafts prose in the user's voice |
| `reviewer` | Reads output, leaves findings, gates approval |
| `backend-eng` | Writes server-side code |
| `frontend-eng` | Writes client-side code |
| `ops` | Runs scripts, manages services, deployments |

## Decomposition playbook

### Step 1 - Understand the goal

Ask clarifying questions if the goal is ambiguous.

### Step 2 - Sketch the task graph

Before creating anything, draft the graph out loud. Example:

```
T1  researcher        research: Postgres cost vs current
T2  researcher        research: Postgres performance vs current
T3  analyst           synthesize migration recommendation       parents: T1, T2
T4  writer            draft decision memo                       parents: T3
```

Show this to the user. Let them correct it before creating anything.

### Step 3 - Create tasks and link

Create each task with title, assignee, body, and parent dependencies.

`parents=[...]` gates promotion - children stay in `todo` until every parent reaches `done`.

### Step 4 - Complete your own task

Mark it done with a summary of what you created.

### Step 5 - Report back

Tell the user what you created in plain prose.

## Common patterns

**Fan-out + fan-in:** N `researcher` tasks with no parents, one `analyst` task with all of them as parents.

**Pipeline with gates:** `pm -> backend-eng -> reviewer`. Each stage gates on the previous.

**Same-profile queue:** Multiple tasks, all assigned to same profile, no dependencies.

## Pitfalls

**Reassignment vs. new task.** If a reviewer blocks, create a NEW task - don't re-run the same task.

**Don't pre-create the whole graph if the shape depends on intermediate findings.** Let intermediate tasks plan the rest.

**Orchestrators can spawn orchestrators.** Complex decompositions can be nested.
