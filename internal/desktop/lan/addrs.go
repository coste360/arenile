package lan

import (
	"net"
	"net/netip"
	"sort"
	"strconv"

	"github.com/coste360/arenile/internal/desktop"
)

// InterfaceAddrs returns the private (RFC 1918, ULA, link-local) unicast
// addresses of the interfaces that are up and not loopback, IPv4 first.
// Link-local IPv6 addresses carry their zone.
func InterfaceAddrs() ([]netip.Addr, error) {
	ifs, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var out []netip.Addr
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := ifc.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip, ok := netip.AddrFromSlice(ipn.IP)
			if !ok {
				continue
			}
			ip = ip.Unmap()
			if ip.IsLinkLocalUnicast() && ip.Is6() {
				ip = ip.WithZone(ifc.Name)
			}
			if !ip.IsLoopback() && desktop.IsPrivateAddr(ip) {
				out = append(out, ip)
			}
		}
	}
	SortAddrs(out)
	return out, nil
}

// SortAddrs orders addresses for display (pure): RFC 1918 IPv4 first, then
// link-local IPv4, ULA, link-local IPv6; stable within a class.
func SortAddrs(a []netip.Addr) {
	rank := func(x netip.Addr) int {
		switch {
		case x.Is4() && x.IsPrivate():
			return 0
		case x.Is4():
			return 1
		case x.IsPrivate():
			return 2
		default:
			return 3
		}
	}
	sort.SliceStable(a, func(i, j int) bool { return rank(a[i]) < rank(a[j]) })
}

// PanelURL builds the https base URL of the LAN listener (pure): the first
// private IPv4 address, else the .local hostname, else "".
func PanelURL(addrs []netip.Addr, hostname string, port int) string {
	if a, ok := desktop.LANAddress(addrs); ok {
		return "https://" + netip.AddrPortFrom(a, uint16(port)).String() //nolint:gosec // G115: port validated 1024..65535
	}
	if hostname != "" {
		return "https://" + net.JoinHostPort(hostname, strconv.Itoa(port))
	}
	return ""
}
