package ports

import (
	"fmt"
	"sort"

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

// lookupProcess returns the process name for the given PID, or "-" if unavailable.
func lookupProcess(pid int32) string {
	if pid == 0 {
		return "-"
	}
	p, err := process.NewProcess(pid)
	if err != nil {
		return "-"
	}
	name, err := p.Name()
	if err != nil {
		return "-"
	}
	return name
}
