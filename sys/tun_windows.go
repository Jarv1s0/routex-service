//go:build windows

package sys

import (
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

var (
	iphlpapi                  = windows.NewLazySystemDLL("iphlpapi.dll")
	procDeleteIpForwardEntry2 = iphlpapi.NewProc("DeleteIpForwardEntry2")
)

var errTunAdapterNotFound = errors.New("找不到 TUN 网卡")

var netClassGUID = &windows.GUID{
	Data1: 0x4d36e972,
	Data2: 0xe325,
	Data3: 0x11ce,
	Data4: [8]byte{0xbf, 0xc1, 0x08, 0x00, 0x2b, 0xe1, 0x03, 0x18},
}

type tunAdapterInfo struct {
	name             string
	description      string
	netCfgInstanceID string
	index            uint32
	hasFakeAddress   bool
	hasFakeDNS       bool
}

func parseFakeIPRanges(values []string) ([]*net.IPNet, error) {
	ranges := make([]*net.IPNet, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		_, ipNet, err := net.ParseCIDR(value)
		if err != nil {
			return nil, fmt.Errorf("无效的 fake-ip-range %q: %w", value, err)
		}
		ranges = append(ranges, ipNet)
	}
	if len(ranges) == 0 {
		return nil, errors.New("fake-ip-range 不能为空")
	}
	return ranges, nil
}

func ipInRanges(ip net.IP, ranges []*net.IPNet) bool {
	if ip == nil {
		return false
	}
	for _, ipNet := range ranges {
		if ipNet.Contains(ip) {
			return true
		}
	}
	return false
}

func sockaddrInetIP(addr windows.RawSockaddrInet) net.IP {
	switch addr.Family {
	case windows.AF_INET:
		raw := (*windows.RawSockaddrInet4)(unsafe.Pointer(&addr))
		return net.IPv4(raw.Addr[0], raw.Addr[1], raw.Addr[2], raw.Addr[3])
	case windows.AF_INET6:
		raw := (*windows.RawSockaddrInet6)(unsafe.Pointer(&addr))
		return net.IP(raw.Addr[:])
	default:
		return nil
	}
}

func getTunAdapterInfo(device string, ranges []*net.IPNet) (*tunAdapterInfo, error) {
	if strings.TrimSpace(device) == "" {
		return nil, errors.New("必须提供 TUN 网卡名称")
	}

	var size uint32
	err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, 0, 0, nil, &size)
	if err != nil && !errors.Is(err, windows.ERROR_BUFFER_OVERFLOW) {
		return nil, err
	}
	if size == 0 {
		return nil, fmt.Errorf("%w: %s", errTunAdapterNotFound, device)
	}

	buffer := make([]byte, size)
	first := (*windows.IpAdapterAddresses)(unsafe.Pointer(&buffer[0]))
	if err := windows.GetAdaptersAddresses(windows.AF_UNSPEC, 0, 0, first, &size); err != nil {
		return nil, err
	}

	for adapter := first; adapter != nil; adapter = adapter.Next {
		name := windows.UTF16PtrToString(adapter.FriendlyName)
		if !strings.EqualFold(name, device) {
			continue
		}

		info := &tunAdapterInfo{
			name:             name,
			description:      windows.UTF16PtrToString(adapter.Description),
			netCfgInstanceID: windows.BytePtrToString(adapter.AdapterName),
			index:            adapter.IfIndex,
		}
		for addr := adapter.FirstUnicastAddress; addr != nil; addr = addr.Next {
			if ipInRanges(addr.Address.IP(), ranges) {
				info.hasFakeAddress = true
				break
			}
		}
		for dns := adapter.FirstDnsServerAddress; dns != nil; dns = dns.Next {
			if ipInRanges(dns.Address.IP(), ranges) {
				info.hasFakeDNS = true
				break
			}
		}
		return info, nil
	}

	return nil, fmt.Errorf("%w: %s", errTunAdapterNotFound, device)
}

func deleteIpForwardEntry2(row *windows.MibIpForwardRow2) error {
	if err := iphlpapi.Load(); err != nil {
		return err
	}
	result, _, _ := procDeleteIpForwardEntry2.Call(uintptr(unsafe.Pointer(row)))
	if result == 0 {
		return nil
	}
	return syscall.Errno(result)
}

func cleanupTunRoutes(index uint32, ranges []*net.IPNet, allowDefaultRoute bool) (int, bool, error) {
	var table *windows.MibIpForwardTable2
	if err := windows.GetIpForwardTable2(windows.AF_INET, &table); err != nil {
		return 0, false, err
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))

	removed := 0
	matched := false
	for _, row := range table.Rows() {
		if row.InterfaceIndex != index {
			continue
		}

		destination := sockaddrInetIP(row.DestinationPrefix.Prefix)
		nextHop := sockaddrInetIP(row.NextHop)
		isDefaultRoute := allowDefaultRoute &&
			row.DestinationPrefix.PrefixLength == 0 &&
			destination.To4() != nil &&
			destination.Equal(net.IPv4(0, 0, 0, 0))
		isFakeRoute := ipInRanges(destination, ranges) || ipInRanges(nextHop, ranges)
		if !isDefaultRoute && !isFakeRoute {
			continue
		}

		matched = true
		if err := deleteIpForwardEntry2(&row); err != nil {
			return removed, matched, err
		}
		removed++
	}
	return removed, matched, nil
}

