package detectors

import (
	"strings"
	"testing"
	"time"

	"github.com/d56de/shrike/internal/core"
)

func mb(n uint64) uint64 { return n * 1024 * 1024 }

func memCfg() core.DetectorConfig {
	return core.DetectorConfig{
		"rss_threshold": uint64(1024) * 1024 * 1024, // 1 GiB
		"min_age":       5 * time.Minute,
		"ignore":        []string(nil),
	}
}

func TestMemleak_HogFiresOnSingleScan(t *testing.T) {
	m := NewMemleak()
	procs := []core.ProcessInfo{
		{PID: 1, Command: "bloaty", RSS: mb(1500), ElapsedTime: time.Hour, StartedAt: time.Unix(1000, 0)},
	}
	got := m.Detect(procs, memCfg())
	if len(got) != 1 {
		t.Fatalf("expected 1 hog finding, got %d", len(got))
	}
	if got[0].Detector != "memleak" {
		t.Errorf("detector = %q, want memleak", got[0].Detector)
	}
}

func TestMemleak_BelowThresholdNoHog(t *testing.T) {
	m := NewMemleak()
	procs := []core.ProcessInfo{{PID: 1, Command: "x", RSS: mb(300), ElapsedTime: time.Hour, StartedAt: time.Unix(1000, 0)}}
	if got := m.Detect(procs, memCfg()); len(got) != 0 {
		t.Fatalf("expected no finding for a 300MB single sample, got %+v", got)
	}
}

func TestMemleak_YoungHogGatedByMinAge(t *testing.T) {
	m := NewMemleak()
	procs := []core.ProcessInfo{{PID: 1, Command: "x", RSS: mb(2000), ElapsedTime: time.Minute, StartedAt: time.Unix(1000, 0)}}
	if got := m.Detect(procs, memCfg()); len(got) != 0 {
		t.Fatalf("expected a young huge proc to be gated by min_age, got %+v", got)
	}
}

func TestMemleak_LeakFiresAfterSustainedGrowth(t *testing.T) {
	m := NewMemleak()
	st := time.Unix(1000, 0)
	seq := []uint64{mb(400), mb(500), mb(600), mb(700)} // below the 1GB hog threshold
	for i, r := range seq {
		got := m.Detect([]core.ProcessInfo{
			{PID: 1, Command: "leaky", RSS: r, ElapsedTime: time.Hour, StartedAt: st},
		}, memCfg())
		if i < 3 && len(got) != 0 {
			t.Fatalf("sample %d: expected no finding yet, got %+v", i, got)
		}
		if i == 3 {
			if len(got) != 1 {
				t.Fatalf("sample 3: expected a leak finding, got %d", len(got))
			}
			if got[0].Severity < core.SeverityMedium {
				t.Errorf("expected >= medium severity, got %v", got[0].Severity)
			}
			if !strings.Contains(got[0].Reason, "climbing") {
				t.Errorf("expected 'climbing' in reason, got %q", got[0].Reason)
			}
		}
	}
}

func TestMemleak_GCJitterNoLeak(t *testing.T) {
	m := NewMemleak()
	st := time.Unix(1000, 0)
	seq := []uint64{mb(400), mb(450), mb(380), mb(420), mb(400), mb(410)} // oscillates, no net growth
	for _, r := range seq {
		got := m.Detect([]core.ProcessInfo{
			{PID: 1, Command: "gc", RSS: r, ElapsedTime: time.Hour, StartedAt: st},
		}, memCfg())
		if len(got) != 0 {
			t.Fatalf("expected no leak for GC jitter, got %+v", got)
		}
	}
}

func TestMemleak_PIDReuseResetsHistory(t *testing.T) {
	m := NewMemleak()
	old := time.Unix(1000, 0)
	for _, r := range []uint64{mb(400), mb(500), mb(600)} {
		m.Detect([]core.ProcessInfo{{PID: 1, Command: "a", RSS: r, ElapsedTime: time.Hour, StartedAt: old}}, memCfg())
	}
	got := m.Detect([]core.ProcessInfo{
		{PID: 1, Command: "b", RSS: mb(700), ElapsedTime: time.Hour, StartedAt: time.Unix(2000, 0)},
	}, memCfg())
	if len(got) != 0 {
		t.Fatalf("expected reset history (no growth) after PID reuse, got %+v", got)
	}
}

func TestMemleak_EvictionResetsHistory(t *testing.T) {
	m := NewMemleak()
	st := time.Unix(1000, 0)
	for _, r := range []uint64{mb(400), mb(500), mb(600)} {
		m.Detect([]core.ProcessInfo{{PID: 1, Command: "a", RSS: r, ElapsedTime: time.Hour, StartedAt: st}}, memCfg())
	}
	m.Detect(nil, memCfg()) // process gone this scan → evicted
	got := m.Detect([]core.ProcessInfo{
		{PID: 1, Command: "a", RSS: mb(700), ElapsedTime: time.Hour, StartedAt: st},
	}, memCfg())
	if len(got) != 0 {
		t.Fatalf("expected eviction to reset history (no leak on reappearance), got %+v", got)
	}
}

func TestMemleak_IgnoreList(t *testing.T) {
	m := NewMemleak()
	cfg := memCfg()
	cfg["ignore"] = []string{"bloaty"}
	procs := []core.ProcessInfo{{PID: 1, Command: "bloaty", RSS: mb(2000), ElapsedTime: time.Hour, StartedAt: time.Unix(1000, 0)}}
	if got := m.Detect(procs, cfg); len(got) != 0 {
		t.Fatalf("expected ignored command to produce no finding, got %+v", got)
	}
}

func TestMemleak_FlatThenSpikeNoLeak(t *testing.T) {
	m := NewMemleak()
	st := time.Unix(1000, 0)
	// Flat at 300MB then one spike to 900MB (still below the 1GB hog threshold).
	// A lone end-spike is not a sustained trend → must not fire.
	seq := []uint64{mb(300), mb(300), mb(300), mb(900)}
	for _, r := range seq {
		got := m.Detect([]core.ProcessInfo{
			{PID: 1, Command: "spiky", RSS: r, ElapsedTime: time.Hour, StartedAt: st},
		}, memCfg())
		if len(got) != 0 {
			t.Fatalf("expected no leak for a lone end-spike, got %+v", got)
		}
	}
}

func TestMemleak_SlowLeakOverManyScans(t *testing.T) {
	m := NewMemleak()
	st := time.Unix(1000, 0)
	var got []core.Finding
	// Steady +40MB climb over 14 scans (300..820MB), staying below the 1GB hog
	// threshold so this exercises the growth path after the ring buffer fills.
	for i := 0; i < 14; i++ {
		rss := mb(300 + uint64(i)*40)
		got = m.Detect([]core.ProcessInfo{
			{PID: 1, Command: "slow", RSS: rss, ElapsedTime: time.Hour, StartedAt: st},
		}, memCfg())
	}
	if len(got) != 1 {
		t.Fatalf("expected a sustained slow leak to fire after the ring buffer fills, got %d", len(got))
	}
	if !strings.Contains(got[0].Reason, "climbing") {
		t.Errorf("expected 'climbing' reason, got %q", got[0].Reason)
	}
}
