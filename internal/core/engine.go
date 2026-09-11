package core

import (
	"context"
	"maps"
	"slices"
	"sort"
	"sync"

	"golang.org/x/sync/errgroup"
)

// Snapshotter captures the current process list. Implemented by internal/sysinfo.
type Snapshotter interface {
	Snapshot(ctx context.Context) ([]ProcessInfo, error)
}

// Engine wires a snapshotter to detectors. Reuse the engine across sequential
// Runs so stateful detectors can retain observation history.
type Engine struct {
	Snapshotter Snapshotter
	Detectors   []Detector
	Configs     map[string]DetectorConfig // keyed by Detector.Name()
	configMu    sync.RWMutex
}

// Ignore updates an in-session ignore list without racing an active scan.
func (e *Engine) Ignore(detector, command string) {
	e.configMu.Lock()
	defer e.configMu.Unlock()
	if e.Configs == nil {
		e.Configs = map[string]DetectorConfig{}
	}
	if e.Configs[detector] == nil {
		e.Configs[detector] = DetectorConfig{}
	}
	ignore, _ := e.Configs[detector]["ignore"].([]string)
	if !slices.Contains(ignore, command) {
		e.Configs[detector]["ignore"] = append(slices.Clone(ignore), command)
	}
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
		e.configMu.RLock()
		cfg := maps.Clone(e.Configs[d.Name()])
		e.configMu.RUnlock()
		g.Go(func() error {
			var fs []Finding
			if contextual, ok := d.(ContextDetector); ok {
				fs = contextual.DetectContext(ctx, snap, cfg)
			} else {
				fs = d.Detect(snap, cfg)
			}
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
