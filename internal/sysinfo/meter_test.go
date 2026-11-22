package sysinfo

import (
	"context"
	"testing"
	"time"

	"github.com/d56de/shrike/internal/core"
)

type meterGPU struct{ samples []core.GPUSample }

func (f *meterGPU) Snapshot(context.Context) (core.GPUSample, error) {
	s := f.samples[0]
	f.samples = f.samples[1:]
	return s, nil
}

func TestLoadMeterDeltas(t *testing.T) {
	t0 := time.Unix(0, 0)
	gpu := &meterGPU{samples: []core.GPUSample{
		{At: t0, Devices: []core.GPUDevice{{Clients: []core.GPUClient{{ID: 1, PID: 9, Ticks: 0}}}}},
		{At: t0.Add(time.Second), Devices: []core.GPUDevice{{Clients: []core.GPUClient{{ID: 1, PID: 9, Ticks: 250_000_000}, {ID: 2, PID: 7, Ticks: 999}}}}},
	}}
	cpus := []cpuTimes{{busy: 10, total: 100}, {busy: 40, total: 200}}
	m := &LoadMeter{GPU: gpu, cpu: func(context.Context) (cpuTimes, error) {
		c := cpus[0]
		cpus = cpus[1:]
		return c, nil
	}}
	if _, ok := m.Sample(context.Background()); ok {
		t.Fatal("first sample has no baseline and must not be ok")
	}
	p, ok := m.Sample(context.Background())
	if !ok {
		t.Fatal("second sample must be ok")
	}
	if p.CPUPercent != 30 { // 30 busy of 100 elapsed
		t.Errorf("CPU = %v, want 30", p.CPUPercent)
	}
	if p.GPUPercent != 25 { // new client ID 2 has no baseline
		t.Errorf("GPU = %v, want 25", p.GPUPercent)
	}
}
