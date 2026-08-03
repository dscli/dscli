package mcphub

import (
	"fmt"

	"github.com/dscli/dscli/internal/config"
)

// ServerConfig defines an MCP server connection.
type ServerConfig struct {
	Name    string   // logical server name; set from map key if empty
	Type    string   // "local" (stdio) or "cloud" (SSE); default "local"
	Command string   // executable (stdio) or URL (http/https)
	Args    []string // command-line args (stdio) or query key=value pairs (SSE)
	Enabled bool
}

// IsSSE reports whether this server uses SSE transport.
// Transport type is determined by the Type field:
//   - "cloud" → SSE transport
//   - otherwise → stdio transport (subprocess)
func (s ServerConfig) IsSSE() bool {
	return s.Type == "cloud"
}

// loadServerConfigs loads MCP server configurations from the
// "mcp-servers" key in the native config data.
//
// Multiple configs can share the same logical Name as long as they have
// different Type values (e.g., a local + cloud pair). The user config
// must use unique sub-keys for each variant.
//
// Config format (in config.dscli):
//
//	mcp-servers {
//	  server-id {
//	    name = my-server
//	    type = local
//	    command = my-server
//	    args = [serve]
//	    enabled = true
//	  }
//	}
//
// No built-in servers are hardcoded anymore: every MCP server, including
// LightPanda, is configured explicitly by the user.
func loadServerConfigs() ([]ServerConfig, error) {
	// Use a composite key "name:type" to allow multiple variants per logical name.
	configMap := make(map[string]ServerConfig)

	// Load user-defined servers from native config data.
	serversVal := config.GetValue("mcp-servers")
	if serversVal == nil {
		return nil, nil
	}

	// If the value is a string (old format "mcp-servers = filename.yaml"),
	// ignore it - there are no built-in servers to fall back to.
	if _, ok := serversVal.(string); ok {
		return nil, nil
	}

	serversMap, ok := serversVal.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("mcp-servers config must be a map block, got %T", serversVal)
	}

	for entryKey, entryVal := range serversMap {
		entry, ok := entryVal.(map[string]any)
		if !ok {
			continue
		}
		cfg := serverConfigFromMap(entry)
		if cfg.Name == "" {
			cfg.Name = entryKey
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

// serverConfigFromMap converts a parsed config map entry to ServerConfig.
func serverConfigFromMap(m map[string]any) ServerConfig {
	cfg := ServerConfig{}
	if v, ok := m["name"]; ok {
		cfg.Name = fmt.Sprint(v)
	}
	if v, ok := m["type"]; ok {
		cfg.Type = fmt.Sprint(v)
	}
	if v, ok := m["command"]; ok {
		cfg.Command = fmt.Sprint(v)
	}
	if v, ok := m["args"]; ok {
		if arr, ok := v.([]any); ok {
			cfg.Args = make([]string, len(arr))
			for i, item := range arr {
				cfg.Args[i] = fmt.Sprint(item)
			}
		}
	}
	if v, ok := m["enabled"]; ok {
		if b, ok := v.(bool); ok {
			cfg.Enabled = b
		}
	}
	return cfg
}
