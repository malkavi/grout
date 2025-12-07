package main

import (
	"grout/constants"
	"grout/models"
	"grout/ui"
	"grout/utils"
	"log/slog"
	"time"

	_ "github.com/UncleJunVIP/certifiable"
	gaba "github.com/UncleJunVIP/gabagool/v2/pkg/gabagool"
	"github.com/brandonkowalski/go-romm"
)

const (
	PlatformSelection       = "platform_selection"
	GameList                = "game_list"
	Search                  = "search"
	Settings                = "settings"
	SettingsPlatformMapping = "platform_mapping"
)

type (
	CurrentGamesList   []romm.DetailedRom
	FullGamesList      []romm.DetailedRom
	SearchFilterString string
	QuitOnBackBool     bool

	GameListPosition struct {
		Index int
		Pos   int
	}

	PlatformListPosition struct {
		Index int
		Pos   int
	}
)

var appConfig *models.Config

func init() {
	cfw := utils.GetCFW()

	gaba.Init(gaba.Options{
		WindowTitle:          "Grout",
		PrimaryThemeColorHex: 0x007C77,
		ShowBackground:       true,
		IsNextUI:             cfw == constants.NEXTUI,
		LogFilename:          "grout.log",
	})

	gaba.SetLogLevel(slog.LevelDebug)

	if !utils.IsConnectedToInternet() {
		_, _ = gaba.ConfirmationMessage("No Internet Connection!\nMake sure you are connected to Wi-Fi.", []gaba.FooterHelpItem{
			{ButtonName: "B", HelpText: "Quit"},
		}, gaba.MessageOptions{})
		defer cleanup()
		utils.LogStandardFatal("No Internet Connection", nil)
	}

	gaba.ProcessMessage("", gaba.ProcessMessageOptions{
		Image:       "resources/splash.png",
		ImageWidth:  gaba.GetWindow().GetWidth(),
		ImageHeight: gaba.GetWindow().GetHeight(),
	}, func() (interface{}, error) {
		time.Sleep(750 * time.Millisecond)
		return nil, nil
	})

	config, err := utils.LoadConfig()
	if err != nil {
		gaba.GetLogger().Debug("No RomM Host Configured")
		loginConfig, loginErr := ui.LoginFlow(models.Host{})
		if loginErr != nil {
			utils.LogStandardFatal("Login failed", loginErr)
		}
		config = loginConfig
		utils.SaveConfig(config)
	}

	appConfig = config

	if config.LogLevel != "" {
		gaba.SetRawLogLevel(config.LogLevel)
	}

	if config.DirectoryMappings == nil || len(config.DirectoryMappings) == 0 {
		screen := ui.NewPlatformMappingScreen()
		result, err := screen.Draw(ui.PlatformMappingInput{
			Host:           config.Hosts[0],
			ApiTimeout:     config.ApiTimeout,
			CFW:            cfw,
			RomDirectory:   utils.GetRomDirectory(),
			AutoSelect:     false,
			HideBackButton: true,
		})

		if err == nil && result.ExitCode == gaba.ExitCodeSuccess {
			config.DirectoryMappings = result.Value.Mappings
			utils.SaveConfig(config)
			appConfig = config
		}
	}

	gaba.GetLogger().Debug("Configuration Loaded!", "config", config.ToLoggable())
}

func cleanup() {
	gaba.Close()
}

func main() {
	defer cleanup()

	logger := gaba.GetLogger()
	logger.Debug("Starting Grout")

	config := appConfig
	cfw := utils.GetCFW()
	quitOnBack := len(config.Hosts) == 1
	platforms := utils.GetMappedPlatforms(config.Hosts[0], config.DirectoryMappings)

	fsm := buildFSM(config, cfw, platforms, quitOnBack)

	if err := fsm.Run(); err != nil {
		logger.Error("FSM error", "error", err)
	}
}

