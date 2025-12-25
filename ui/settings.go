package ui

import (
	"errors"
	"grout/constants"
	"grout/romm"
	"grout/utils"
	"time"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
	"github.com/BrandonKowalski/gabagool/v2/pkg/gabagool/i18n"
)

type SettingsInput struct {
	Config            *utils.Config
	CFW               constants.CFW
	Host              romm.Host
	LastSelectedIndex int
}

type SettingsOutput struct {
	Config              *utils.Config
	EditMappingsClicked bool
	InfoClicked         bool
	LastSelectedIndex   int
}

type SettingsScreen struct{}

func NewSettingsScreen() *SettingsScreen {
	return &SettingsScreen{}
}

var (
	apiTimeoutOptions = []struct {
		I18nKey string
		Value   time.Duration
	}{
		{"time_15_seconds", 15 * time.Second},
		{"time_30_seconds", 30 * time.Second},
		{"time_45_seconds", 45 * time.Second},
		{"time_60_seconds", 60 * time.Second},
		{"time_75_seconds", 75 * time.Second},
		{"time_90_seconds", 90 * time.Second},
		{"time_120_seconds", 120 * time.Second},
		{"time_180_seconds", 180 * time.Second},
		{"time_240_seconds", 240 * time.Second},
		{"time_300_seconds", 300 * time.Second},
	}

	downloadTimeoutOptions = []struct {
		I18nKey string
		Value   time.Duration
	}{
		{"time_15_minutes", 15 * time.Minute},
		{"time_30_minutes", 30 * time.Minute},
		{"time_45_minutes", 45 * time.Minute},
		{"time_60_minutes", 60 * time.Minute},
		{"time_75_minutes", 75 * time.Minute},
		{"time_90_minutes", 90 * time.Minute},
		{"time_105_minutes", 105 * time.Minute},
		{"time_120_minutes", 120 * time.Minute},
	}
)

func (s *SettingsScreen) Draw(input SettingsInput) (ScreenResult[SettingsOutput], error) {
	config := input.Config
	output := SettingsOutput{Config: config}

	items := s.buildMenuItems(config)

	result, err := gaba.OptionsList(
		i18n.GetString("settings_title"),
		gaba.OptionListSettings{
			FooterHelpItems: []gaba.FooterHelpItem{
				{ButtonName: "B", HelpText: i18n.GetString("button_cancel")},
				{ButtonName: "←→", HelpText: i18n.GetString("button_cycle")},
				{ButtonName: "Start", HelpText: i18n.GetString("button_save")},
			},
			InitialSelectedIndex: input.LastSelectedIndex,
		},
		items,
	)

	if err != nil {
		if errors.Is(err, gaba.ErrCancelled) {
			return back(SettingsOutput{}), nil
		}
		return withCode(SettingsOutput{}, gaba.ExitCodeError), err
	}

	output.LastSelectedIndex = result.Selected

	if result.Action == gaba.ListActionSelected && result.Selected == 0 {
		output.EditMappingsClicked = true
		return withCode(output, constants.ExitCodeEditMappings), nil
	}

	if result.Action == gaba.ListActionSelected && result.Selected == len(items)-1 {
		output.InfoClicked = true
		return withCode(output, constants.ExitCodeInfo), nil
	}

	s.applySettings(config, result.Items)

	output.Config = config
	return success(output), nil
}

