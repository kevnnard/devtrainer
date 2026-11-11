package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/kevnnard/devtrainer/internal/llm/pi"
	"github.com/kevnnard/devtrainer/internal/tutor"
)

// config is the optional config.json. Anything missing takes its default.
//
//	{"pi":"pi","models":{"explain":"...","draft":"..."},"timeout_seconds":90}
type config struct {
	Pi             string            `json:"pi,omitempty"`    // the pi binary; "pi" from PATH by default
	Models         map[string]string `json:"models"`          // capability -> provider/model
	TimeoutSeconds int               `json:"timeout_seconds"` // per AI answer
}

// defaultConfig uses the strong model english-teacher uses for its
// quality-sensitive jobs: explanations must be right, and drafts must
// follow the schema.
func defaultConfig() config {
	return config{
		Pi: "pi",
		Models: map[string]string{
			"explain": "claude-bridge/claude-sonnet-5-5",
			"draft":   "claude-bridge/claude-sonnet-5-5",
		},
		TimeoutSeconds: int(pi.DefaultTimeout / time.Second),
	}
}

// defaultConfigPath follows the XDG spec: $XDG_CONFIG_HOME, else ~/.config.
func defaultConfigPath(xdgConfig, home string) string {
	if xdgConfig == "" {
		xdgConfig = filepath.Join(home, ".config")
	}
	return filepath.Join(xdgConfig, "devtrainer", "config.json")
}

// loadConfig reads path over the defaults; a missing file means defaults.
func loadConfig(path string) (config, error) {
	c := defaultConfig()
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return c, err
	}
	var file config
	if err := json.Unmarshal(data, &file); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if file.Pi != "" {
		c.Pi = file.Pi
	}
	for k, v := range file.Models {
		c.Models[k] = v
	}
	if file.TimeoutSeconds > 0 {
		c.TimeoutSeconds = file.TimeoutSeconds
	}
	return c, nil
}

// model is the pi model of one capability.
func (c config) model(capability string) tutor.Model {
	return pi.Model{Command: c.Pi, Model: c.Models[capability], Timeout: time.Duration(c.TimeoutSeconds) * time.Second}
}

// modelFor builds the model of a capability; tests replace it.
var modelFor = func(c config, capability string) tutor.Model { return c.model(capability) }

// newTutor wires one model per capability.
func newTutor(c config) *tutor.Tutor {
	return tutor.New(tutor.Models{Explain: modelFor(c, "explain"), Draft: modelFor(c, "draft")})
}