func buildFSM(config *models.Config, cfw constants.CFW, platforms []romm.Platform, quitOnBack bool) *gaba.FSM {
	fsm := gaba.NewFSM()

	gaba.Set(fsm.Context(), config)
	gaba.Set(fsm.Context(), cfw)
	gaba.Set(fsm.Context(), config.Hosts[0])
	gaba.Set(fsm.Context(), platforms)
	gaba.Set(fsm.Context(), QuitOnBackBool(quitOnBack))
	gaba.Set(fsm.Context(), SearchFilterString(""))

	gaba.AddState(fsm, PlatformSelection, func(ctx *gaba.Context) (ui.PlatformSelectionOutput, gaba.ExitCode) {
		platforms, _ := gaba.Get[[]romm.Platform](ctx)
		quitOnBack, _ := gaba.Get[QuitOnBackBool](ctx)
		platPos, _ := gaba.Get[PlatformListPosition](ctx)

		screen := ui.NewPlatformSelectionScreen()
		result, err := screen.Draw(ui.PlatformSelectionInput{
			Platforms:            platforms,
			QuitOnBack:           bool(quitOnBack),
			LastSelectedIndex:    platPos.Index,
			LastSelectedPosition: platPos.Pos,
		})

		if err != nil {
			return ui.PlatformSelectionOutput{}, gaba.ExitCodeError
		}

		// Store platform list positions
		gaba.Set(ctx, PlatformListPosition{
			Index: result.Value.LastSelectedIndex,
			Pos:   result.Value.LastSelectedPosition,
		})

		return result.Value, result.ExitCode
	}).
		OnWithHook(gaba.ExitCodeSuccess, GameList, func(ctx *gaba.Context) error {
			// Reset game list state when selecting a platform
			gaba.Set(ctx, SearchFilterString(""))
			gaba.Set(ctx, CurrentGamesList(nil))
			gaba.Set(ctx, GameListPosition{Index: 0, Pos: 0})
			return nil
		}).
		On(gaba.ExitCodeAction, Settings).
		Exit(gaba.ExitCodeQuit)

	gaba.AddState(fsm, GameList, func(ctx *gaba.Context) (ui.GameListOutput, gaba.ExitCode) {
		config, _ := gaba.Get[*models.Config](ctx)
		host, _ := gaba.Get[models.Host](ctx)
		platform, _ := gaba.Get[ui.PlatformSelectionOutput](ctx)
		games, _ := gaba.Get[CurrentGamesList](ctx)
		filter, _ := gaba.Get[SearchFilterString](ctx)
		pos, _ := gaba.Get[GameListPosition](ctx)

		screen := ui.NewGameListScreen()
		result, err := screen.Draw(ui.GameListInput{
			Config:               config,
			Host:                 host,
			Platform:             platform.SelectedPlatform,
			Games:                games,
			SearchFilter:         string(filter),
			LastSelectedIndex:    pos.Index,
			LastSelectedPosition: pos.Pos,
		})

		if err != nil {
			return ui.GameListOutput{}, gaba.ExitCodeError
		}

		// Store full games list and positions after screen returns
		gaba.Set(ctx, FullGamesList(result.Value.AllGames))
		gaba.Set(ctx, GameListPosition{
			Index: result.Value.LastSelectedIndex,
			Pos:   result.Value.LastSelectedPosition,
		})
		gaba.Set(ctx, SearchFilterString(result.Value.SearchFilter))

		return result.Value, result.ExitCode
	}).
		OnWithHook(gaba.ExitCodeSuccess, GameList, func(ctx *gaba.Context) error {
			// Download games and update state
			output, _ := gaba.Get[ui.GameListOutput](ctx)
			config, _ := gaba.Get[*models.Config](ctx)
			host, _ := gaba.Get[models.Host](ctx)
			platform, _ := gaba.Get[ui.PlatformSelectionOutput](ctx)
			filter, _ := gaba.Get[SearchFilterString](ctx)

			downloadScreen := ui.NewDownloadScreen()
			downloadOutput := downloadScreen.Execute(*config, host, platform.SelectedPlatform, output.SelectedGames, output.AllGames, string(filter))
			gaba.Set(ctx, CurrentGamesList(downloadOutput.AllGames))
			gaba.Set(ctx, SearchFilterString(downloadOutput.SearchFilter))
			return nil
		}).
		On(constants.ExitCodeSearch, Search).
		OnWithHook(constants.ExitCodeClearSearch, GameList, func(ctx *gaba.Context) error {
			// Clear search filter
			gaba.Set(ctx, SearchFilterString(""))
			fullGames, _ := gaba.Get[FullGamesList](ctx)
			gaba.Set(ctx, CurrentGamesList(fullGames))
			gaba.Set(ctx, GameListPosition{Index: 0, Pos: 0})
			return nil
		}).
		OnWithHook(gaba.ExitCodeBack, PlatformSelection, func(ctx *gaba.Context) error {
			// Clear games when going back
			gaba.Set(ctx, CurrentGamesList(nil))
			return nil
		}).
		On(constants.ExitCodeNoResults, Search)

	gaba.AddState(fsm, Search, func(ctx *gaba.Context) (ui.SearchOutput, gaba.ExitCode) {
		filter, _ := gaba.Get[SearchFilterString](ctx)

		screen := ui.NewSearchScreen()
		result, err := screen.Draw(ui.SearchInput{
			InitialText: string(filter),
		})

		if err != nil {
			return ui.SearchOutput{}, gaba.ExitCodeError
		}

		return result.Value, result.ExitCode
	}).
		OnWithHook(gaba.ExitCodeSuccess, GameList, func(ctx *gaba.Context) error {
			// Apply search filter
			output, _ := gaba.Get[ui.SearchOutput](ctx)
			gaba.Set(ctx, SearchFilterString(output.Query))
			fullGames, _ := gaba.Get[FullGamesList](ctx)
			gaba.Set(ctx, CurrentGamesList(fullGames))
			gaba.Set(ctx, GameListPosition{Index: 0, Pos: 0})
			return nil
		}).
		OnWithHook(gaba.ExitCodeBack, GameList, func(ctx *gaba.Context) error {
			// Cancel search
			gaba.Set(ctx, SearchFilterString(""))
			fullGames, _ := gaba.Get[FullGamesList](ctx)
			gaba.Set(ctx, CurrentGamesList(fullGames))
			return nil
		})

	gaba.AddState(fsm, Settings, func(ctx *gaba.Context) (ui.SettingsOutput, gaba.ExitCode) {
		config, _ := gaba.Get[*models.Config](ctx)
		cfw, _ := gaba.Get[constants.CFW](ctx)
		host, _ := gaba.Get[models.Host](ctx)

		screen := ui.NewSettingsScreen()
		result, err := screen.Draw(ui.SettingsInput{
			Config: config,
			CFW:    cfw,
			Host:   host,
		})

		if err != nil {
			return ui.SettingsOutput{}, gaba.ExitCodeError
		}

		return result.Value, result.ExitCode
	}).
		OnWithHook(gaba.ExitCodeSuccess, PlatformSelection, func(ctx *gaba.Context) error {
			output, _ := gaba.Get[ui.SettingsOutput](ctx)
			utils.SaveConfig(output.Config)
			gaba.Set(ctx, output.Config)
			return nil
		}).
		On(constants.ExitCodeEditMappings, SettingsPlatformMapping).
		On(gaba.ExitCodeBack, PlatformSelection)

	gaba.AddState(fsm, SettingsPlatformMapping, func(ctx *gaba.Context) (ui.PlatformMappingOutput, gaba.ExitCode) {
		host, _ := gaba.Get[models.Host](ctx)
		config, _ := gaba.Get[*models.Config](ctx)
		cfw, _ := gaba.Get[constants.CFW](ctx)

		screen := ui.NewPlatformMappingScreen()
		result, err := screen.Draw(ui.PlatformMappingInput{
			Host:           host,
			ApiTimeout:     config.ApiTimeout,
			CFW:            cfw,
			RomDirectory:   utils.GetRomDirectory(),
			AutoSelect:     false,
			HideBackButton: false,
		})

		if err != nil {
			return ui.PlatformMappingOutput{}, gaba.ExitCodeError
		}

		return result.Value, result.ExitCode
	}).
		OnWithHook(gaba.ExitCodeSuccess, Settings, func(ctx *gaba.Context) error {
			output, _ := gaba.Get[ui.PlatformMappingOutput](ctx)
			config, _ := gaba.Get[*models.Config](ctx)
			host, _ := gaba.Get[models.Host](ctx)

			config.DirectoryMappings = output.Mappings
			utils.SaveConfig(config)
			gaba.Set(ctx, config)
			gaba.Set(ctx, utils.GetMappedPlatforms(host, output.Mappings))
			return nil
		}).
		On(gaba.ExitCodeBack, Settings)

	return fsm.Start(PlatformSelection)
}
