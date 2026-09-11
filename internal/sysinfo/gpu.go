package sysinfo

import (
	"context"
	"encoding/xml"
	"fmt"
	"math"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/d56de/shrike/internal/core"
)

// GPUProvider reads the accelerator subtree without root or CGO. Registry fields
// vary by driver/macOS; missing counters are deliberately left unavailable.
type GPUProvider struct{}

// Snapshot takes one bounded observation of GPU load and client counters.
func (GPUProvider) Snapshot(ctx context.Context) (core.GPUSample, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return core.GPUSample{}, err
	}
	raw, err := exec.CommandContext(ctx, "/usr/sbin/ioreg", "-a", "-l", "-r", "-c", "IOAccelerator").Output()
	if ctx.Err() != nil {
		return core.GPUSample{}, ctx.Err()
	}
	if err != nil {
		return core.GPUSample{}, fmt.Errorf("read GPU registry: %w", err)
	}
	devices, err := parseGPUs(raw)
	return core.GPUSample{At: time.Now(), Devices: devices}, err
}

// plistNode retains integer text to avoid loss of 64-bit counters through float64.
type plistNode struct {
	XMLName  xml.Name
	Text     string      `xml:",chardata"`
	Children []plistNode `xml:",any"`
}

func (n plistNode) field(key string) plistNode {
	if n.XMLName.Local != "dict" {
		return plistNode{}
	}
	for i := 0; i+1 < len(n.Children); i += 2 {
		if n.Children[i].XMLName.Local == "key" && n.Children[i].Text == key {
			return n.Children[i+1]
		}
	}
	return plistNode{}
}

func (n plistNode) integer() (uint64, bool) {
	if n.XMLName.Local != "integer" {
		return 0, false
	}
	v, err := strconv.ParseUint(strings.TrimSpace(n.Text), 0, 64)
	return v, err == nil
}

func parseGPUs(raw []byte) ([]core.GPUDevice, error) {
	var root plistNode
	if err := xml.Unmarshal(raw, &root); err != nil {
		return nil, fmt.Errorf("decode GPU registry: %w", err)
	}
	if root.XMLName.Local != "plist" || len(root.Children) != 1 || root.Children[0].XMLName.Local != "array" {
		return nil, fmt.Errorf("GPU registry is not a plist array")
	}
	var devices []core.GPUDevice
	seen := map[uint64]bool{}
	for _, node := range root.Children[0].Children {
		id, ok := node.field("IORegistryEntryID").integer()
		if !ok || id == 0 || seen[id] {
			continue
		}
		seen[id] = true
		d := core.GPUDevice{ID: id, Name: node.field("IORegistryEntryName").Text}
		if d.Name == "" {
			d.Name = "GPU"
		}
		u := node.field("PerformanceStatistics").field("Device Utilization %")
		if u.XMLName.Local == "integer" || u.XMLName.Local == "real" {
			if v, err := strconv.ParseFloat(strings.TrimSpace(u.Text), 64); err == nil && !math.IsNaN(v) && v >= 0 && v <= 100 {
				d.Utilization = &v
			}
		}
		seenClients := map[uint64]bool{}
		var clients func(plistNode)
		clients = func(n plistNode) {
			cid, idOK := n.field("IORegistryEntryID").integer()
			usage := n.field("AppUsage")
			timeOK := usage.XMLName.Local == "array" && len(usage.Children) > 0
			var ticks uint64
			for _, entry := range usage.Children {
				v, ok := entry.field("accumulatedGPUTime").integer()
				if !ok || v > math.MaxUint64-ticks {
					timeOK = false
					break
				}
				ticks += v
			}
			var pid int
			_, pidErr := fmt.Sscanf(n.field("IOUserClientCreator").Text, "pid %d,", &pid)
			if idOK && cid > 0 && timeOK && pidErr == nil && pid > 0 && !seenClients[cid] {
				d.Clients = append(d.Clients, core.GPUClient{ID: cid, PID: pid, Ticks: ticks, Channels: len(usage.Children)})
				seenClients[cid] = true
			}
			for _, child := range n.field("IORegistryEntryChildren").Children {
				clients(child)
			}
		}
		clients(node)
		devices = append(devices, d)
	}
	return devices, nil
}
