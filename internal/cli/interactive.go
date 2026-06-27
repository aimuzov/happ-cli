package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"syscall"

	"github.com/mattn/go-isatty"

	"github.com/aimuzov/happ-cli/internal/link"
	"github.com/aimuzov/happ-cli/internal/store"
	"github.com/aimuzov/happ-cli/internal/xray"
)

// Default local proxy ports, shared with the connect command's flag defaults.
const (
	defaultSocksPort = 10808
	defaultHTTPPort  = 10809
)

// errAborted signals the user cancelled an interactive prompt (Esc / Ctrl+C).
// It is converted to a clean exit by runInteractive.
var errAborted = errors.New("cancelled")

// method is an interactive connection choice: the connect mode plus whether to
// also set the system proxy.
type method struct {
	Mode        string // "proxy" or "tun"
	SystemProxy bool
}

// needsRoot reports whether the method requires elevated privileges.
func (m method) needsRoot() bool {
	return m.Mode == "tun" || m.SystemProxy
}

// methodChoice pairs a method with its menu label.
type methodChoice struct {
	Label  string
	Method method
}

// methodChoices is the fixed set of connection methods offered interactively.
func methodChoices() []methodChoice {
	return []methodChoice{
		{"Local proxy + system proxy — sudo", method{Mode: "proxy", SystemProxy: true}},
		{"Local proxy — SOCKS5 + HTTP, no sudo", method{Mode: "proxy"}},
		{"TUN tunnel — route all traffic, sudo", method{Mode: "tun"}},
	}
}

// serverChoice is a selectable server: its original 0-based index and a label.
type serverChoice struct {
	Index int
	Label string
}

// supportedServerChoices returns choices for the servers xray-core can dial,
// preserving each server's original index so a later selector stays correct.
func supportedServerChoices(servers []*link.Server) []serverChoice {
	out := make([]serverChoice, 0, len(servers))
	for i, s := range servers {
		if !xray.Supported(s.Protocol) {
			continue
		}
		out = append(out, serverChoice{
			Index: i,
			Label: fmt.Sprintf("%s  [%s]  %s:%d", s.Tag, s.Protocol, s.Address, s.Port),
		})
	}
	return out
}

// buildSudoArgs builds the argv for re-executing happ under sudo as the
// equivalent non-interactive connect command. argv[0] is "sudo".
func buildSudoArgs(self, home, subName string, idx int, m method) []string {
	args := []string{"sudo", self, "connect", strconv.Itoa(idx + 1), "--mode", m.Mode}
	if subName != "" {
		args = append(args, "--sub", subName)
	}
	if home != "" {
		args = append(args, "--home", home)
	}
	if m.SystemProxy {
		args = append(args, "--system-proxy")
	}
	return args
}

// runInteractive drives the interactive flow: pick subscription, server and
// method, then either run in-process or re-exec under sudo when root is needed.
func runInteractive(ctx context.Context) error {
	if err := interactiveFlow(ctx); err != nil {
		if errors.Is(err, errAborted) {
			return nil
		}
		return err
	}
	return nil
}

func interactiveFlow(ctx context.Context) error {
	if !isatty.IsTerminal(os.Stdin.Fd()) && !isatty.IsCygwinTerminal(os.Stdin.Fd()) {
		return fmt.Errorf("interactive mode requires a terminal; use 'happ connect' instead")
	}

	st, err := openStore()
	if err != nil {
		return err
	}
	sub, err := pickSubscription(st)
	if err != nil {
		return err
	}
	srv, idx, err := pickServer(sub.Servers())
	if err != nil {
		return err
	}
	m, err := pickMethod()
	if err != nil {
		return err
	}

	if m.needsRoot() && os.Geteuid() != 0 {
		self, err := os.Executable()
		if err != nil {
			return fmt.Errorf("locate happ binary for re-exec: %w", err)
		}
		dir, err := storeDir()
		if err != nil {
			return err
		}
		return reexecWithSudo(self, dir, sub.Name, idx, m)
	}

	fmt.Printf("Server #%d: %s [%s] %s:%d\n", idx+1, srv.Tag, srv.Protocol, srv.Address, srv.Port)
	if m.Mode == "tun" {
		return runTun(ctx, srv, defaultSocksPort)
	}
	return runProxy(ctx, srv, defaultSocksPort, defaultHTTPPort, m.SystemProxy)
}

// reexecWithSudo replaces the current process with `sudo happ connect ...`.
// On success it never returns; on failure it returns the exec error.
func reexecWithSudo(self, home, subName string, idx int, m method) error {
	sudoPath, err := exec.LookPath("sudo")
	if err != nil {
		return fmt.Errorf("cannot elevate: sudo not found in PATH: %w", err)
	}
	argv := buildSudoArgs(self, home, subName, idx, m)
	fmt.Printf("Elevating with sudo for %s mode...\n", m.Mode)
	return syscall.Exec(sudoPath, argv, os.Environ())
}

func pickSubscription(st *store.Store) (store.SubEntry, error) {
	subs := st.Subscriptions()
	if len(subs) == 0 {
		return store.SubEntry{}, fmt.Errorf("no subscriptions; add one with 'happ sub add <url>'")
	}
	if len(subs) == 1 {
		return subs[0], nil
	}

	active := st.Active()
	labels := make([]string, len(subs))
	for i, s := range subs {
		label := s.Name
		if s.Title != "" {
			label = fmt.Sprintf("%s (%s)", s.Name, s.Title)
		}
		if s.Name == active {
			label += " *"
		}
		labels[i] = label
	}

	i, err := fuzzySelect("Subscription", labels)
	if err != nil {
		return store.SubEntry{}, err
	}
	return subs[i], nil
}

func pickServer(servers []*link.Server) (*link.Server, int, error) {
	choices := supportedServerChoices(servers)
	if len(choices) == 0 {
		return nil, 0, fmt.Errorf("no servers that xray-core can dial in this subscription")
	}

	labels := make([]string, len(choices))
	for i, c := range choices {
		labels[i] = c.Label
	}

	i, err := fuzzySelect("Server", labels)
	if err != nil {
		return nil, 0, err
	}
	idx := choices[i].Index
	return servers[idx], idx, nil
}

func pickMethod() (method, error) {
	choices := methodChoices()
	labels := make([]string, len(choices))
	for i, c := range choices {
		labels[i] = c.Label
	}

	i, err := fuzzySelect("Method", labels)
	if err != nil {
		return method{}, err
	}
	return choices[i].Method, nil
}
