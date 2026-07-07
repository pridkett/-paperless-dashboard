package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.toml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

const baseConfig = `
[paperless]
url = "https://file.example.com"
token = "file-token"
`

func TestPrecedenceFlagBeatsEnvAndFile(t *testing.T) {
	t.Setenv(EnvURL, "https://env.example.com")
	path := writeConfig(t, baseConfig)

	cfg, err := Load(Flags{ConfigPath: path, URL: "https://flag.example.com"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.URL.Value != "https://flag.example.com" || cfg.URL.Source != SourceFlag {
		t.Errorf("URL = %q from %s, want flag value", cfg.URL.Value, cfg.URL.Source)
	}

	found := false
	for _, w := range cfg.Warnings {
		if strings.Contains(w, "paperless URL") && strings.Contains(w, string(SourceFlag)) {
			found = true
		}
	}
	if !found {
		t.Errorf("expected an override warning for the URL, got %v", cfg.Warnings)
	}
}

func TestPrecedenceEnvBeatsFile(t *testing.T) {
	t.Setenv(EnvToken, "env-token")
	path := writeConfig(t, baseConfig)

	cfg, err := Load(Flags{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Token.Value != "env-token" || cfg.Token.Source != SourceEnv {
		t.Errorf("token = %q from %s, want env value", cfg.Token.Value, cfg.Token.Source)
	}
	if len(cfg.Warnings) == 0 {
		t.Error("expected an override warning when env shadows file")
	}
}

func TestDefaultsProduceNoWarnings(t *testing.T) {
	path := writeConfig(t, baseConfig)

	cfg, err := Load(Flags{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Listen.Value != DefaultListen || cfg.Listen.Source != SourceDefault {
		t.Errorf("listen = %q from %s, want default", cfg.Listen.Value, cfg.Listen.Source)
	}
	if len(cfg.Warnings) != 0 {
		t.Errorf("unexpected warnings: %v", cfg.Warnings)
	}
}

func TestCheckValidation(t *testing.T) {
	path := writeConfig(t, baseConfig+`
[[check]]
name = "Bad Frequency"
frequency = "fortnightly"
`)
	if _, err := Load(Flags{ConfigPath: path}); err == nil {
		t.Error("expected an error for unknown frequency")
	}
}

func TestCheckDefaults(t *testing.T) {
	path := writeConfig(t, baseConfig+`
[[check]]
name = "Simple"
`)
	cfg, err := Load(Flags{ConfigPath: path})
	if err != nil {
		t.Fatal(err)
	}
	ck := cfg.Checks[0]
	if ck.Frequency != "monthly" || ck.Lookback != 12 {
		t.Errorf("defaults = %s/%d, want monthly/12", ck.Frequency, ck.Lookback)
	}
}

func TestMissingURLFails(t *testing.T) {
	path := writeConfig(t, `[paperless]
token = "t"`)
	if _, err := Load(Flags{ConfigPath: path}); err == nil {
		t.Error("expected an error when no URL is configured")
	}
}
