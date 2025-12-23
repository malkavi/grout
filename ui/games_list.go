package ui

import (
	"errors"
	"fmt"
	"grout/constants"
	"grout/romm"
	"grout/utils"
	"slices"
	"strings"
	"time"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
	"github.com/BrandonKowalski/gabagool/v2/pkg/gabagool/i18n"
)

type fetchType int

const (
	ftPlatform fetchType = iota
	ftCollection
)

type GameListInput struct {
	Config               *utils.Config
	Host                 romm.Host
	Platform             romm.Platform
	Collection           romm.Collection
	Games                []romm.Rom
	SearchFilter         string
	LastSelectedIndex    int
	LastSelectedPosition int
}

type GameListOutput struct {
	SelectedGames        []romm.Rom
	Platform             romm.Platform
	Collection           romm.Collection
	SearchFilter         string
	AllGames             []romm.Rom
	LastSelectedIndex    int
	LastSelectedPosition int
}

type GameListScreen struct{}

func NewGameListScreen() *GameListScreen {
	return &GameListScreen{}
}

// isCollectionSet checks if a collection is set, accounting for all collection types
func isCollectionSet(c romm.Collection) bool {
	return c.ID != 0 || c.VirtualID != ""
}

func (s *GameListScreen) Draw(input GameListInput) (ScreenResult[GameListOutput], error) {
	games := input.Games

	if len(games) == 0 {
		loaded, err := s.loadGames(input)
		if err != nil {
			return withCode(GameListOutput{}, gaba.ExitCodeError), err
		}
		games = loaded
	}

	output := GameListOutput{
		Platform:             input.Platform,
		Collection:           input.Collection,
		SearchFilter:         input.SearchFilter,
		AllGames:             games,
		LastSelectedIndex:    input.LastSelectedIndex,
		LastSelectedPosition: input.LastSelectedPosition,
	}

	displayGames := utils.PrepareRomNames(games, *input.Config)

	// Filter out downloaded games if configured to do so
	if input.Config.DownloadedGamesDisplayOption == "filter" {
		filteredGames := make([]romm.Rom, 0, len(displayGames))
		for _, game := range displayGames {
			if !utils.IsGameDownloadedLocally(game, *input.Config) {
				filteredGames = append(filteredGames, game)
			}
		}
		displayGames = filteredGames
	}

	displayName := input.Platform.Name
	allGamesFilteredOut := false
	if isCollectionSet(input.Collection) {
		displayName = input.Collection.Name
		originalCount := len(displayGames)
		filteredGames := make([]romm.Rom, 0, len(displayGames))
		for _, game := range displayGames {
			if _, hasMapping := input.Config.DirectoryMappings[game.PlatformSlug]; hasMapping {
				filteredGames = append(filteredGames, game)
			}
		}
		displayGames = filteredGames

		allGamesFilteredOut = originalCount > 0 && len(displayGames) == 0

		if input.Platform.ID == 0 {
			for i := range displayGames {
				displayGames[i].DisplayName = fmt.Sprintf("[%s] %s", displayGames[i].PlatformSlug, displayGames[i].DisplayName)
			}
		} else {
			displayName = fmt.Sprintf("%s - %s", input.Collection.Name, input.Platform.Name)
		}
	}

	title := displayName
	if input.SearchFilter != "" {
		message := i18n.GetStringWithData("games_list_search_prefix", map[string]interface{}{"Query": input.SearchFilter})
		title = fmt.Sprintf("[%s] | %s", message, displayName)
		displayGames = filterList(displayGames, input.SearchFilter)
	}

	if len(displayGames) == 0 {
		if allGamesFilteredOut {
			s.showFilteredOutMessage(displayName)
		} else {
			s.showEmptyMessage(displayName, input.SearchFilter)
		}
		if input.SearchFilter != "" {
			return withCode(output, constants.ExitCodeNoResults), nil
		}
		if isCollectionSet(input.Collection) && input.Platform.ID != 0 {
			return withCode(output, constants.ExitCodeBackToCollectionPlatform), nil
		}
		if isCollectionSet(input.Collection) {
			return withCode(output, constants.ExitCodeBackToCollection), nil
		}
		return back(output), nil
	}

	menuItems := make([]gaba.MenuItem, len(displayGames))
	for i, game := range displayGames {
		menuItems[i] = gaba.MenuItem{
			Text:     game.DisplayName,
			Selected: false,
			Focused:  false,
			Metadata: game,
		}
	}

	options := gaba.DefaultListOptions(title, menuItems)
	options.SmallTitle = true
	options.EnableAction = true
	options.EnableMultiSelect = true
	options.EnableHelp = true

	options.HelpTitle = i18n.GetString("games_list_help_title")
	options.HelpText = strings.Split(i18n.GetString("games_list_help_body"), "\n")

	options.FooterHelpItems = []gaba.FooterHelpItem{
		{ButtonName: "B", HelpText: i18n.GetString("button_back")},
		{ButtonName: i18n.GetString("button_menu"), HelpText: i18n.GetString("button_help")},
		{ButtonName: "X", HelpText: i18n.GetString("button_search")},
		{ButtonName: "A", HelpText: i18n.GetString("button_select")},
	}

	options.SelectedIndex = input.LastSelectedIndex
	options.VisibleStartIndex = max(0, input.LastSelectedIndex-input.LastSelectedPosition)

	res, err := gaba.List(options)
	if err != nil {
		if errors.Is(err, gaba.ErrCancelled) {
			if input.SearchFilter != "" {
				output.SearchFilter = ""
				output.LastSelectedIndex = 0
				output.LastSelectedPosition = 0
				return withCode(output, constants.ExitCodeClearSearch), nil
			}
			if isCollectionSet(input.Collection) && input.Platform.ID != 0 {
				return withCode(output, constants.ExitCodeBackToCollectionPlatform), nil
			}
			if isCollectionSet(input.Collection) {
				return withCode(output, constants.ExitCodeBackToCollection), nil
			}
			return back(output), nil
		}
		return withCode(output, gaba.ExitCodeError), err
	}

	switch res.Action {
	case gaba.ListActionSelected:
		selectedGames := make([]romm.Rom, 0, len(res.Selected))
		for _, idx := range res.Selected {
			selectedGames = append(selectedGames, res.Items[idx].Metadata.(romm.Rom))
		}
		output.LastSelectedIndex = res.Selected[0]
		output.LastSelectedPosition = res.VisiblePosition
		output.SelectedGames = selectedGames
		return success(output), nil

	case gaba.ListActionTriggered:
		return withCode(output, constants.ExitCodeSearch), nil
	}

	if isCollectionSet(input.Collection) && input.Platform.ID != 0 {
		return withCode(output, constants.ExitCodeBackToCollectionPlatform), nil
	}
	if isCollectionSet(input.Collection) {
		return withCode(output, constants.ExitCodeBackToCollection), nil
	}
	return back(output), nil
}

