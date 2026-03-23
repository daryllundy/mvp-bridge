// Package config provides configuration file management for MVPBridge.
// It handles loading and saving project configuration in .mvpbridge/config.yaml.
package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"

	"github.com/daryllundy/mvp-bridge/internal/detect"
	"github.com/daryllundy/mvp-bridge/internal/projectfs"
)

const (
	// ConfigDir is the directory name for MVPBridge configuration
	ConfigDir = ".mvpbridge"
	// ConfigFile is the name of the configuration file
	ConfigFile = "config.yaml"
)

type frameworkName string

const (
	frameworkVite   frameworkName = "vite"
	frameworkNextJS frameworkName = "nextjs"
)

type targetName string

const (
	targetDO    targetName = "do"
	targetAWS   targetName = "aws"
	targetGCP   targetName = "gcp"
	targetAzure targetName = "azure"
	targetLocal targetName = "local"
)

type outputTypeName string

const (
	outputTypeStatic outputTypeName = "static"
)

// Config represents the MVPBridge project configuration
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
		AppName        string `yaml:"app_name,omitempty"`
		Region         string `yaml:"region,omitempty"`
		ProjectID      string `yaml:"project_id,omitempty"`
		SubscriptionID string `yaml:"subscription_id,omitempty"`
		ResourceGroup  string `yaml:"resource_group,omitempty"`
		Environment    string `yaml:"environment,omitempty"`
	} `yaml:"deploy,omitempty"`
}

// Load reads config from .mvpbridge/config.yaml
func Load(root string) (*Config, error) {
	data, err := readFileInRoot(root, filepath.Join(ConfigDir, ConfigFile))
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
	if err := os.MkdirAll(dir, 0750); err != nil {
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
	framework := frameworkFromDetect(d.Framework)

	cfg := &Config{
		Version:   1,
		Framework: string(framework),
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

	if _, ok := parseFrameworkName(c.Framework); !ok {
		return fmt.Errorf("unsupported framework: %s", c.Framework)
	}

	if c.Target != "" {
		if _, ok := parseTargetName(c.Target); !ok {
			return fmt.Errorf("unsupported target: %s", c.Target)
		}
	}

	return nil
}

// IsStatic returns true if the project outputs static files
func (c *Config) IsStatic() bool {
	return outputTypeName(c.Detected.OutputType) == outputTypeStatic
}

// GetFramework returns the framework as a detect.Framework type
func (c *Config) GetFramework() detect.Framework {
	switch frameworkName(c.Framework) {
	case frameworkVite:
		return detect.Vite
	case frameworkNextJS:
		return detect.NextJS
	default:
		return detect.Unknown
	}
}

func parseFrameworkName(value string) (frameworkName, bool) {
	switch frameworkName(value) {
	case frameworkVite, frameworkNextJS:
		return frameworkName(value), true
	default:
		return "", false
	}
}

func parseTargetName(value string) (targetName, bool) {
	switch targetName(value) {
	case targetDO, targetAWS, targetGCP, targetAzure, targetLocal:
		return targetName(value), true
	default:
		return "", false
	}
}

func frameworkFromDetect(fw detect.Framework) frameworkName {
	switch fw {
	case detect.Vite:
		return frameworkVite
	case detect.NextJS:
		return frameworkNextJS
	default:
		return frameworkName(fw)
	}
}

func readFileInRoot(root, rel string) ([]byte, error) {
	return projectfs.ReadFileInRoot(root, rel)
}
