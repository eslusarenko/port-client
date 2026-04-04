package docker

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// startUnixServer wires an httptest.Server to a Unix socket at a short temp
// path and returns the socket path. The server is closed via t.Cleanup.
// Uses /tmp directly to stay within macOS's 104-char sun_path limit.
func startUnixServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "dtest")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	sockPath := filepath.Join(dir, "d.sock")
	listener, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix %s: %v", sockPath, err)
	}
	srv := httptest.NewUnstartedServer(handler)
	srv.Listener = listener
	srv.Start()
	t.Cleanup(srv.Close)
	return sockPath
}

// containersHandler returns an http.Handler that serves a canned containers JSON list.
func containersHandler(containers []containerJSON) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(containers) //nolint:errcheck // test helper
	})
}

func TestPortLabels_NoSocket(t *testing.T) {
	t.Setenv("DOCKER_HOST", "unix:///nonexistent/path/docker.sock")

	labels, ok := PortLabels()

	if ok {
		t.Error("expected ok=false when socket does not exist")
	}
	if labels != nil {
		t.Errorf("expected nil map, got %v", labels)
	}
}

func TestPortLabels_ContainersWithPorts(t *testing.T) {
	containers := []containerJSON{
		{
			Names: []string{"/my-postgres"},
			Image: "postgres:14",
			Ports: []portBinding{{PublicPort: 5432, Type: "tcp"}},
		},
		{
			Names: []string{"/redis_dev"},
			Image: "redis:alpine",
			Ports: []portBinding{
				{PublicPort: 6379, Type: "tcp"},
				{PublicPort: 16379, Type: "tcp"}, // container with multiple exported ports
			},
		},
	}

	sockPath := startUnixServer(t, containersHandler(containers))
	t.Setenv("DOCKER_HOST", "unix://"+sockPath)

	labels, ok := PortLabels()
	if !ok {
		t.Fatal("expected ok=true with accessible socket")
	}

	tests := []struct {
		port uint32
		want string
	}{
		{5432, "my-postgres (postgres:14)"},
		{6379, "redis_dev (redis:alpine)"},
		{16379, "redis_dev (redis:alpine)"},
	}
	for _, tc := range tests {
		got, found := labels[tc.port]
		if !found {
			t.Errorf("port %d not in labels", tc.port)
			continue
		}
		if got != tc.want {
			t.Errorf("labels[%d] = %q, want %q", tc.port, got, tc.want)
		}
	}
}

func TestPortLabels_EmptyContainerList(t *testing.T) {
	sockPath := startUnixServer(t, containersHandler([]containerJSON{}))
	t.Setenv("DOCKER_HOST", "unix://"+sockPath)

	labels, ok := PortLabels()
	if !ok {
		t.Fatal("expected ok=true with accessible socket")
	}
	if len(labels) != 0 {
		t.Errorf("expected empty map, got %v", labels)
	}
}

func TestPortLabels_MalformedJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("not json {{{")) //nolint:errcheck // test helper
	})
	sockPath := startUnixServer(t, handler)
	t.Setenv("DOCKER_HOST", "unix://"+sockPath)

	labels, ok := PortLabels()
	if ok {
		t.Error("expected ok=false on malformed JSON")
	}
	if labels != nil {
		t.Errorf("expected nil map, got %v", labels)
	}
}

func TestPortLabels_UDPPortsIgnored(t *testing.T) {
	containers := []containerJSON{
		{
			Names: []string{"/dns-server"},
			Image: "coredns/coredns",
			Ports: []portBinding{
				{PublicPort: 53, Type: "udp"},  // must be ignored
				{PublicPort: 5353, Type: "tcp"}, // must appear
			},
		},
	}
	sockPath := startUnixServer(t, containersHandler(containers))
	t.Setenv("DOCKER_HOST", "unix://"+sockPath)

	labels, ok := PortLabels()
	if !ok {
		t.Fatal("expected ok=true")
	}
	if _, found := labels[53]; found {
		t.Error("UDP port 53 should not be in TCP labels map")
	}
	if _, found := labels[5353]; !found {
		t.Error("TCP port 5353 should be in labels map")
	}
}

func TestPortLabels_ContainerNameStripsLeadingSlash(t *testing.T) {
	containers := []containerJSON{
		{
			Names: []string{"/my-app"},
			Image: "nginx:latest",
			Ports: []portBinding{{PublicPort: 80, Type: "tcp"}},
		},
	}
	sockPath := startUnixServer(t, containersHandler(containers))
	t.Setenv("DOCKER_HOST", "unix://"+sockPath)

	labels, _ := PortLabels()
	if got := labels[80]; got != "my-app (nginx:latest)" {
		t.Errorf("labels[80] = %q, want %q", got, "my-app (nginx:latest)")
	}
}
