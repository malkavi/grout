package utils

import (
	"encoding/json"
	"fmt"
	"grout/romm"
	"os"
	"time"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
	"github.com/BrandonKowalski/gabagool/v2/pkg/gabagool/i18n"
)

type Config struct {
	Hosts                  []romm.Host                 `json:"hosts,omitempty"`
	DirectoryMappings      map[string]DirectoryMapping `json:"directory_mappings,omitempty"`
	ShowGameDetails        bool                        `json:"show_game_details"`
	AutoSyncSaves          bool                        `json:"auto_sync_saves"`
	DownloadArt            bool                        `json:"download_art,omitempty"`
	UnzipDownloads         bool                        `json:"unzip_downloads,omitempty"`
	ShowCollections        bool                        `json:"show_collections"`
	ShowVirtualCollections bool                        `json:"show_virtual_collections"`
	ApiTimeout             time.Duration               `json:"api_timeout"`
	DownloadTimeout        time.Duration               `json:"download_timeout"`
	LogLevel               string                      `json:"log_level,omitempty"`
	Language               string                      `json:"language,omitempty"`
}

type DirectoryMapping struct {
	RomMSlug     string `json:"slug"`
	RelativePath string `json:"relative_path"`
}

func (c Config) ToLoggable() any {
	safeHosts := make([]map[string]any, len(c.Hosts))
	for i, host := range c.Hosts {
		safeHosts[i] = host.ToLoggable()
	}

	return map[string]any{
		"hosts":                    safeHosts,
		"directory_mappings":       c.DirectoryMappings,
		"api_timeout":              c.ApiTimeout,
		"download_timeout":         c.DownloadTimeout,
		"unzip_downloads":          c.UnzipDownloads,
		"download_art":             c.DownloadArt,
		"show_game_details":        c.ShowGameDetails,
		"show_collections":         c.ShowCollections,
		"show_virtual_collections": c.ShowVirtualCollections,
		"log_level":                c.LogLevel,
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

	if config.Language == "" {
		config.Language = "en"
	}

	return &config, nil
}

func SaveConfig(config *Config) error {
	if config.LogLevel == "" {
		config.LogLevel = "ERROR"
	}

	if config.Language == "" {
		config.Language = "en"
	}

	gaba.SetRawLogLevel(config.LogLevel)

	if err := i18n.SetWithCode(config.Language); err != nil {
		gaba.GetLogger().Error("Failed to set language", "error", err, "language", config.Language)
	}

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
