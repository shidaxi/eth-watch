package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Interval int      `yaml:"interval"`
	RPCs     []string `yaml:"rpcs"`
}

func loadConfig(path string) (*Config, error) {
	cfg := &Config{
		Interval: 12,
	}

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("reading config file: %w", err)
	}
	if err == nil {
		if err := yaml.Unmarshal(data, cfg); err != nil {
			return nil, fmt.Errorf("parsing config file: %w", err)
		}
	}

	// Environment variables override config file values
	if v := os.Getenv("INTERVAL"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil {
			return nil, fmt.Errorf("invalid INTERVAL env var: %w", err)
		}
		cfg.Interval = n
	}
	if v := os.Getenv("RPCS"); v != "" {
		var rpcs []string
		for _, r := range strings.Split(v, ",") {
			r = strings.TrimSpace(r)
			if r != "" {
				rpcs = append(rpcs, r)
			}
		}
		cfg.RPCs = rpcs
	}

	if cfg.Interval <= 0 {
		cfg.Interval = 12
	}
	return cfg, nil
}

func (c *Config) IntervalDuration() time.Duration {
	return time.Duration(c.Interval) * time.Second
}
