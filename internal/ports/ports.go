package ports

import (
	"fmt"
	"sort"

	gopsnet "github.com/shirou/gopsutil/v4/net"
)

// Entry represents a single listening port.
type Entry struct {
	Proto     string
	LocalAddr string
	Port      uint32
}

// ListListening returns all TCP ports currently in LISTEN state, sorted by port number.
func ListListening() ([]Entry, error) {
	conns, err := gopsnet.Connections("tcp")
	if err != nil {
		return nil, fmt.Errorf("reading tcp connections: %w", err)
	}

	var entries []Entry
	for _, c := range conns {
		if c.Status == "LISTEN" {
			entries = append(entries, Entry{
				Proto:     "tcp",
				LocalAddr: c.Laddr.IP,
				Port:      c.Laddr.Port,
			})
		}
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Port < entries[j].Port
	})

	return entries, nil
}
