//go:build linux

package efficiency

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

type ResourceSnapshot struct {
	MemoryTotalBytes     uint64   `json:"memoryTotalBytes"`
	MemoryAvailableBytes uint64   `json:"memoryAvailableBytes"`
	Load1                float64  `json:"load1"`
	ThermalC             *float64 `json:"thermalC,omitempty"`
	StorageFreeBytes     uint64   `json:"storageFreeBytes"`
	StorageTotalBytes    uint64   `json:"storageTotalBytes"`
}

func ReadResources(path string) ResourceSnapshot {
	var out ResourceSnapshot
	if f, err := os.Open("/proc/meminfo"); err == nil {
		scanner := bufio.NewScanner(f)
		for scanner.Scan() {
			fields := strings.Fields(scanner.Text())
			if len(fields) < 2 {
				continue
			}
			kb, _ := strconv.ParseUint(fields[1], 10, 64)
			switch strings.TrimSuffix(fields[0], ":") {
			case "MemTotal":
				out.MemoryTotalBytes = kb * 1024
			case "MemAvailable":
				out.MemoryAvailableBytes = kb * 1024
			}
		}
		_ = f.Close()
	}
	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		fields := strings.Fields(string(b))
		if len(fields) > 0 {
			out.Load1, _ = strconv.ParseFloat(fields[0], 64)
		}
	}
	if matches, _ := filepath.Glob("/sys/class/thermal/thermal_zone*/temp"); len(matches) > 0 {
		var maxC float64
		found := false
		for _, p := range matches {
			b, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			raw, err := strconv.ParseFloat(strings.TrimSpace(string(b)), 64)
			if err != nil {
				continue
			}
			c := raw
			if raw > 1000 {
				c = raw / 1000
			}
			if c > -50 && c < 200 && (!found || c > maxC) {
				maxC = c
				found = true
			}
		}
		if found {
			out.ThermalC = &maxC
		}
	}
	if strings.TrimSpace(path) == "" {
		path = "."
	}
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err == nil {
		out.StorageTotalBytes = st.Blocks * uint64(st.Bsize)
		out.StorageFreeBytes = st.Bavail * uint64(st.Bsize)
	}
	return out
}

func (r ResourceSnapshot) Validate() error {
	if r.MemoryTotalBytes > 0 && r.MemoryAvailableBytes > r.MemoryTotalBytes {
		return fmt.Errorf("available memory exceeds total memory")
	}
	return nil
}
