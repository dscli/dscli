package mcphub

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/dscli/dscli/internal/config"
	"github.com/goccy/go-yaml"
)

// ServerConfig defines an MCP server connection.
type ServerConfig struct {
	Name    string   `yaml:"name"` // server identifier
	Command string   `yaml:"command"`
	Args    []string `yaml:"args"`
	Enabled bool     `yaml:"enabled"`
}

// serversFile is the parsed structure of the mcp-servers YAML file.
type serversFile struct {
	Servers []ServerConfig `yaml:"servers"`
}

// builtinServers are hardcoded MCP server configurations.
// These are always available and can be overridden by user config.
var builtinServers = []ServerConfig{
	{
		Name:    "lightpanda",
		Command: "lightpanda",
		Args:    []string{"mcp"},
		Enabled: true,
	},
}

// loadServerConfigs loads MCP server configurations.
// It starts with built-in servers, then overlays user-defined servers
// from the YAML file specified by the "mcp-servers" config key.
//
// The YAML file is resolved relative to the config directory (~/.dscli/).
func loadServerConfigs() ([]ServerConfig, error) {
	// Start with built-in servers.
	servers := make(map[string]ServerConfig)
	for _, s := range builtinServers {
		servers[s.Name] = s
	}

	// Load user-defined servers from YAML.
	filename := config.Get("mcp-servers", "")
	if filename == "" {
		return builtinServers, nil
	}

	// Resolve relative to config directory.
	if !filepath.IsAbs(filename) {
		filename = filepath.Join(config.ConfigDir, filename)
	}

	data, err := os.ReadFile(filename)
	if err != nil {
		if os.IsNotExist(err) {
			return builtinServers, nil
		}
		return nil, fmt.Errorf("reading mcp-servers config: %w", err)
	}

	var sf serversFile
	if err := yaml.Unmarshal(data, &sf); err != nil {
		return nil, fmt.Errorf("parsing mcp-servers config: %w", err)
	}

	for _, cfg := range sf.Servers {
		if cfg.Name == "" {
			continue
		}
		servers[cfg.Name] = cfg
	}

	result := make([]ServerConfig, 0, len(servers))
	for _, s := range servers {
		result = append(result, s)
	}
	return result, nil
}
