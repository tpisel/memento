---
name: memento-write
description: Use before creating or updating any note in the memento vault (memento-memory/). Loads the project writing guide and the safe-write rules so durable knowledge lands correctly and read-only notes are not corrupted.
---

# Writing to the memento vault

Before authoring a vault write:

1. **Read the writing guide.** Run `memento read _memento/writing` to load when/what to write, what to keep in beads instead, and the expected note shape. Do this before composing, not after.
2. **Write through memento, not native file edits.** Use `memento write` so the mode check (`append-only` / `living` / `read-only`) is applied. A native `Edit`/`Write` of a vault file bypasses that check and can silently overwrite a `read-only` note — the read-only guarantee lives in the write verb, not in the file.
3. **Keep it scannable.** Durable notes should read cleanly from `memento brief`; lead the summary with the load-bearing fact or decision.

This skill is a delivery surface for `_memento/writing.md` — that file is the source of truth. If the two ever disagree, the guide wins.
