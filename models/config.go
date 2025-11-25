package models

import "time"

type Config struct {
	Hosts             Hosts              `json:"hosts,omitempty"`
	DirectoryMappings []DirectoryMapping `json:"directory_mappings,omitempty"`
	ApiTimeout        time.Duration      `json:"api_timeout"`
	DownloadTimeout   time.Duration      `json:"download_timeout"`
	UnzipDownloads    bool               `json:"unzip_downloads,omitempty"`
	DownloadArt       bool               `json:"download_art,omitempty"`
	GroupBinCue       bool               `json:"group_bin_cue,omitempty"`
	GroupMultiDisc    bool               `json:"group_multi_disc,omitempty"`
	LogLevel          string             `json:"log_level,omitempty"`
}
