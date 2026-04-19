package actions

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/d56de/shrike/internal/core"
)

// Sample runs `sample <pid> 5` and parses the output into call-stack summaries.
//
// Execute is side-effect-free apart from launching the macOS sample(1) helper.
// The parsed stacks are stored for the TUI modal to render; the caller
// retrieves them via Sample.Stacks after Execute returns.
type Sample struct {
	Stacks map[int][]Stack // keyed by PID
}

// Stack is one hot call-stack entry from a sample run.
type Stack struct {
	Percent float64
	Top     string // top-of-stack symbol
	Frames  []string
}

// NewSample returns a ready-to-use Sample action with an empty Stacks map.
func NewSample() *Sample { return &Sample{Stacks: map[int][]Stack{}} }

// Key implements core.Action.
func (*Sample) Key() rune { return 's' }

// Name implements core.Action.
func (*Sample) Name() string { return "sample" }

// Confirm implements core.Action. Empty → no confirmation needed (read-only).
func (*Sample) Confirm() string { return "" }

// Destructive implements core.Action.
func (*Sample) Destructive() bool { return false }

// Execute runs `sample <pid> 5` per target and stores the parsed call-stack
// summary under Sample.Stacks[pid].
func (s *Sample) Execute(ctx context.Context, targets []core.ProcessInfo) []core.ActionResult {
	out := make([]core.ActionResult, 0, len(targets))
	for _, t := range targets {
		cctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		cmd := exec.CommandContext(cctx, "sample", fmt.Sprintf("%d", t.PID), "5") //nolint:gosec // argv is integer PID + fixed "5", no user-controlled shell input
		raw, err := cmd.Output()
		cancel()
		if err != nil {
			out = append(out, core.ActionResult{PID: t.PID, Err: err, Message: "sample failed"})
			continue
		}
		s.Stacks[t.PID] = parseSampleOutput(string(raw))
		out = append(out, core.ActionResult{PID: t.PID, Message: fmt.Sprintf("%d stacks", len(s.Stacks[t.PID]))})
	}
	return out
}

// Matches `sample` output rows like:
//
//	"    5000 Thread_0x1   (in X) + 0"
//	"    + 4700 start  (in dyld) + 0"
//
// i.e. an optional tree-prefix of `+` chars and whitespace, then a count,
// then a symbol, then " (in <lib>) + <offset>".
var stackLineRE = regexp.MustCompile(`^[\s+]*(\d+)\s+(\S.*?)\s+\(in\s+[^)]+\)\s*\+\s*\d+$`)

// parseSampleOutput converts textual sample(1) output into a ranked list of
// top call stacks by sample count. Returns at most the 3 hottest distinct
// stacks.
func parseSampleOutput(out string) []Stack {
	counts := map[string]int{}
	total := 0
	for _, line := range strings.Split(out, "\n") {
		m := stackLineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n := 0
		if _, err := fmt.Sscanf(m[1], "%d", &n); err != nil || n == 0 {
			continue
		}
		symbol := strings.TrimSpace(m[2])
		counts[symbol] = n
		if n > total {
			total = n
		}
	}
	if total == 0 {
		return nil
	}
	type kv struct {
		sym string
		n   int
	}
	kvs := make([]kv, 0, len(counts))
	for k, v := range counts {
		kvs = append(kvs, kv{k, v})
	}
	sort.Slice(kvs, func(i, j int) bool { return kvs[i].n > kvs[j].n })

	var stacks []Stack
	for i, x := range kvs {
		if i >= 3 {
			break
		}
		stacks = append(stacks, Stack{
			Percent: float64(x.n) / float64(total) * 100,
			Top:     x.sym,
		})
	}
	return stacks
}
