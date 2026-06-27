package cli

import (
	"errors"
	"reflect"
	"testing"

	"github.com/aimuzov/happ-cli/internal/link"
)

func TestBuildSudoArgs(t *testing.T) {
	tests := []struct {
		name    string
		home    string
		subName string
		idx     int
		method  method
		want    []string
	}{
		{
			name:    "tun with sub and home",
			home:    "/cfg",
			subName: "work",
			idx:     1,
			method:  method{Mode: "tun"},
			want:    []string{"sudo", "/bin/happ", "connect", "2", "--mode", "tun", "--sub", "work", "--home", "/cfg"},
		},
		{
			name:    "proxy with system proxy",
			home:    "/cfg",
			subName: "work",
			idx:     0,
			method:  method{Mode: "proxy", SystemProxy: true},
			want:    []string{"sudo", "/bin/happ", "connect", "1", "--mode", "proxy", "--sub", "work", "--home", "/cfg", "--system-proxy"},
		},
		{
			name:    "no sub no home",
			home:    "",
			subName: "",
			idx:     4,
			method:  method{Mode: "tun"},
			want:    []string{"sudo", "/bin/happ", "connect", "5", "--mode", "tun"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildSudoArgs("/bin/happ", tt.home, tt.subName, tt.idx, tt.method)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("buildSudoArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestMethodNeedsRoot(t *testing.T) {
	tests := []struct {
		method method
		want   bool
	}{
		{method{Mode: "proxy"}, false},
		{method{Mode: "proxy", SystemProxy: true}, true},
		{method{Mode: "tun"}, true},
	}
	for _, tt := range tests {
		if got := tt.method.needsRoot(); got != tt.want {
			t.Errorf("needsRoot(%+v) = %v, want %v", tt.method, got, tt.want)
		}
	}
}

func TestSupportedServerChoices(t *testing.T) {
	servers := []*link.Server{
		{Tag: "alpha", Protocol: "vless", Address: "1.1.1.1", Port: 443},
		{Tag: "beta", Protocol: "hysteria2", Address: "2.2.2.2", Port: 8443}, // unsupported
		{Tag: "gamma", Protocol: "trojan", Address: "3.3.3.3", Port: 443},
	}
	got := supportedServerChoices(servers)
	want := []serverChoice{
		{Index: 0, Label: "alpha  [vless]  1.1.1.1:443"},
		{Index: 2, Label: "gamma  [trojan]  3.3.3.3:443"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("supportedServerChoices() = %v, want %v", got, want)
	}
}

func TestParseFzfSelection(t *testing.T) {
	t.Run("valid index from tabbed line", func(t *testing.T) {
		idx, err := parseFzfSelection("2\tgamma  [trojan]  3.3.3.3:443\n", 3)
		if err != nil || idx != 2 {
			t.Fatalf("got (%d, %v), want (2, nil)", idx, err)
		}
	})
	t.Run("empty output is abort", func(t *testing.T) {
		if _, err := parseFzfSelection("", 3); !errors.Is(err, errAborted) {
			t.Fatalf("got %v, want errAborted", err)
		}
	})
	t.Run("out-of-range index errors", func(t *testing.T) {
		if _, err := parseFzfSelection("9\tx\n", 3); err == nil || errors.Is(err, errAborted) {
			t.Fatalf("expected a hard error, got %v", err)
		}
	})
}

func TestMethodChoicesMapping(t *testing.T) {
	choices := methodChoices()
	if len(choices) != 3 {
		t.Fatalf("expected 3 method choices, got %d", len(choices))
	}
	// Exactly one choice runs without root, two require it.
	rootCount := 0
	for _, c := range choices {
		if c.Method.needsRoot() {
			rootCount++
		}
	}
	if rootCount != 2 {
		t.Errorf("expected 2 root-requiring methods, got %d", rootCount)
	}
}
