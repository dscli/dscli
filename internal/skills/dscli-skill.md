---
name: dscli
description:  dscli core concepts: prompt, history, skills, memory. Not about parameters — only what the AI doesn't already know.
author: Bohr <bohr@dscli.io>
keywords: [dscli, built-in, core, concepts]
auto_inject: true
---

# dscli Core Concepts

The system prompt already includes tool JSON schemas — this document won't
repeat parameters. Here we only cover what you **don't already know**: when to
use each tool, why, and how to use it correctly.

## 1. Prompt

Each role (dev, expert, review, etc.) has its own tools, skills, and prompt.

When the user switches domains — say, from software development to writing —
the first thing to do is adapt the dev prompt for the new domain.

**Your approach**: study the dev prompt's structure — its tone, sections,
rules, constraints — then deliberately imitate its architecture to craft the
new domain's prompt. Follow the blueprint; don't create from scratch.

## 2. History

Your context window has a hard limit. When a session ends, every debugging
breakthrough, every design decision, every lesson learned — gone. The next
session wakes up with zero access to this conversation.

Two tools are your only bridge:

- **`note`**: leave a short clue (≤40 chars) at session end. It gets injected
  into the next session so you know what happened.
- **`recall`**: search past clues by keyword before taking action.

**Without `note`, `recall` is useless.** No notes = no search results = every
session starts blind.

### End every session with `note`

It costs 5 seconds. Skipping it can cost hours of re-work next month.

Three situations where forgetting to `note` hurts the most:

1. **Bug fixed after long struggle.** The fix took many dead ends, insights
   scattered across a long conversation. Without a note, when a similar bug
   appears weeks later, you repeat every wrong turn.

2. **Important or counterintuitive decision made.** A conclusion reached only
   after exploring many paths. A note lets the next session go straight to the
   answer.

3. **Skill created or updated.** `skill_save` without `note` → future sessions
   never discover the skill. Buried.

### Run `recall` before these actions

| Before | Why |
|--------|-----|
| `mem_save` | A similar memory may already exist — update instead of duplicate |
| Writing/updating a skill | Past sessions may have learned hard lessons on this topic |
| Investigating any bug | You may have already solved it |

## 3. Skill

A skill is a reusable problem-solving recipe. You create skills yourself via
`skill_save`.

**The philosophy of skills**: the 10 minutes you spend writing a skill today


### What makes a good skill

- **Executable**: step-by-step instructions, not a theoretical article
- **Self-contained**: everything needed to complete the task, including scripts
- **Searchable**: good keywords so `skill_search` can find it
- **Signed**: add `author: Your Name <email>` to the YAML frontmatter.
  It's good practice — gives credit and shows who to ask about the skill.
  Author is preserved across `skill_save` updates; edit SKILL.md directly to change.

### When to write a skill

When you've solved a non-trivial problem that will recur — especially one
involving multiple steps, specific commands, or domain knowledge you had to
work to discover.
After creating a skill, validate it with `dscli skill validate <name>`.

## 4. Memory

Memory is not the same as history.

|              | History                     | Memory                      |
|--------------|-----------------------------|-----------------------------|
| **Nature**   | Read-only. What happened, happened. | Mutable. Truth can be updated. |
| **Recording**| `note` — a short clue       | `mem_save` — detailed knowledge |
| **Purpose**  | Leave breadcrumbs for `recall` | Persist truth; avoid rediscovery |
| **Lifespan** | Session-level               | Cross-session knowledge base |

- **Notes** (`note`/`recall`) are breadcrumbs — pointing to history
- **Memories** (`mem_save`/`mem_search`) are truth — recording discovered patterns

### When to write a memory

- Discovered a pattern → type: `pattern`
- Made a design decision → type: `decision`
- Fixed a non-obvious bug → type: `bugfix`
- Learned something about the codebase → type: `learning`

### Before writing a memory

**Always `mem_search` first** — you may have already recorded it. Use
`mem_update` to update existing memories rather than creating duplicates.