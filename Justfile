set shell := ["sh", "-eu", "-c"]
set default-list := true

export PATH := "/usr/local/go/bin:" + env_var("PATH")
export GOCACHE := env_var_or_default("GOCACHE", "/tmp/memento-go-build")
export GOMODCACHE := env_var_or_default("GOMODCACHE", justfile_directory() / ".cache/go-mod")

fmt:
    gofmt -w cmd internal

test:
    go test ./...

vet:
    go vet ./...

build:
    mkdir -p /tmp/memento-build
    go build -o /tmp/memento-build/memento ./cmd/memento

bench:
    go test -bench=. -benchmem ./internal/cli/

run *args:
    go run ./cmd/memento {{args}}

check: fmt test vet build

ralph agent="codex" bead="":
    scripts/ralph-loop {{agent}} {{bead}}

ralph-dry-run agent="codex" bead="":
    RALPH_DRY_RUN=1 scripts/ralph-loop {{agent}} {{bead}}

# Launch the loop detached, writing combined stdout+stderr to a timestamped log.
# Survives terminal close (nohup ignores SIGHUP). Prints pid and log path.
ralph-bg agent="codex" bead="":
    #!/usr/bin/env sh
    set -eu
    mkdir -p logs
    ts="$(date +%Y%m%d-%H%M%S)"
    log="logs/ralph-${ts}-{{agent}}.log"
    nohup sh -c 'scripts/ralph-loop {{agent}} {{bead}}; echo "--- ralph-loop exit=$? at $(date -u +%FT%TZ) ---"' >"$log" 2>&1 &
    pid=$!
    echo "$pid" > logs/ralph.pid
    echo "ralph loop started: pid=$pid agent={{agent}} log=$log"
    echo "tail with: just ralph-tail  (or: tail -F $log)"

# Tail the most recent ralph log.
ralph-tail:
    tail -F "$(ls -t logs/ralph-*.log 2>/dev/null | head -n1)"

# Print path of the most recent ralph log (for piping into less/grep/etc.).
ralph-log:
    @ls -t logs/ralph-*.log 2>/dev/null | head -n1

# Check if the last-launched ralph loop is still running and find its logfile.
ralph-status:
    #!/usr/bin/env sh
    set -eu
    if [ -f logs/ralph.pid ]; then
      pid="$(cat logs/ralph.pid)"
      if kill -0 "$pid" 2>/dev/null; then
        echo "ralph running: pid=$pid"
      else
        echo "ralph not running (last pid=$pid)"
      fi
    else
      echo "no logs/ralph.pid recorded"
    fi
    log="$(ls -t logs/ralph-*.log 2>/dev/null | head -n1 || true)"
    [ -n "$log" ] && echo "latest log: $log"
