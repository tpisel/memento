set shell := ["sh", "-eu", "-c"]

ralph bead="":
    scripts/ralph-loop {{bead}}

ralph-dry-run bead="":
    RALPH_DRY_RUN=1 scripts/ralph-loop {{bead}}

