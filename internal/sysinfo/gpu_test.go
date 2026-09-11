package sysinfo

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"
)

const gpuXML = `<?xml version="1.0"?><plist version="1.0"><array>
<dict><key>IORegistryEntryID</key><integer>42</integer>
<key>IORegistryEntryName</key><string>AGX</string>
<key>PerformanceStatistics</key><dict><key>Device Utilization %</key><integer>97</integer></dict>
<key>IORegistryEntryChildren</key><array><dict>
<key>IORegistryEntryID</key><integer>99</integer>
<key>IOUserClientCreator</key><string>pid 123, Example</string>
<key>AppUsage</key><array><dict><key>API</key><string>Metal</string><key>accumulatedGPUTime</key><integer>18446744073709551500</integer></dict><dict><key>API</key><string>Metal</string><key>accumulatedGPUTime</key><integer>100</integer></dict></array>
</dict><dict><key>IORegistryEntryID</key><integer>100</integer>
<key>IOUserClientCreator</key><string>pid 0, invalid</string>
<key>AppUsage</key><array><dict><key>accumulatedGPUTime</key><integer>999</integer></dict></array>
</dict></array></dict>
<dict><key>IORegistryEntryID</key><integer>43</integer><key>IORegistryEntryName</key><string>Other GPU</string>
<key>PerformanceStatistics</key><dict><key>Device Utilization %</key><real>101</real></dict></dict>
</array></plist>`

func TestGPUParserPreservesDeviceAndClientIdentity(t *testing.T) {
	devices, err := parseGPUs([]byte(gpuXML))
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 2 {
		t.Fatalf("devices = %#v", devices)
	}
	d := devices[0]
	if d.ID != 42 || d.Name != "AGX" || d.Utilization == nil || *d.Utilization != 97 {
		t.Fatalf("device = %#v", d)
	}
	if len(d.Clients) != 1 || d.Clients[0].ID != 99 || d.Clients[0].PID != 123 || d.Clients[0].Ticks != 18446744073709551600 {
		t.Fatalf("clients = %#v", d.Clients)
	}
	if devices[1].Utilization != nil {
		t.Fatal("invalid utilization must be unknown, not clamped")
	}
}

func TestGPUProviderLive(t *testing.T) {
	if os.Getenv("SHRIKE_GPU_SMOKE") != "1" {
		t.Skip("opt-in live GPU telemetry check")
	}
	first, err := (GPUProvider{}).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	second, err := (GPUProvider{}).Snapshot(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	previous := map[uint64]uint64{}
	for _, d := range first.Devices {
		for _, c := range d.Clients {
			previous[c.ID] = c.Ticks
		}
	}
	for _, d := range second.Devices {
		growing := 0
		for _, c := range d.Clients {
			if prev, ok := previous[c.ID]; ok && c.Ticks > prev {
				growing++
			}
		}
		if d.Utilization == nil {
			t.Logf("GPU %d %s: utilization unavailable, clients=%d", d.ID, d.Name, len(d.Clients))
			continue
		}
		t.Logf("GPU %d %s: %.1f%%, clients=%d, advancing counters=%d", d.ID, d.Name, *d.Utilization, len(d.Clients), growing)
	}
	if len(second.Devices) == 0 {
		t.Log("no supported GPU devices")
	}
}

func TestGPUParserRejectsMalformedAndUnsupportedData(t *testing.T) {
	for _, raw := range []string{"oops", "<plist><array>", "<plist><dict/></plist>"} {
		if _, err := parseGPUs([]byte(raw)); err == nil {
			t.Fatalf("accepted %q", raw)
		}
	}
	devices, err := parseGPUs([]byte("<plist><array/></plist>"))
	if err != nil || len(devices) != 0 {
		t.Fatalf("empty registry: %v, %v", devices, err)
	}
}

func TestGPUCollectorHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := (GPUProvider{}).Snapshot(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
}
