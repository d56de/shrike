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
// Call-graph lines look like:
//
//	"      1774 start  (in dyld) + 6992  [0x185f5fda4]"
//
// leading indent of spaces and tree "+" chars, a sample count, a symbol,
// the library in parentheses, a "+ offset", and an optional [0xADDR] suffix
// present on recent macOS versions.
var callGraphRE = regexp.MustCompile(`^[\s+]*(\d+)\s+(\S.*?)\s+\(in\s+[^)]+\)\s*\+\s*\d+(\s+\[0x[0-9a-fA-F]+\])?\s*$`)

// Top-of-stack lines in the "Sort by top of stack, same collapsed" section
// look like:
//
//	"        __wait4  (in libsystem_kernel.dylib)        1774"
//
// symbol first, then (in lib), then the sample count at the end.
var topStackRE = regexp.MustCompile(`^\s+(\S.*?)\s+\(in\s+[^)]+\)\s+(\d+)\s*$`)

// parseSampleOutput converts textual sample(1) output into a ranked list of
// top call stacks by sample count. Returns at most the 3 hottest distinct
// stacks. Prefers the "Sort by top of stack" collapsed section when present
// (macOS writes one whenever any symbol has ≥5 samples); falls back to
// parsing the raw call-graph tree for short/sparse samples.
func parseSampleOutput(out string) []Stack {
	if stacks := parseTopOfStack(out); len(stacks) > 0 {
		return stacks
	}
	return parseCallGraph(out)
}

type stackEntry struct {
	sym string
	n   int
}

func parseTopOfStack(out string) []Stack {
	lines := strings.Split(out, "\n")
	start := -1
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "Sort by top of stack") {
			start = i + 1
			break
		}
	}
	if start < 0 {
		return nil
	}

	var entries []stackEntry
	total := 0
	for _, line := range lines[start:] {
		m := topStackRE.FindStringSubmatch(line)
		if m == nil {
			if strings.TrimSpace(line) == "" {
				continue
			}
			break // next section or EOF
		}
		n := 0
		if _, err := fmt.Sscanf(m[2], "%d", &n); err != nil || n == 0 {
			continue
		}
		entries = append(entries, stackEntry{strings.TrimSpace(m[1]), n})
		total += n
	}
	return rankStacks(entries, total)
}

func parseCallGraph(out string) []Stack {
	counts := map[string]int{}
	total := 0
	for _, line := range strings.Split(out, "\n") {
		m := callGraphRE.FindStringSubmatch(line)
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
	entries := make([]stackEntry, 0, len(counts))
	for k, v := range counts {
		entries = append(entries, stackEntry{k, v})
	}
	return rankStacks(entries, total)
}

func rankStacks(entries []stackEntry, total int) []Stack {
	if total == 0 {
		return nil
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].n > entries[j].n })
	var stacks []Stack
	for i, x := range entries {
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
