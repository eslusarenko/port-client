package ports

import (
	"fmt"
	"sort"
	"strings"

	"github.com/eslusarenko/port-client/internal/docker"
	gopsnet "github.com/shirou/gopsutil/v4/net"
	"github.com/shirou/gopsutil/v4/process"
)

// Entry represents a single listening port.
type Entry struct {
	Proto     string // "tcp4" or "tcp6"
	LocalAddr string
	Port      uint32
	PID       int32
	Process   string // human-friendly process name, "-" if unavailable
	CmdLine   string // full executable path with arguments, "-" if unavailable
}

// ListListening returns all TCP ports currently in LISTEN state, sorted by port then proto.
func ListListening() ([]Entry, error) {
	// Fetch Docker container labels once up front; used for all entries below.
	dockerLabels, dockerAvail := docker.PortLabels()

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

			name, cmdLine := lookupProcessInfo(c.Pid)

			if dockerAvail && isDockerProcess(name) {
				if label, ok := dockerLabels[c.Laddr.Port]; ok {
					name = "Docker [" + label + "]"
				}
			}

			entries = append(entries, Entry{
				Proto:     kind,
				LocalAddr: normalizeAddr(c.Laddr.IP, kind),
				Port:      c.Laddr.Port,
				PID:       c.Pid,
				Process:   name,
				CmdLine:   cmdLine,
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

// normalizeAddr returns the canonical wildcard address for the given family
// when addr is any known wildcard representation, or the original addr otherwise.
// Handles macOS ("*"), Linux ("0.0.0.0" / "::"), and empty string defensively.
func normalizeAddr(addr, kind string) string {
	switch addr {
	case "", "*", "0.0.0.0", "::":
		if kind == "tcp6" {
			return "::"
		}
		return "0.0.0.0"
	default:
		return addr
	}
}

// lookupProcessInfo returns a human-friendly display name and the full command line
// for the given PID. Both values fall back to "-" if unavailable.
// A single process.Process object is created to avoid redundant syscalls.
func lookupProcessInfo(pid int32) (name, cmdLine string) {
	if pid == 0 {
		return "-", "-"
	}

	p, err := process.NewProcess(pid)
	if err != nil {
		return "-", "-"
	}

	// Resolve display name: prefer .app bundle name on macOS, fall back to raw name.
	if exe, err := p.Exe(); err == nil {
		if appName := appBundleName(exe); appName != "" {
			name = appName
		}
	}
	if name == "" {
		if n, err := p.Name(); err == nil {
			name = n
		} else {
			name = "-"
		}
	}

	// Full command line with arguments.
	if cl, err := p.Cmdline(); err == nil && cl != "" {
		cmdLine = cl
	} else {
		cmdLine = "-"
	}

	return name, cmdLine
}

// isDockerProcess reports whether the process name corresponds to the Docker
// host-port proxy on the current platform.
//   - macOS Docker Desktop: bundle name resolves to "Docker"
//   - Linux: the proxy binary is named "docker-proxy"
func isDockerProcess(name string) bool {
	return name == "Docker" || name == "docker-proxy"
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
