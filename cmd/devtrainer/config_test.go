package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/kevnnard/devtrainer/internal/llm/pi"
)

func TestDefaultConfigPath(t *testing.T) {
	if got := defaultConfigPath("/xdg", "/home/u"); got != "/xdg/devtrainer/config.json" {
		t.Errorf("got %q", got)
	}
	if got := defaultConfigPath("", "/home/u"); got != "/home/u/.config/devtrainer/config.json" {
		t.Errorf("got %q", got)
	}
}

func TestLoadConfigMissingFileGivesDefaults(t *testing.T) {
	c, err := loadConfig(filepath.Join(t.TempDir(), "none.json"))
	if err != nil {
		t.Fatal(err)
	}
	if c.Pi != "pi" || c.Models["explain"] == "" || c.Models["draft"] == "" || c.TimeoutSeconds != 90 {
		t.Errorf("defaults = %+v", c)
	}
}

func TestLoadConfigOverridesDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	data := `{"pi":"/opt/pi","models":{"draft":"x/big"},"timeout_seconds":30}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := loadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if c.Pi != "/opt/pi" || c.Models["draft"] != "x/big" || c.Models["explain"] != defaultConfig().Models["explain"] || c.TimeoutSeconds != 30 {
		t.Errorf("config = %+v", c)
	}
	m, ok := c.model("draft").(pi.Model)
	if !ok || m.Command != "/opt/pi" || m.Model != "x/big" || m.Timeout != 30*time.Second {
		t.Errorf("model = %+v", m)
	}
}

func TestLoadConfigRejectsBadJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	os.WriteFile(path, []byte("{"), 0o644)
	if _, err := loadConfig(path); err == nil {
		t.Error("want an error")
	}
}
