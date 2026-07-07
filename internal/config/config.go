// Package config loads dashboard configuration with a strict precedence
// order: command-line flag > environment variable > config file > default.
// When a higher-precedence source overrides a lower one, the load records a
// warning so the caller can surface it at startup.
package config

import (
	"errors"
	"fmt"
	"os"

	"github.com/BurntSushi/toml"
)

// Source identifies where a setting's effective value came from.
type Source string

const (
	SourceFlag    Source = "command line flag"
	SourceEnv     Source = "environment variable"
	SourceFile    Source = "configuration file"
	SourceDefault Source = "default"
)

// Defaults.
const (
	DefaultListen  = "127.0.0.1:8985"
	DefaultConfig  = "config.toml"
	DefaultRefresh = 15 // minutes between automatic re-checks
)

// Environment variable names.
const (
	EnvURL    = "PAPERLESS_URL"
	EnvToken  = "PAPERLESS_TOKEN"
	EnvListen = "PAPERLESS_DASHBOARD_LISTEN"
)

// Check is one document-freshness rule from the config file.
type Check struct {
	Name          string   `toml:"name"`
	Correspondent string   `toml:"correspondent"`
	DocumentType  string   `toml:"document_type"`
	Tags          []string `toml:"tags"`
	Frequency     string   `toml:"frequency"`      // monthly, quarterly, yearly
	Lookback      int      `toml:"lookback"`       // number of past periods to inspect
	GraceDays     int      `toml:"grace_days"`     // days into a new period before it counts as missing
	ExpectedMonth int      `toml:"expected_month"` // yearly only: month (1-12) the document is normally filed
	CustomFields  []string `toml:"custom_fields"`  // custom field names to display, e.g. "amount"
}

// fileConfig mirrors the TOML file layout.
type fileConfig struct {
	Paperless struct {
		URL   string `toml:"url"`
		Token string `toml:"token"`
	} `toml:"paperless"`
	Server struct {
		Listen         string `toml:"listen"`
		RefreshMinutes int    `toml:"refresh_minutes"`
	} `toml:"server"`
	Checks []Check `toml:"check"`
}

// Setting is a resolved configuration value plus its provenance.
type Setting struct {
	Value  string
	Source Source
}

// Config is the fully resolved configuration.
type Config struct {
	URL            Setting
	Token          Setting
	Listen         Setting
	RefreshMinutes int
	Checks         []Check
	Warnings       []string
}

// Flags carries raw command-line flag values; empty string means unset.
type Flags struct {
	ConfigPath string
	URL        string
	Token      string
	Listen     string
}

// Load resolves configuration from all sources in precedence order.
func Load(flags Flags) (*Config, error) {
	cfg := &Config{RefreshMinutes: DefaultRefresh}

	path := flags.ConfigPath
	explicit := path != ""
	if !explicit {
		path = DefaultConfig
	}

	var fc fileConfig
	meta, err := toml.DecodeFile(path, &fc)
	switch {
	case err == nil:
		if undecoded := meta.Undecoded(); len(undecoded) > 0 {
			cfg.Warnings = append(cfg.Warnings,
				fmt.Sprintf("config file %s has unrecognized keys: %v", path, undecoded))
		}
	case errors.Is(err, os.ErrNotExist) && !explicit:
		// A missing default config file is fine; env/flags may supply everything.
	default:
		return nil, fmt.Errorf("reading config file %s: %w", path, err)
	}

	if fc.Server.RefreshMinutes > 0 {
		cfg.RefreshMinutes = fc.Server.RefreshMinutes
	}
	cfg.Checks = fc.Checks

	cfg.URL = resolve(cfg, "paperless URL", flags.URL, EnvURL, fc.Paperless.URL, "")
	cfg.Token = resolve(cfg, "paperless API token", flags.Token, EnvToken, fc.Paperless.Token, "")
	cfg.Listen = resolve(cfg, "listen address", flags.Listen, EnvListen, fc.Server.Listen, DefaultListen)

	if err := validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// resolve applies the precedence rules for one setting and records a warning
// when a higher-precedence source shadows a lower one. Overriding a default
// is normal and never warned about.
func resolve(cfg *Config, label, flagVal, envName, fileVal, defVal string) Setting {
	envVal := os.Getenv(envName)

	var shadowed []string
	if flagVal != "" {
		if envVal != "" {
			shadowed = append(shadowed, string(SourceEnv)+" "+envName)
		}
		if fileVal != "" {
			shadowed = append(shadowed, string(SourceFile))
		}
		if len(shadowed) > 0 {
			cfg.Warnings = append(cfg.Warnings, overrideWarning(label, SourceFlag, shadowed))
		}
		return Setting{flagVal, SourceFlag}
	}
	if envVal != "" {
		if fileVal != "" {
			cfg.Warnings = append(cfg.Warnings,
				overrideWarning(label, Source(string(SourceEnv)+" "+envName), []string{string(SourceFile)}))
		}
		return Setting{envVal, SourceEnv}
	}
	if fileVal != "" {
		return Setting{fileVal, SourceFile}
	}
	return Setting{defVal, SourceDefault}
}

func overrideWarning(label string, winner Source, shadowed []string) string {
	msg := fmt.Sprintf("%s is set in multiple places; using the %s instead of the %s", label, winner, shadowed[0])
	if len(shadowed) > 1 {
		msg = fmt.Sprintf("%s is set in multiple places; using the %s instead of the %s and %s",
			label, winner, shadowed[0], shadowed[1])
	}
	return msg
}

func validate(cfg *Config) error {
	if cfg.URL.Value == "" {
		return fmt.Errorf("no Paperless URL configured (set %s, --url, or paperless.url in the config file)", EnvURL)
	}
	if cfg.Token.Value == "" {
		return fmt.Errorf("no Paperless API token configured (set %s, --token, or paperless.token in the config file)", EnvToken)
	}
	for i := range cfg.Checks {
		c := &cfg.Checks[i]
		if c.Name == "" {
			return fmt.Errorf("check #%d has no name", i+1)
		}
		switch c.Frequency {
		case "monthly", "quarterly", "yearly":
		case "":
			c.Frequency = "monthly"
		default:
			return fmt.Errorf("check %q: unknown frequency %q (want monthly, quarterly, or yearly)", c.Name, c.Frequency)
		}
		if c.Lookback <= 0 {
			switch c.Frequency {
			case "monthly":
				c.Lookback = 12
			case "quarterly":
				c.Lookback = 4
			case "yearly":
				c.Lookback = 3
			}
		}
		if c.ExpectedMonth < 0 || c.ExpectedMonth > 12 {
			return fmt.Errorf("check %q: expected_month must be 1-12", c.Name)
		}
	}
	return nil
}
