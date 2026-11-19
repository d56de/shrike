package sysinfo

import (
	"math"
	"testing"
	"time"

	"github.com/d56de/shrike/internal/core"
)

const window = time.Second

func near(t *testing.T, name string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.001 {
		t.Fatalf("%s = %v, want %v", name, got, want)
	}
}

func TestComputeLoad_CurrentNotLifetime(t *testing.T) {
	a := loadReading{Procs: map[int]procReading{1: {Started: 1000, CPUSeconds: 5000}}}
	b := loadReading{Procs: map[int]procReading{1: {Started: 1000, CPUSeconds: 5000.25}}}
	near(t, "CPUPercent", computeLoad(a, b, window)[1].CPUPercent, 25)
}

func TestComputeLoad_ExitedAndNewProcesses(t *testing.T) {
	a := loadReading{Procs: map[int]procReading{2: {Started: 1, CPUSeconds: 1}}}
	b := loadReading{Procs: map[int]procReading{3: {Started: 1, CPUSeconds: 1}}}
	got := computeLoad(a, b, window)
	if _, ok := got[2]; ok {
		t.Fatal("exited pid 2 must be absent")
	}
	if _, ok := got[3]; ok {
		t.Fatal("new pid 3 must be absent")
	}
}

func TestComputeLoad_PIDReuse(t *testing.T) {
	a := loadReading{Procs: map[int]procReading{4: {Started: 1000, CPUSeconds: 1}}}
	b := loadReading{Procs: map[int]procReading{4: {Started: 2000, CPUSeconds: 9}}}
	if _, ok := computeLoad(a, b, window)[4]; ok {
		t.Fatal("reused pid 4 must be absent")
	}
}

func TestComputeLoad_GPUPercent(t *testing.T) {
	procs := map[int]procReading{1: {Started: 1, CPUSeconds: 1}}
	a := loadReading{Procs: procs, GPU: []core.GPUClient{
		{ID: 9, PID: 1, Channels: 2, Ticks: 1_000_000_000},
		{ID: 10, PID: 1, Channels: 1, Ticks: 0},
	}}
	b := loadReading{Procs: procs, GPU: []core.GPUClient{
		{ID: 9, PID: 1, Channels: 2, Ticks: 1_090_000_000},
		{ID: 10, PID: 1, Channels: 1, Ticks: 50_000_000},
	}}
	near(t, "GPUPercent", computeLoad(a, b, window)[1].GPUPercent, 14)
}

func TestComputeLoad_GPUCounterResetAndChannelChange(t *testing.T) {
	procs := map[int]procReading{1: {Started: 1, CPUSeconds: 1}}
	a := loadReading{Procs: procs, GPU: []core.GPUClient{
		{ID: 1, PID: 1, Channels: 1, Ticks: 500},
		{ID: 2, PID: 1, Channels: 2, Ticks: 0},
		{ID: 3, PID: 7, Channels: 1, Ticks: 0},
	}}
	b := loadReading{Procs: procs, GPU: []core.GPUClient{
		{ID: 1, PID: 1, Channels: 1, Ticks: 100},
		{ID: 2, PID: 1, Channels: 3, Ticks: 500_000_000},
		{ID: 3, PID: 7, Channels: 1, Ticks: 500_000_000},
	}}
	got := computeLoad(a, b, window)
	near(t, "GPUPercent", got[1].GPUPercent, 0)
	if _, ok := got[7]; ok {
		t.Fatal("client of unmatched process 7 must not create a key")
	}
}

func TestComputeLoad_NegativeCPUDelta(t *testing.T) {
	a := loadReading{Procs: map[int]procReading{1: {Started: 1, CPUSeconds: 10}}}
	b := loadReading{Procs: map[int]procReading{1: {Started: 1, CPUSeconds: 9}}}
	near(t, "CPUPercent", computeLoad(a, b, window)[1].CPUPercent, 0)
}

func TestComputeLoad_UsesMeasuredElapsedPerProcess(t *testing.T) {
	t0 := time.Unix(100, 0)
	a := loadReading{Procs: map[int]procReading{1: {Started: 1, CPUSeconds: 10, At: t0}}}
	b := loadReading{Procs: map[int]procReading{1: {Started: 1, CPUSeconds: 11, At: t0.Add(2 * time.Second)}}}
	near(t, "CPUPercent", computeLoad(a, b, window)[1].CPUPercent, 50)
}

func TestComputeLoad_UsesMeasuredGPUElapsed(t *testing.T) {
	t0 := time.Unix(100, 0)
	procs := map[int]procReading{1: {Started: 1}}
	a := loadReading{Procs: procs, GPUAt: t0, GPU: []core.GPUClient{{ID: 1, PID: 1, Ticks: 0}}}
	b := loadReading{Procs: procs, GPUAt: t0.Add(2 * time.Second), GPU: []core.GPUClient{{ID: 1, PID: 1, Ticks: 1_000_000_000}}}
	near(t, "GPUPercent", computeLoad(a, b, window)[1].GPUPercent, 50)
}
