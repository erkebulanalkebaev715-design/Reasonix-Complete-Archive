//go:build !linux

package efficiency

type ResourceSnapshot struct {
	MemoryTotalBytes     uint64   `json:"memoryTotalBytes"`
	MemoryAvailableBytes uint64   `json:"memoryAvailableBytes"`
	Load1                float64  `json:"load1"`
	ThermalC             *float64 `json:"thermalC,omitempty"`
	StorageFreeBytes     uint64   `json:"storageFreeBytes"`
	StorageTotalBytes    uint64   `json:"storageTotalBytes"`
}

func ReadResources(string) ResourceSnapshot { return ResourceSnapshot{} }
func (ResourceSnapshot) Validate() error    { return nil }
