package ports

import "testing"

func TestIsDockerProcess(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		process string
		want    bool
	}{
		{name: "macos docker desktop", process: "Docker", want: true},
		{name: "linux docker proxy", process: "docker-proxy", want: true},
		{name: "postgres", process: "postgres", want: false},
		{name: "empty string", process: "", want: false},
		{name: "lowercase docker is not a match", process: "docker", want: false},
		{name: "partial match docker-something", process: "docker-something", want: false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := isDockerProcess(tc.process)
			if got != tc.want {
				t.Errorf("isDockerProcess(%q) = %v, want %v", tc.process, got, tc.want)
			}
		})
	}
}

func TestNormalizeAddr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		addr string
		kind string
		want string
	}{
		// macOS wildcard
		{name: "macos wildcard tcp4", addr: "*", kind: "tcp4", want: "0.0.0.0"},
		{name: "macos wildcard tcp6", addr: "*", kind: "tcp6", want: "::"},

		// Linux wildcards passed through as-is (idempotent)
		{name: "linux wildcard tcp4", addr: "0.0.0.0", kind: "tcp4", want: "0.0.0.0"},
		{name: "linux wildcard tcp6", addr: "::", kind: "tcp6", want: "::"},

		// Cross-family normalisation (defensive: wildcard on wrong family)
		{name: "cross family :: on tcp4", addr: "::", kind: "tcp4", want: "0.0.0.0"},
		{name: "cross family 0.0.0.0 on tcp6", addr: "0.0.0.0", kind: "tcp6", want: "::"},

		// Empty string
		{name: "empty tcp4", addr: "", kind: "tcp4", want: "0.0.0.0"},
		{name: "empty tcp6", addr: "", kind: "tcp6", want: "::"},

		// Specific addresses must pass through unchanged
		{name: "loopback ipv4", addr: "127.0.0.1", kind: "tcp4", want: "127.0.0.1"},
		{name: "loopback ipv6", addr: "::1", kind: "tcp6", want: "::1"},
		{name: "specific ipv4", addr: "192.168.1.100", kind: "tcp4", want: "192.168.1.100"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeAddr(tc.addr, tc.kind)
			if got != tc.want {
				t.Errorf("normalizeAddr(%q, %q) = %q, want %q", tc.addr, tc.kind, got, tc.want)
			}
		})
	}
}

func TestAppBundleName(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		exePath string
		want    string
	}{
		{
			name:    "nested helper inside outer bundle",
			exePath: "/Applications/IntelliJ IDEA CE.app/Contents/jbr/Contents/Frameworks/cef_server.app/Contents/MacOS/cef_server",
			want:    "IntelliJ IDEA CE",
		},
		{
			name:    "direct bundle executable",
			exePath: "/Applications/Google Drive.app/Contents/MacOS/Google Drive",
			want:    "Google Drive",
		},
		{
			name:    "app in system frameworks",
			exePath: "/System/Library/CoreServices/ControlCenter.app/Contents/MacOS/ControlCenter",
			want:    "ControlCenter",
		},
		{
			name:    "non-bundle linux binary",
			exePath: "/usr/bin/postgres",
			want:    "",
		},
		{
			name:    "non-bundle usr libexec",
			exePath: "/usr/libexec/rapportd",
			want:    "",
		},
		{
			name:    "empty path",
			exePath: "",
			want:    "",
		},
		{
			name:    "path with .app in filename but not a bundle",
			exePath: "/usr/local/bin/myapp",
			want:    "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := appBundleName(tc.exePath)
			if got != tc.want {
				t.Errorf("appBundleName(%q) = %q, want %q", tc.exePath, got, tc.want)
			}
		})
	}
}
