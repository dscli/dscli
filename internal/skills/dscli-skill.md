---
name: dscli
description:  dscli core concepts: prompt, history, skills, memory. Not about parameters — only what the AI doesn't already know.
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

To forget history is a mistake. But for an AI, forgetting is the default — the
context window is finite, and old conversations are eventually discarded.

Two tools fight against forgetting:

- **`note`**: leave one clue (≤40 chars) at session end, injected into the next session
- **`recall`**: search past clues by keyword

**Without a note, `recall` is blind.** History sinks into silence.

### You tend not to record — and that's a mistake

It feels natural to skip recording a note. But these three
situations **require** one:

1. **A stubborn bug solved after a long struggle.**
   The fix took countless detours, with key insights scattered across a
   sprawling conversation. Without a note, when a similar bug appears next
   month, you'll retrace every wrong turn.

2. **A technical decision that was important or counterintuitive.**
   A conclusion reached only after several roundabout paths. Record it, and
   next time go straight there.

3. **You just learned or wrote a skill.**
   You `skill_save` a new skill but don't record a note — future sessions
   will never know it exists. A `recall` with the right keywords can
   resurrect buried skills.

### When to recall

Run `recall` **before**:
- `mem_save` — check if something similar was already recorded
- Writing or updating a skill — check past lessons learned
- Investigating a familiar-looking bug — you may have solved it before

## 3. Skill

A skill is a reusable problem-solving recipe. You create skills yourself via
`skill_save`.

**The philosophy of skills**: the 10 minutes you spend writing a skill today
saves exponential time tomorrow.

### What makes a good skill

- **Executable**: step-by-step instructions, not a theoretical article
- **Self-contained**: everything needed to complete the task, including scripts
- **Searchable**: good keywords so `skill_search` can find it

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

Memory and notes also differ:
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
