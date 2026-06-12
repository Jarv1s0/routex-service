package sys

import (
	"fmt"
	"runtime"
)

const defaultTunFakeIPRange = "198.18.0.0/15"

type TunCleanupOptions struct {
	Device       string   `json:"device,omitempty"`
	FakeIPRanges []string `json:"fake_ip_ranges,omitempty"`
}

type TunCleanupResult struct {
	Device          string `json:"device,omitempty"`
	InterfaceIndex  uint32 `json:"interface_index,omitempty"`
	Matched         bool   `json:"matched"`
	RoutesRemoved   int    `json:"routes_removed"`
	DNSReset        bool   `json:"dns_reset"`
	AdapterDisabled bool   `json:"adapter_disabled"`
	AdapterRemoved  bool   `json:"adapter_removed"`
}

func normalizeTunCleanupOptions(opts TunCleanupOptions) TunCleanupOptions {
	if len(opts.FakeIPRanges) == 0 {
		opts.FakeIPRanges = []string{defaultTunFakeIPRange}
	}
	return opts
}

func CleanupTun(opts TunCleanupOptions) (*TunCleanupResult, error) {
	opts = normalizeTunCleanupOptions(opts)
	switch runtime.GOOS {
	case "windows":
		return cleanupTunWindows(opts)
	default:
		return nil, fmt.Errorf("不支持的操作系统: %s", runtime.GOOS)
	}
}
