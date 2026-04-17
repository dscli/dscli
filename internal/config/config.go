package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var (
	ConfigDir = getConfigDir()
	_config   = loadConfig()
)

func Get(name string, deval string) (value string) {
	value = _config[name]
	if value != "" {
		return
	}
	value = deval
	return
}

func getConfigDir() (configDir string) {
	configDir = filepath.Join(os.Getenv("HOME"), ".dscli")
	err := os.MkdirAll(configDir, 0o755)
	if err != nil {
		panic(err)
	}
	return
}

func loadConfig() (config map[string]string) {
	configFile := filepath.Join(ConfigDir, "config.dscli")
	config = loadConfigFromFile(configFile)
	if len(config) > 0 {
		return
	}
	configFile = filepath.Join(ConfigDir, "dscli.env")
	config = loadConfigFromFile(configFile)
	if len(config) > 0 {
		err := saveConfigToFile(config)
		if err != nil {
			panic(err)
		}
		return
	}

	config = loadConfigFromEnv()
	if len(config) != 0 {
		err := saveConfigToFile(config)
		if err != nil {
			panic(err)
		}
		return
	}
	return
}

func saveConfigToFile(config map[string]string) (err error) {
	lines := []string{}

	for k, v := range config {
		line := fmt.Sprintf("%s = %s", k, v)
		lines = append(lines, line)
	}
	data := strings.Join(lines, "\n\n")
	configFile := filepath.Join(ConfigDir, "config.dscli")
	err = os.WriteFile(configFile, []byte(data), 0o600)
	if err != nil {
		return
	}
	return
}

func loadConfigFromFile(filename string) (config map[string]string) {
	b, err := os.ReadFile(filename)
	if err != nil {
		return
	}
	return parseConfig(string(b))
}

func loadConfigFromEnv() (config map[string]string) {
	const (
		BaseURL = "DEEPSEEK_BASE_URL"
		APIKey  = "DEEPSEEK_API_KEY"
	)
	config = map[string]string{}
	baseURL := os.Getenv(BaseURL)
	if baseURL != "" {
		config[configName(BaseURL)] = baseURL
	}
	apiKey := os.Getenv(APIKey)
	if apiKey != "" {
		config[configName(APIKey)] = apiKey
	}
	return
}

func configName(en string) (cn string) {
	if en == "" {
		return
	}
	en = strings.ReplaceAll(en, "_", "-")
	cn = strings.ToLower(en)
	return
}

func parseConfig(data string) (config map[string]string) {
	config = map[string]string{}
	for line := range strings.Lines(data) {
		line = strings.TrimSpace(line)
		idx := strings.Index(line, "#")
		if idx != -1 {
			line = strings.TrimSpace(line[0:idx])
		}
		idx = strings.Index(line, "=")
		if idx == -1 {
			continue
		}
		name := strings.TrimSpace(line[0:idx])
		if name == "" {
			continue
		}
		value := strings.TrimSpace(line[idx+1:])
		idx = strings.Index(name, " ")
		if idx != -1 {
			if strings.TrimSpace(name[0:idx]) != "export" {
				continue
			}
			name = name[idx+1:]
		}
		config[configName(name)] = value
	}
	return
}
