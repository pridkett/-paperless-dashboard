// Command paperless-dashboard serves a single-page dashboard showing whether
// recurring documents (bills, statements, tax filings) are up to date in a
// Paperless-NGX instance.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/pridkett/paperless-dashboard/internal/checks"
	"github.com/pridkett/paperless-dashboard/internal/config"
	"github.com/pridkett/paperless-dashboard/internal/paperless"
	"github.com/pridkett/paperless-dashboard/internal/web"
)

// ANSI styles, disabled when stdout is not a terminal or NO_COLOR is set.
var (
	bold   = style("\033[1m")
	dim    = style("\033[2m")
	green  = style("\033[32m")
	yellow = style("\033[33m")
	red    = style("\033[31m")
	cyan   = style("\033[36m")
	reset  = style("\033[0m")
)

func style(code string) string {
	if os.Getenv("NO_COLOR") != "" {
		return ""
	}
	if fi, err := os.Stdout.Stat(); err != nil || fi.Mode()&os.ModeCharDevice == 0 {
		return ""
	}
	return code
}

func main() {
	var flags config.Flags
	flag.StringVar(&flags.ConfigPath, "config", "", "path to TOML config file (default \"config.toml\")")
	flag.StringVar(&flags.URL, "url", "", "Paperless-NGX base URL")
	flag.StringVar(&flags.Token, "token", "", "Paperless-NGX API token")
	flag.StringVar(&flags.Listen, "listen", "", "address to serve the dashboard on")
	flag.Parse()

	if err := run(flags); err != nil {
		fmt.Fprintf(os.Stderr, "%s✗ %v%s\n", red, err, reset)
		os.Exit(1)
	}
}

func run(flags config.Flags) error {
	cfg, err := config.Load(flags)
	if err != nil {
		return err
	}

	fmt.Printf("%s%s📄 paperless-dashboard%s\n\n", bold, cyan, reset)
	for _, w := range cfg.Warnings {
		fmt.Printf("%s⚠ %s%s\n", yellow, w, reset)
	}
	if len(cfg.Warnings) > 0 {
		fmt.Println()
	}

	printSetting("Paperless URL", cfg.URL)
	printSetting("Listen address", cfg.Listen)
	printSetting("API token", config.Setting{Value: mask(cfg.Token.Value), Source: cfg.Token.Source})
	fmt.Printf("  %-16s %s%d configured%s\n", "Checks", bold, len(cfg.Checks), reset)
	fmt.Printf("  %-16s %severy %d minutes%s\n\n", "Refresh", dim, cfg.RefreshMinutes, reset)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	client := paperless.New(cfg.URL.Value, cfg.Token.Value)
	if err := client.Ping(ctx); err != nil {
		return fmt.Errorf("cannot reach Paperless at %s: %w", cfg.URL.Value, err)
	}
	fmt.Printf("%s✓ connected to Paperless%s\n", green, reset)
	fmt.Printf("%s✓ dashboard on http://%s%s\n", green, cfg.Listen.Value, reset)

	srv := web.New(&checks.Engine{Client: client}, cfg)
	return srv.Run(ctx)
}

func printSetting(label string, s config.Setting) {
	note := ""
	if s.Source != config.SourceDefault {
		note = fmt.Sprintf(" %s(from %s)%s", dim, s.Source, reset)
	}
	fmt.Printf("  %-16s %s%s%s%s\n", label, bold, s.Value, reset, note)
}

func mask(token string) string {
	if len(token) <= 4 {
		return "****"
	}
	return "****" + token[len(token)-4:]
}