func (s *SettingsScreen) buildMenuItems(config *utils.Config) []gaba.ItemWithOptions {
	return []gaba.ItemWithOptions{
		{
			Item:    gaba.MenuItem{Text: i18n.GetString("settings_edit_mappings")},
			Options: []gaba.Option{{Type: gaba.OptionTypeClickable}},
		},
		{
			Item: gaba.MenuItem{Text: i18n.GetString("settings_show_game_details")},
			Options: []gaba.Option{
				{DisplayName: i18n.GetString("common_true"), Value: true},
				{DisplayName: i18n.GetString("common_false"), Value: false},
			},
			SelectedOption: boolToIndex(!config.ShowGameDetails),
		},

		// Collection Settings
		{
			Item: gaba.MenuItem{Text: i18n.GetString("settings_show_collections")},
			Options: []gaba.Option{
				{DisplayName: i18n.GetString("common_true"), Value: true},
				{DisplayName: i18n.GetString("common_false"), Value: false},
			},
			SelectedOption: boolToIndex(!config.ShowCollections),
		},
		{
			Item: gaba.MenuItem{Text: i18n.GetString("settings_show_smart_collections")},
			Options: []gaba.Option{
				{DisplayName: i18n.GetString("common_true"), Value: true},
				{DisplayName: i18n.GetString("common_false"), Value: false},
			},
			SelectedOption: boolToIndex(!config.ShowSmartCollections),
		},
		{
			Item: gaba.MenuItem{Text: i18n.GetString("settings_show_virtual_collections")},
			Options: []gaba.Option{
				{DisplayName: i18n.GetString("common_true"), Value: true},
				{DisplayName: i18n.GetString("common_false"), Value: false},
			},
			SelectedOption: boolToIndex(!config.ShowVirtualCollections),
		},

		{
			Item: gaba.MenuItem{Text: i18n.GetString("settings_downloaded_games")},
			Options: []gaba.Option{
				{DisplayName: i18n.GetString("downloaded_games_do_nothing"), Value: "do_nothing"},
				{DisplayName: i18n.GetString("downloaded_games_mark"), Value: "mark"},
				{DisplayName: i18n.GetString("downloaded_games_filter"), Value: "filter"},
			},
			SelectedOption: s.downloadedGamesActionToIndex(config.DownloadedGamesDisplayOption),
		},

		// TODO Enable Later
		//{
		//	Item: gaba.MenuItem{Text: "Auto Sync Saves"},
		//	Options: []gaba.Option{
		//		{DisplayName: i18n.GetString("common_true"), Value: true},
		//		{DisplayName: i18n.GetString("common_false"), Value: false},
		//	},
		//	SelectedOption: boolToIndex(!config.AutoSyncSaves),
		//},
		{
			Item: gaba.MenuItem{Text: i18n.GetString("settings_download_art")},
			Options: []gaba.Option{
				{DisplayName: i18n.GetString("common_true"), Value: true},
				{DisplayName: i18n.GetString("common_false"), Value: false},
			},
			SelectedOption: boolToIndex(!config.DownloadArt),
		},
		{
			Item: gaba.MenuItem{Text: i18n.GetString("settings_unzip_downloads")},
			Options: []gaba.Option{
				{DisplayName: i18n.GetString("common_true"), Value: true},
				{DisplayName: i18n.GetString("common_false"), Value: false},
			},
			SelectedOption: boolToIndex(!config.UnzipDownloads),
		},
		{
			Item:           gaba.MenuItem{Text: i18n.GetString("settings_api_timeout")},
			Options:        s.buildApiTimeoutOptions(),
			SelectedOption: s.findApiTimeoutIndex(config.ApiTimeout),
		},
		{
			Item:           gaba.MenuItem{Text: i18n.GetString("settings_download_timeout")},
			Options:        s.buildDownloadTimeoutOptions(),
			SelectedOption: s.findDownloadTimeoutIndex(config.DownloadTimeout),
		},
		{
			Item: gaba.MenuItem{Text: i18n.GetString("settings_language")},
			Options: []gaba.Option{
				{DisplayName: i18n.GetString("settings_language_english"), Value: "en"},
				{DisplayName: i18n.GetString("settings_language_spanish"), Value: "es"},
				{DisplayName: i18n.GetString("settings_language_french"), Value: "fr"},
			},
			SelectedOption: languageToIndex(config.Language),
		},
		{
			Item: gaba.MenuItem{Text: i18n.GetString("settings_log_level")},
			Options: []gaba.Option{
				{DisplayName: i18n.GetString("log_level_debug"), Value: "DEBUG"},
				{DisplayName: i18n.GetString("log_level_error"), Value: "ERROR"},
			},
			SelectedOption: logLevelToIndex(config.LogLevel),
		},
		{
			Item:    gaba.MenuItem{Text: i18n.GetString("settings_info")},
			Options: []gaba.Option{{Type: gaba.OptionTypeClickable}},
		},
	}
}

