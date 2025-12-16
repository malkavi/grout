package utils

import (
	"encoding/json"
	"fmt"
	"grout/romm"
	"os"
	"time"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

type Config struct {
	Hosts             []romm.Host                 `json:"hosts,omitempty"`
	DirectoryMappings map[string]DirectoryMapping `json:"directory_mappings,omitempty"`
	ShowGameDetails   bool                        `json:"show_game_details"`
	AutoSyncSaves     bool                        `json:"auto_sync_saves"`
	DownloadArt       bool                        `json:"download_art,omitempty"`
	UnzipDownloads    bool                        `json:"unzip_downloads,omitempty"`
	ApiTimeout        time.Duration               `json:"api_timeout"`
	DownloadTimeout   time.Duration               `json:"download_timeout"`
	LogLevel          string                      `json:"log_level,omitempty"`
}

type DirectoryMapping struct {
	RomMSlug      string `json:"slug"`
	RelativePath  string `json:"relative_path"`
	SaveDirectory string `json:"save_directory,omitempty"`
}

func (c Config) ToLoggable() any {
	safeHosts := make([]map[string]any, len(c.Hosts))
	for i, host := range c.Hosts {
		safeHosts[i] = host.ToLoggable()
	}

	return map[string]any{
		"hosts":              safeHosts,
		"directory_mappings": c.DirectoryMappings,
		"api_timeout":        c.ApiTimeout,
		"download_timeout":   c.DownloadTimeout,
		"unzip_downloads":    c.UnzipDownloads,
		"download_art":       c.DownloadArt,
		"show_game_details":  c.ShowGameDetails,
		"log_level":          c.LogLevel,
	}
}

func LoadConfig() (*Config, error) {
	data, err := os.ReadFile("config.json")
	if err != nil {
		return nil, fmt.Errorf("reading config.json: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parsing config.json: %w", err)
	}

	if config.ApiTimeout == 0 {
		config.ApiTimeout = 30 * time.Minute
	}

	if config.DownloadTimeout == 0 {
		config.DownloadTimeout = 60 * time.Minute
	}

	return &config, nil
}

func SaveConfig(config *Config) error {
	if config.LogLevel == "" {
		config.LogLevel = "ERROR"
	}

	gaba.SetRawLogLevel(config.LogLevel)

	pretty, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		gaba.GetLogger().Error("Failed to marshal config to JSON", "error", err)
		return err
	}

	if err := os.WriteFile("config.json", pretty, 0644); err != nil {
		gaba.GetLogger().Error("Failed to write config file", "error", err)
		return err
	}

	return nil
}
