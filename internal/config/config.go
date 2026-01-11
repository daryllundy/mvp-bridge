package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"mvpbridge/internal/detect"
)

const (
	ConfigDir  = ".mvpbridge"
	ConfigFile = "config.yaml"
)

type Config struct {
	Version   int    `yaml:"version"`
	Framework string `yaml:"framework"`
	Target    string `yaml:"target"`

	// Detected values (populated by inspect)
	Detected struct {
		PackageManager string `yaml:"package_manager,omitempty"`
		BuildCommand   string `yaml:"build_command,omitempty"`
		OutputDir      string `yaml:"output_dir,omitempty"`
		NodeVersion    string `yaml:"node_version,omitempty"`
		OutputType     string `yaml:"output_type,omitempty"`
	} `yaml:"detected,omitempty"`

	// Deployment settings
	Deploy struct {
		AppName string `yaml:"app_name,omitempty"`
		Region  string `yaml:"region,omitempty"`
	} `yaml:"deploy,omitempty"`
}

// Load reads config from .mvpbridge/config.yaml
func Load(root string) (*Config, error) {
	path := filepath.Join(root, ConfigDir, ConfigFile)

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("config not found - run 'mvpbridge init' first")
		}
		return nil, err
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return &cfg, nil
}

// Save writes config to .mvpbridge/config.yaml
func (c *Config) Save(root string) error {
	dir := filepath.Join(root, ConfigDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}

	path := filepath.Join(dir, ConfigFile)
	return os.WriteFile(path, data, 0600)
}

// NewFromDetection creates a config from detection results
func NewFromDetection(d *detect.Detection, target string) *Config {
	cfg := &Config{
		Version:   1,
		Framework: string(d.Framework),
		Target:    target,
	}

	cfg.Detected.PackageManager = string(d.PackageManager)
	cfg.Detected.BuildCommand = d.BuildCommand
	cfg.Detected.OutputDir = d.OutputDir
	cfg.Detected.NodeVersion = d.NodeVersion
	cfg.Detected.OutputType = string(d.OutputType)

	return cfg
}

// Validate checks if config has required fields
func (c *Config) Validate() error {
	if c.Version != 1 {
		return fmt.Errorf("unsupported config version: %d", c.Version)
	}

	if c.Framework == "" {
		return fmt.Errorf("framework not set")
	}

	validFrameworks := map[string]bool{"vite": true, "nextjs": true}
	if !validFrameworks[c.Framework] {
		return fmt.Errorf("unsupported framework: %s", c.Framework)
	}

	if c.Target != "" {
		validTargets := map[string]bool{"do": true, "aws": true}
		if !validTargets[c.Target] {
			return fmt.Errorf("unsupported target: %s", c.Target)
		}
	}

	return nil
}

// IsStatic returns true if the project outputs static files
func (c *Config) IsStatic() bool {
	return c.Detected.OutputType == "static"
}

// GetFramework returns the framework as a detect.Framework type
func (c *Config) GetFramework() detect.Framework {
	switch c.Framework {
	case "vite":
		return detect.Vite
	case "nextjs":
		return detect.NextJS
	default:
		return detect.Unknown
	}
}
