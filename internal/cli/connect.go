package cli

import (
	"context"
	"fmt"
	"net"

	"github.com/spf13/cobra"

	"github.com/aimuzov/happ-cli/internal/link"
	"github.com/aimuzov/happ-cli/internal/sysproxy"
	"github.com/aimuzov/happ-cli/internal/tunnel"
	"github.com/aimuzov/happ-cli/internal/xray"
)

func newConnectCmd() *cobra.Command {
	var mode, subName string
	var socksPort, httpPort int
	var systemProxy bool
	cmd := &cobra.Command{
		Use:     "connect [selector]",
		Aliases: []string{"up"},
		Short:   "Connect to a server (proxy or full TUN tunnel)",
		Long: "Connect runs in the foreground until interrupted (Ctrl+C).\n\n" +
			"selector picks the server: empty = first, a number = 1-based index,\n" +
			"or a case-insensitive substring of the server tag.\n\n" +
			"Modes:\n" +
			"  proxy  local SOCKS5 + HTTP proxy on 127.0.0.1 (no root)\n" +
			"  tun    system-wide VPN via a utun device (requires sudo, macOS)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeServerSelector,
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := openStore()
			if err != nil {
				return err
			}
			sub, err := resolveSub(st, subName)
			if err != nil {
				return err
			}
			selector := ""
			if len(args) > 0 {
				selector = args[0]
			}
			srv, idx, err := selectServer(sub.Servers(), selector)
			if err != nil {
				return err
			}
			if !xray.Supported(srv.Protocol) {
				return fmt.Errorf("server #%d uses %q, which xray-core cannot dial; pick another server", idx+1, srv.Protocol)
			}

			fmt.Printf("Server #%d: %s [%s] %s:%d\n", idx+1, srv.Tag, srv.Protocol, srv.Address, srv.Port)

			switch mode {
			case "proxy":
				return runProxy(cmd.Context(), srv, socksPort, httpPort, systemProxy)
			case "tun":
				return runTun(cmd.Context(), srv, socksPort)
			default:
				return fmt.Errorf("unknown mode %q (use 'proxy' or 'tun')", mode)
			}
		},
	}
	cmd.Flags().StringVarP(&mode, "mode", "m", "proxy", "connection mode: proxy or tun")
	cmd.Flags().IntVar(&socksPort, "socks", defaultSocksPort, "local SOCKS5 port")
	cmd.Flags().IntVar(&httpPort, "http", defaultHTTPPort, "local HTTP proxy port (proxy mode)")
	cmd.Flags().StringVar(&subName, "sub", "", "subscription name (default: active)")
	cmd.Flags().BoolVar(&systemProxy, "system-proxy", false, "set the macOS system SOCKS proxy (requires sudo, proxy mode)")
	_ = cmd.RegisterFlagCompletionFunc("sub", completeSubFlag)
	_ = cmd.RegisterFlagCompletionFunc("mode", func(*cobra.Command, []string, string) ([]cobra.Completion, cobra.ShellCompDirective) {
		return []cobra.Completion{
			cobra.CompletionWithDesc("proxy", "local SOCKS5 + HTTP proxy (no root)"),
			cobra.CompletionWithDesc("tun", "system-wide VPN via utun (sudo)"),
		}, cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

func runProxy(ctx context.Context, srv *link.Server, socksPort, httpPort int, systemProxy bool) error {
	cfg, err := xray.BuildConfig(srv, xray.Options{SocksPort: socksPort, HTTPPort: httpPort})
	if err != nil {
		return err
	}
	raw, err := cfg.JSON()
	if err != nil {
		return err
	}
	inst, err := xray.Start(raw)
	if err != nil {
		return err
	}
	defer inst.Close()

	fmt.Printf("Proxy is up:\n  SOCKS5  127.0.0.1:%d\n  HTTP    127.0.0.1:%d\n", socksPort, httpPort)

	if systemProxy {
		restore, err := sysproxy.Enable("127.0.0.1", socksPort, httpPort)
		if err != nil {
			return fmt.Errorf("enable system proxy: %w", err)
		}
		defer func() {
			if err := restore(); err != nil {
				fmt.Println("warning: failed to restore system proxy:", err)
			}
		}()
		fmt.Println("System SOCKS/HTTP proxy set on all network services (will be restored on exit).")
	}

	fmt.Println("Press Ctrl+C to disconnect.")
	<-ctx.Done()
	fmt.Println("\nDisconnecting...")
	return nil
}

func runTun(ctx context.Context, srv *link.Server, socksPort int) error {
	ips, err := resolveIPv4(srv.Address)
	if err != nil {
		return fmt.Errorf("resolve server address %q: %w", srv.Address, err)
	}

	// Pin the outbound to a concrete IP and keep the TLS SNI on the domain, so
	// xray dials the exact IP we route around the tunnel.
	pinned := *srv
	if pinned.SNI == "" {
		pinned.SNI = srv.Address
	}
	pinned.Address = ips[0]

	cfg, err := xray.BuildConfig(&pinned, xray.Options{SocksPort: socksPort})
	if err != nil {
		return err
	}
	raw, err := cfg.JSON()
	if err != nil {
		return err
	}
	inst, err := xray.Start(raw)
	if err != nil {
		return err
	}
	defer inst.Close()

	tun, err := tunnel.Start(tunnel.Options{
		SocksAddr: fmt.Sprintf("127.0.0.1:%d", socksPort),
		ServerIPs: ips,
	})
	if err != nil {
		return err
	}
	defer tun.Close()

	fmt.Printf("TUN tunnel is up; all traffic is routed through %s.\n", srv.Tag)
	fmt.Println("Press Ctrl+C to disconnect and restore routing.")
	<-ctx.Done()
	fmt.Println("\nDisconnecting and restoring routes...")
	return nil
}

// resolveIPv4 returns the IPv4 addresses for host, or host itself if it is one.
func resolveIPv4(host string) ([]string, error) {
	if ip := net.ParseIP(host); ip != nil {
		if ip.To4() == nil {
			return nil, fmt.Errorf("IPv6 server addresses are not supported in TUN mode yet")
		}
		return []string{host}, nil
	}
	addrs, err := net.LookupIP(host)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, a := range addrs {
		if v4 := a.To4(); v4 != nil {
			out = append(out, v4.String())
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no IPv4 address found for %q", host)
	}
	return out, nil
}
