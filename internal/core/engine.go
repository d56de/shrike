package core

import (
	"context"
	"sort"
	"sync"

	"golang.org/x/sync/errgroup"
)

// Snapshotter captures the current process list. Implemented by internal/sysinfo.
type Snapshotter interface {
	Snapshot(ctx context.Context) ([]ProcessInfo, error)
}

// Engine wires a snapshotter to a slice of detectors. Engines are stateless
// across Run calls — construct once, reuse.
type Engine struct {
	Snapshotter Snapshotter
	Detectors   []Detector
	Configs     map[string]DetectorConfig // keyed by Detector.Name()
}

// Run takes a fresh snapshot, runs every detector in parallel, merges and
// sorts the findings (Severity desc, then Score desc).
func (e *Engine) Run(ctx context.Context) ([]Finding, error) {
	snap, err := e.Snapshotter.Snapshot(ctx)
	if err != nil {
		return nil, err
	}

	var (
		g   errgroup.Group
		mu  sync.Mutex
		all []Finding
	)
	for _, d := range e.Detectors {
		d := d
		g.Go(func() error {
			fs := d.Detect(snap, e.Configs[d.Name()])
			mu.Lock()
			all = append(all, fs...)
			mu.Unlock()
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return nil, err
	}

	sort.Slice(all, func(i, j int) bool {
		if all[i].Severity != all[j].Severity {
			return all[i].Severity > all[j].Severity
		}
		return all[i].Score > all[j].Score
	})
	return all, nil
}