func (s *GameListScreen) loadGames(input GameListInput) ([]romm.Rom, error) {
	config := input.Config
	host := input.Host
	platform := input.Platform
	collection := input.Collection

	id := platform.ID
	ft := ftPlatform
	displayName := platform.Name

	if isCollectionSet(collection) {
		id = collection.ID
		ft = ftCollection
		displayName = collection.Name
	}

	logger := gaba.GetLogger()

	var games []romm.Rom
	var loadErr error

	_, err := gaba.ProcessMessage(
		i18n.GetStringWithData("games_list_loading", map[string]interface{}{"Name": displayName}),
		gaba.ProcessMessageOptions{ShowThemeBackground: true},
		func() (interface{}, error) {
			roms, err := fetchList(config, host, id, ft)
			if err != nil {
				logger.Error("Error downloading game list", "error", err)
				loadErr = err
				return nil, err
			}
			games = roms
			return nil, nil
		},
	)

	if err != nil || loadErr != nil {
		return nil, fmt.Errorf("failed to load games: %w", err)
	}

	return games, nil
}

func (s *GameListScreen) showEmptyMessage(platformName, searchFilter string) {
	var message string
	if searchFilter != "" {
		message = i18n.GetStringWithData("games_list_no_results", map[string]interface{}{"Query": searchFilter})
	} else {
		message = i18n.GetStringWithData("games_list_no_games", map[string]interface{}{"Name": platformName})
	}

	gaba.ProcessMessage(
		message,
		gaba.ProcessMessageOptions{ShowThemeBackground: true},
		func() (interface{}, error) {
			time.Sleep(time.Second * 1)
			return nil, nil
		},
	)
}

func (s *GameListScreen) showFilteredOutMessage(collectionName string) {
	message := i18n.GetStringWithData("games_list_filtered_out", map[string]interface{}{"Name": collectionName})

	gaba.ProcessMessage(
		message,
		gaba.ProcessMessageOptions{ShowThemeBackground: true},
		func() (interface{}, error) {
			time.Sleep(time.Second * 1)
			return nil, nil
		},
	)
}

func fetchList(config *utils.Config, host romm.Host, queryID int, fetchType fetchType) ([]romm.Rom, error) {
	logger := gaba.GetLogger()

	rc := utils.GetRommClient(host, config.ApiTimeout)

	opt := romm.GetRomsQuery{
		Limit: 10000,
	}

	switch fetchType {
	case ftPlatform:
		opt.PlatformID = queryID
	case ftCollection:
		opt.CollectionID = queryID
	}

	res, err := rc.GetRoms(opt)
	if err != nil {
		return nil, err
	}
	logger.Debug("Fetched platform games", "count", len(res.Items), "total", res.Total)
	return res.Items, nil
}

func filterList(itemList []romm.Rom, filter string) []romm.Rom {
	var result []romm.Rom

	for _, item := range itemList {
		if strings.Contains(strings.ToLower(item.Name), strings.ToLower(filter)) {
			result = append(result, item)
		}
	}

	slices.SortFunc(result, func(a, b romm.Rom) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})

	return result
}
