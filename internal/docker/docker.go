package docker

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// PortLabels queries the local Docker daemon via Unix socket and returns a map
// from host port number to a human-readable label of the form "name (image)".
// The second return value is false when Docker is not accessible — in that case
// the map is nil and the caller should skip enrichment entirely.
//
// Exactly one HTTP request is made regardless of how many ports are occupied
// by containers. All errors are absorbed; this function never fails the caller.
func PortLabels() (map[uint32]string, bool) {
	sock := socketPath()
	if sock == "" {
		return nil, false
	}

	containers, err := fetchContainers(sock)
	if err != nil {
		return nil, false
	}

	labels := make(map[uint32]string, len(containers))
	for _, c := range containers {
		name := ""
		if len(c.Names) > 0 {
			name = strings.TrimPrefix(c.Names[0], "/")
		}
		label := name + " (" + c.Image + ")"

		for _, p := range c.Ports {
			if p.Type == "tcp" && p.PublicPort > 0 {
				labels[uint32(p.PublicPort)] = label
			}
		}
	}

	return labels, true
}

// socketPath returns the first Docker Unix socket path that is accessible on
// disk. It respects DOCKER_HOST when set to a unix:// URL and does not fall
// through to the default candidates in that case.
func socketPath() string {
	if host := os.Getenv("DOCKER_HOST"); host != "" {
		if strings.HasPrefix(host, "unix://") {
			path := strings.TrimPrefix(host, "unix://")
			if _, err := os.Stat(path); err == nil {
				return path
			}
		}
		// DOCKER_HOST is set (but not reachable or not a unix socket) — do not fall through.
		return ""
	}

	candidates := []string{
		"/var/run/docker.sock",
		filepath.Join(os.Getenv("HOME"), ".docker/run/docker.sock"),
	}
	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// fetchContainers calls GET /containers/json on the Docker daemon socket and
// returns the decoded container list.
func fetchContainers(sockPath string) ([]containerJSON, error) {
	client := newSocketClient(sockPath)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://localhost/containers/json", nil)
	if err != nil {
		return nil, err
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var containers []containerJSON
	if err := json.NewDecoder(resp.Body).Decode(&containers); err != nil {
		return nil, err
	}
	return containers, nil
}

// newSocketClient builds an *http.Client that routes all requests through the
// given Unix domain socket, ignoring the host part of the URL.
func newSocketClient(sockPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", sockPath)
			},
		},
	}
}

// containerJSON is the minimal shape of each element returned by /containers/json.
type containerJSON struct {
	Names []string     `json:"Names"`
	Image string       `json:"Image"`
	Ports []portBinding `json:"Ports"`
}

// portBinding mirrors the Ports array element from the Docker API response.
type portBinding struct {
	PublicPort uint16 `json:"PublicPort"`
	Type       string `json:"Type"`
}
