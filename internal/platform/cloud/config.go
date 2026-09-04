package cloud

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/GrayCodeAI/graycode-cli/internal/auth"
	"github.com/GrayCodeAI/graycode-cli/internal/storage"
)

const (
	tokenService = "graycode-cloud"
	tokenAccount = "device-token"
)

type DeviceConfig struct {
	Endpoint  string `json:"endpoint"`
	DeviceID  string `json:"device_id"`
	ProjectID string `json:"project_id"`
}

func configPath() string { return filepath.Join(storage.ConfigDir(), "cloud.json") }

func LoadDeviceConfig() (DeviceConfig, error) {
	var cfg DeviceConfig
	b, err := os.ReadFile(configPath()) // #nosec G304 -- fixed Graycode config path
	if err != nil {
		return cfg, err
	}
	return cfg, json.Unmarshal(b, &cfg)
}

func SaveDeviceConfig(cfg DeviceConfig, token string) error {
	if err := auth.NewSecureStorage(tokenService).Set(tokenAccount, token); err != nil {
		return err
	}
	b, err := json.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(configPath()), 0o700); err != nil {
		return err
	}
	return os.WriteFile(configPath(), b, 0o600)
}

func LoadClient() (*Client, DeviceConfig, error) {
	cfg, err := LoadDeviceConfig()
	if err != nil {
		return nil, cfg, err
	}
	token, err := auth.NewSecureStorage(tokenService).Get(tokenAccount)
	if err != nil {
		return nil, cfg, err
	}
	return New(Config{Endpoint: cfg.Endpoint, DeviceToken: token}), cfg, nil
}