func (s *SettingsScreen) buildApiTimeoutOptions() []gaba.Option {
	options := make([]gaba.Option, len(apiTimeoutOptions))
	for i, opt := range apiTimeoutOptions {
		options[i] = gaba.Option{DisplayName: i18n.GetString(opt.I18nKey), Value: opt.Value}
	}
	return options
}

func (s *SettingsScreen) buildDownloadTimeoutOptions() []gaba.Option {
	options := make([]gaba.Option, len(downloadTimeoutOptions))
	for i, opt := range downloadTimeoutOptions {
		options[i] = gaba.Option{DisplayName: i18n.GetString(opt.I18nKey), Value: opt.Value}
	}
	return options
}

func (s *SettingsScreen) findApiTimeoutIndex(timeout time.Duration) int {
	for i, opt := range apiTimeoutOptions {
		if opt.Value == timeout {
			return i
		}
	}
	return 0
}

func (s *SettingsScreen) findDownloadTimeoutIndex(timeout time.Duration) int {
	for i, opt := range downloadTimeoutOptions {
		if opt.Value == timeout {
			return i
		}
	}
	return 0
}

func (s *SettingsScreen) applySettings(config *utils.Config, items []gaba.ItemWithOptions) {
	for _, item := range items {
		text := item.Item.Text
		switch text {
		case i18n.GetString("settings_download_art"):
			config.DownloadArt = item.SelectedOption == 0
		case i18n.GetString("settings_auto_sync_saves"):
			config.AutoSyncSaves = item.SelectedOption == 0
		case i18n.GetString("settings_unzip_downloads"):
			config.UnzipDownloads = item.SelectedOption == 0
		case i18n.GetString("settings_show_game_details"):
			config.ShowGameDetails = item.SelectedOption == 0
		case i18n.GetString("settings_show_collections"):
			config.ShowCollections = item.SelectedOption == 0
		case i18n.GetString("settings_show_smart_collections"):
			config.ShowSmartCollections = item.SelectedOption == 0
		case i18n.GetString("settings_show_virtual_collections"):
			config.ShowVirtualCollections = item.SelectedOption == 0
		case i18n.GetString("settings_api_timeout"):
			idx := item.SelectedOption
			if idx < len(apiTimeoutOptions) {
				config.ApiTimeout = apiTimeoutOptions[idx].Value
			}
		case i18n.GetString("settings_download_timeout"):
			idx := item.SelectedOption
			if idx < len(downloadTimeoutOptions) {
				config.DownloadTimeout = downloadTimeoutOptions[idx].Value
			}
		case i18n.GetString("settings_log_level"):
			if val, ok := item.Options[item.SelectedOption].Value.(string); ok {
				config.LogLevel = val
			}
		case i18n.GetString("settings_language"):
			if val, ok := item.Options[item.SelectedOption].Value.(string); ok {
				config.Language = val
			}
		case i18n.GetString("settings_downloaded_games"):
			if val, ok := item.Options[item.SelectedOption].Value.(string); ok {
				config.DownloadedGamesDisplayOption = val
			}
		}
	}
}

func boolToIndex(b bool) int {
	if b {
		return 1
	}
	return 0
}

func logLevelToIndex(level string) int {
	switch level {
	case "DEBUG":
		return 0
	case "ERROR":
		return 1
	default:
		return 0
	}
}

func languageToIndex(lang string) int {
	switch lang {
	case "en":
		return 0
	case "es":
		return 1
	case "fr":
		return 2
	default:
		return 0
	}
}

func (s *SettingsScreen) downloadedGamesActionToIndex(action string) int {
	switch action {
	case "do_nothing":
		return 0
	case "mark":
		return 1
	case "filter":
		return 2
	default:
		return 0
	}
}