func runNetsh(args ...string) error {
	output, err := exec.Command("netsh", args...).CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, text)
	}
	return nil
}

func resetInterfaceDNS(device string) error {
	return runNetsh("interface", "ipv4", "set", "dnsservers", "name="+device, "source=dhcp")
}

func flushDNSCache() {
	_ = exec.Command("ipconfig", "/flushdns").Run()
}

func isKnownTunAdapter(adapter tunAdapterInfo) bool {
	value := strings.ToLower(adapter.name + " " + adapter.description)
	return strings.Contains(value, "wintun") ||
		strings.Contains(value, "mihomo") ||
		strings.Contains(value, "clash")
}

func normalizeNetCfgInstanceID(value string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(value), "{}"))
}

func deviceNetCfgInstanceID(devInfo windows.DevInfo, devInfoData *windows.DevInfoData) (string, error) {
	keyHandle, err := devInfo.OpenDevRegKey(
		devInfoData,
		windows.DICS_FLAG_GLOBAL,
		0,
		windows.DIREG_DRV,
		uint32(registry.QUERY_VALUE),
	)
	if err != nil {
		return "", err
	}
	key := registry.Key(keyHandle)
	defer key.Close()

	value, _, err := key.GetStringValue("NetCfgInstanceId")
	if err != nil {
		return "", err
	}
	return value, nil
}

func removeDevice(devInfo windows.DevInfo, devInfoData *windows.DevInfoData) error {
	params := windows.RemoveDeviceParams{
		ClassInstallHeader: *windows.MakeClassInstallHeader(windows.DIF_REMOVE),
		Scope:              windows.DI_REMOVEDEVICE_GLOBAL,
	}
	if err := devInfo.SetClassInstallParams(
		devInfoData,
		&params.ClassInstallHeader,
		uint32(unsafe.Sizeof(params)),
	); err != nil {
		return err
	}
	return devInfo.CallClassInstaller(windows.DIF_REMOVE, devInfoData)
}

func removeTunAdapter(adapter tunAdapterInfo) error {
	targetID := normalizeNetCfgInstanceID(adapter.netCfgInstanceID)
	if targetID == "" {
		return errors.New("TUN 网卡缺少 NetCfgInstanceId")
	}

	devInfo, err := windows.SetupDiGetClassDevsEx(
		netClassGUID,
		"",
		0,
		windows.DIGCF_PRESENT,
		0,
		"",
	)
	if err != nil {
		return err
	}
	defer devInfo.Close()

	for i := 0; ; i++ {
		devInfoData, err := devInfo.EnumDeviceInfo(i)
		if err != nil {
			if errors.Is(err, windows.ERROR_NO_MORE_ITEMS) {
				break
			}
			continue
		}

		netCfgInstanceID, err := deviceNetCfgInstanceID(devInfo, devInfoData)
		if err != nil {
			continue
		}
		if normalizeNetCfgInstanceID(netCfgInstanceID) != targetID {
			continue
		}
		return removeDevice(devInfo, devInfoData)
	}

	return fmt.Errorf("找不到 TUN 设备实例: %s", adapter.name)
}

func cleanupTunWindows(opts TunCleanupOptions) (*TunCleanupResult, error) {
	ranges, err := parseFakeIPRanges(opts.FakeIPRanges)
	if err != nil {
		return nil, err
	}

	adapter, err := getTunAdapterInfo(opts.Device, ranges)
	if err != nil {
		if errors.Is(err, errTunAdapterNotFound) {
			return &TunCleanupResult{Device: opts.Device}, nil
		}
		return nil, err
	}

	isTunAdapter := isKnownTunAdapter(*adapter)
	removedRoutes, hasRoute, err := cleanupTunRoutes(
		adapter.index,
		ranges,
		adapter.hasFakeAddress || adapter.hasFakeDNS || isTunAdapter,
	)
	if err != nil {
		return nil, err
	}

	result := &TunCleanupResult{
		Device:         adapter.name,
		InterfaceIndex: adapter.index,
		Matched:        adapter.hasFakeAddress || adapter.hasFakeDNS || hasRoute || isTunAdapter,
		RoutesRemoved:  removedRoutes,
	}
	if !result.Matched {
		return result, nil
	}

	if adapter.hasFakeDNS {
		if err := resetInterfaceDNS(adapter.name); err != nil {
			return result, err
		}
		result.DNSReset = true
	}

	if err := removeTunAdapter(*adapter); err != nil {
		return result, err
	}
	result.AdapterRemoved = true
	flushDNSCache()
	return result, nil
}
