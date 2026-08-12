package config

import (
	"encoding/json"
	"os"

	"github.com/donaina/driftwood/pkg/types"
)

// LoadConfigFile loads proxy configuration from a JSON file
func LoadConfigFile(path string) (*types.ProxyConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg types.ProxyConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
