//go:build !windows

package sys

func cleanupTunWindows(opts TunCleanupOptions) (*TunCleanupResult, error) {
	return nil, nil
}
