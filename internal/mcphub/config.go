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
	Name    string   `yaml:"name"`   // logical server name; set from map key if empty
	Type    string   `yaml:"type"`   // "local" (stdio) or "cloud" (SSE); default "local"
	Command string   `yaml:"command"` // executable (stdio) or URL (http/https)
	Args    []string `yaml:"args"`    // command-line args (stdio) or query key=value pairs (SSE)
	Enabled bool     `yaml:"enabled"`
}

// IsSSE reports whether this server uses SSE transport.
// Transport type is determined by the Type field:
//   - "cloud" → SSE transport
//   - otherwise → stdio transport (subprocess)
func (s ServerConfig) IsSSE() bool {
	return s.Type == "cloud"
}

// serversFile is the parsed structure of the mcp-servers YAML file.
type serversFile struct {
	Servers map[string]ServerConfig `yaml:"servers"`
}

// builtinServers are hardcoded MCP server configurations.
// These are always available and can be overridden by user config.
var builtinServers = []ServerConfig{
	{
		Name:    "lightpanda",
		Type:    "local",
		Command: "lightpanda",
		Args:    []string{"mcp"},
		Enabled: true,
	},
}

// loadServerConfigs loads MCP server configurations.
// It starts with built-in servers, then overlays user-defined servers
// from the YAML file specified by the "mcp-servers" config key.
//
// Multiple configs can share the same logical Name as long as they have
// different Type values (e.g., lightpanda local + cloud). The user YAML
// must use unique map keys for each variant.
//
// The YAML file is resolved relative to the config directory (~/.dscli/).
func loadServerConfigs() ([]ServerConfig, error) {
	// Use a composite key "name:type" to allow multiple variants per logical name.
	configMap := make(map[string]ServerConfig)
	for _, s := range builtinServers {
		key := s.Name + ":" + s.Type
		configMap[key] = s
	}

	// Load user-defined servers from YAML.
	filename := config.Get("mcp-servers", "")
	if filename == "" {
		// No user config — return built-in servers only.
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

	for key, cfg := range sf.Servers {
		if cfg.Name == "" {
			cfg.Name = key
		}
		if cfg.Type == "" {
			cfg.Type = "local"
		}
		configKey := cfg.Name + ":" + cfg.Type
		configMap[configKey] = cfg
	}

	result := make([]ServerConfig, 0, len(configMap))
	for _, s := range configMap {
		result = append(result, s)
	}
	return result, nil
}
