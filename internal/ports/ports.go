package ports

import (
	"fmt"
	"sort"
	"strings"

	gopsnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// Entry represents a single listening port.
type Entry struct {
	Proto     string // "tcp4" or "tcp6"
	LocalAddr string
	Port      uint32
	PID       int32
	Process   string // process name, "-" if unknown or unavailable
}

// ListListening returns all TCP ports currently in LISTEN state, sorted by port then proto.
func ListListening() ([]Entry, error) {
	var entries []Entry

	for _, kind := range []string{"tcp4", "tcp6"} {
		conns, err := gopsnet.Connections(kind)
		if err != nil {
			return nil, fmt.Errorf("reading %s connections: %w", kind, err)
		}

		for _, c := range conns {
			if c.Status != "LISTEN" {
				continue
			}

			entries = append(entries, Entry{
				Proto:     kind,
				LocalAddr: normalizeAddr(c.Laddr.IP, kind),
				Port:      c.Laddr.Port,
				PID:       c.Pid,
				Process:   lookupProcess(c.Pid),
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Port != entries[j].Port {
			return entries[i].Port < entries[j].Port
		}
		return entries[i].Proto < entries[j].Proto
	})

	return entries, nil
}

// normalizeAddr replaces the wildcard "*" with the canonical address for the given family.
func normalizeAddr(addr, kind string) string {
	if addr != "*" && addr != "" {
		return addr
	}
	if kind == "tcp6" {
		return "::"
	}
	return "0.0.0.0"
}

// lookupProcess returns a human-friendly process name for the given PID.
// On macOS, if the executable lives inside a .app bundle, the outermost
// bundle name is returned (e.g. "IntelliJ IDEA CE" instead of "cef_server").
// Falls back to the raw process name, or "-" if nothing is available.
func lookupProcess(pid int32) string {
	if pid == 0 {
		return "-"
	}
	p, err := process.NewProcess(pid)
	if err != nil {
		return "-"
	}

	if exe, err := p.Exe(); err == nil {
		if appName := appBundleName(exe); appName != "" {
			return appName
		}
	}

	name, err := p.Name()
	if err != nil {
		return "-"
	}
	return name
}

// appBundleName extracts the outermost macOS .app bundle name from an executable path.
// For example:
//
//	/Applications/IntelliJ IDEA CE.app/Contents/jbr/.../cef_server → "IntelliJ IDEA CE"
//	/Applications/Google Drive.app/Contents/MacOS/Google Drive      → "Google Drive"
//
// Returns an empty string if the path is not inside a .app bundle.
func appBundleName(exePath string) string {
	idx := strings.Index(exePath, ".app/")
	if idx == -1 {
		return ""
	}
	// Walk back to the preceding slash to isolate the bundle directory name.
	segment := exePath[:idx]
	start := strings.LastIndex(segment, "/") + 1
	return segment[start:]
}
