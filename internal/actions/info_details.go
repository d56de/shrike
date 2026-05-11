package actions

import (
	"context"
	"fmt"
	"strings"

	"github.com/shirou/gopsutil/v4/process"
)

// InfoDetails carries lazy-loaded extra fields shown in the info modal.
// All fields are best-effort — missing data renders as "—" rather than
// failing the whole panel.
type InfoDetails struct {
	PID       int
	Cwd       string
	Threads   int32
	IOReads   uint64 // bytes
	IOWrites  uint64 // bytes
	Ancestors []Ancestor
	Notes     []string // human-readable hints when fields couldn't be fetched
}

// Ancestor is one rung of the process ancestry chain, root-most first.
type Ancestor struct {
	PID     int
	Command string
}

// FetchInfo gathers the lazy info-modal fields for a single PID. Always
// returns a struct, even when the process has exited or denied access; the
// caller renders missing fields as "—".
//
// Budget: ~50ms typical. Walks at most 8 parents to keep the cost bounded
// (an in-the-wild process tree rarely exceeds that depth).
func FetchInfo(ctx context.Context, pid int) InfoDetails {
	d := InfoDetails{PID: pid}

	// macOS PIDs are bounded by kern.maxproc (~100k); the int32 cast is safe
	// in practice. gosec G115 doesn't model that constraint.
	p, err := process.NewProcessWithContext(ctx, int32(pid)) //nolint:gosec // bounded PID
	if err != nil {
		d.Notes = append(d.Notes, fmt.Sprintf("process gone: %v", err))
		return d
	}

	if cwd, err := p.CwdWithContext(ctx); err == nil {
		d.Cwd = cwd
	}
	if n, err := p.NumThreadsWithContext(ctx); err == nil {
		d.Threads = n
	}
	if io, err := p.IOCountersWithContext(ctx); err == nil && io != nil {
		d.IOReads = io.ReadBytes
		d.IOWrites = io.WriteBytes
	}

	d.Ancestors = walkAncestors(ctx, p, 8)
	if len(d.Ancestors) == 0 {
		d.Notes = append(d.Notes, "ancestry unavailable")
	}
	return d
}

// walkAncestors climbs the parent chain up to `limit` rungs, returning the
// list root-most first (e.g. launchd → Terminal → zsh → Chrome).
// Self-references and cycles are guarded against.
func walkAncestors(ctx context.Context, p *process.Process, limit int) []Ancestor {
	out := make([]Ancestor, 0, limit)
	cur := p
	seen := map[int32]bool{p.Pid: true}

	for i := 0; i < limit; i++ {
		parent, err := cur.ParentWithContext(ctx)
		if err != nil || parent == nil {
			break
		}
		if seen[parent.Pid] {
			break // cycle guard
		}
		seen[parent.Pid] = true

		name, _ := parent.NameWithContext(ctx)
		out = append(out, Ancestor{PID: int(parent.Pid), Command: name})
		cur = parent
		if parent.Pid == 1 {
			break
		}
	}

	// Reverse so the eldest ancestor (launchd) comes first.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out
}

// FormatBytes renders a byte count as a compact human-readable string:
// "0 B", "12 KB", "3.4 MB", "1.2 GB". Used by the info-modal renderer to
// keep IO counter rows readable.
func FormatBytes(b uint64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := uint64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	suffix := "KMGTPE"[exp]
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), suffix)
}

// FormatAncestors renders the ancestor chain as a one-line breadcrumb:
// "launchd(1) → Terminal(456) → zsh(789)". Returns "—" when empty.
func FormatAncestors(a []Ancestor) string {
	if len(a) == 0 {
		return "—"
	}
	parts := make([]string, len(a))
	for i, anc := range a {
		name := anc.Command
		if name == "" {
			name = "?"
		}
		parts[i] = fmt.Sprintf("%s(%d)", name, anc.PID)
	}
	return strings.Join(parts, " → ")
}
