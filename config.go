package main

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Interval int      `yaml:"interval"`
	RPCs     []string `yaml:"rpcs"`
}

func loadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	cfg := &Config{
		Interval: 12,
	}
	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	if cfg.Interval <= 0 {
		cfg.Interval = 12
	}
	return cfg, nil
}

func (c *Config) IntervalDuration() time.Duration {
	return time.Duration(c.Interval) * time.Second
}
