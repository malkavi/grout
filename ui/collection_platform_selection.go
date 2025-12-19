package ui

import (
	"errors"
	"fmt"
	"grout/romm"
	"grout/utils"
	"slices"
	"strings"
	"time"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
	"github.com/BrandonKowalski/gabagool/v2/pkg/gabagool/i18n"
)

type CollectionPlatformSelectionInput struct {
	Config               *utils.Config
	Host                 romm.Host
	Collection           romm.Collection
	CachedGames          []romm.Rom
	LastSelectedIndex    int
	LastSelectedPosition int
}

type CollectionPlatformSelectionOutput struct {
	SelectedPlatform     romm.Platform
	Collection           romm.Collection
	AllGames             []romm.Rom
	LastSelectedIndex    int
	LastSelectedPosition int
}

type CollectionPlatformSelectionScreen struct{}

func NewCollectionPlatformSelectionScreen() *CollectionPlatformSelectionScreen {
	return &CollectionPlatformSelectionScreen{}
}

func (s *CollectionPlatformSelectionScreen) Draw(input CollectionPlatformSelectionInput) (ScreenResult[CollectionPlatformSelectionOutput], error) {
	logger := gaba.GetLogger()
	output := CollectionPlatformSelectionOutput{
		Collection:           input.Collection,
		LastSelectedIndex:    input.LastSelectedIndex,
		LastSelectedPosition: input.LastSelectedPosition,
	}

	var allGames []romm.Rom
	if len(input.CachedGames) > 0 {
		allGames = input.CachedGames
	} else {
		var loadErr error
		_, err := gaba.ProcessMessage(
			i18n.GetStringWithData("games_list_loading", map[string]interface{}{"Name": input.Collection.Name}),
			gaba.ProcessMessageOptions{ShowThemeBackground: true},
			func() (interface{}, error) {
				rc := utils.GetRommClient(input.Host)
				opt := romm.GetRomsQuery{
					Limit: 10000,
				}

				// Use appropriate ID based on collection type
				if input.Collection.IsVirtual {
					opt.VirtualCollectionID = input.Collection.VirtualID
				} else if input.Collection.IsSmart {
					opt.SmartCollectionID = input.Collection.ID
				} else {
					opt.CollectionID = input.Collection.ID
				}

				res, err := rc.GetRoms(opt)
				if err != nil {
					logger.Error("Error downloading game list", "error", err)
					loadErr = err
					return nil, err
				}
				allGames = res.Items
				return nil, nil
			},
		)

		if err != nil || loadErr != nil {
			return withCode(output, gaba.ExitCodeError), err
		}
	}

	platformMap := make(map[int]romm.Platform)
	for _, game := range allGames {
		if _, exists := platformMap[game.PlatformID]; !exists {
			if _, hasMapping := input.Config.DirectoryMappings[game.PlatformSlug]; hasMapping {
				platformMap[game.PlatformID] = romm.Platform{
					ID:   game.PlatformID,
					Slug: game.PlatformSlug,
					Name: game.PlatformDisplayName,
				}
			}
		}
	}

	if len(platformMap) == 0 {
		gaba.ProcessMessage(
			i18n.GetStringWithData("collection_platform_no_mapped", map[string]interface{}{"Name": input.Collection.Name}),
			gaba.ProcessMessageOptions{ShowThemeBackground: true},
			func() (interface{}, error) {
				time.Sleep(time.Second * 2)
				return nil, nil
			},
		)
		return withCode(output, gaba.ExitCodeBack), nil
	}

	platforms := make([]romm.Platform, 0, len(platformMap))
	for _, platform := range platformMap {
		platforms = append(platforms, platform)
	}

	slices.SortFunc(platforms, func(a, b romm.Platform) int {
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})

	menuItems := make([]gaba.MenuItem, len(platforms))
	for i, platform := range platforms {
		gameCount := 0
		for _, game := range allGames {
			if game.PlatformID == platform.ID {
				gameCount++
			}
		}

		displayName := fmt.Sprintf("%s (%d)", platform.Name, gameCount)
		menuItems[i] = gaba.MenuItem{
			Text:     displayName,
			Selected: false,
			Focused:  false,
			Metadata: platform,
		}
	}

	footerItems := []gaba.FooterHelpItem{
		{ButtonName: "B", HelpText: i18n.GetString("button_back")},
		{ButtonName: "A", HelpText: i18n.GetString("button_select")},
	}

	title := i18n.GetStringWithData("collection_platform_title", map[string]interface{}{"Name": input.Collection.Name})
	options := gaba.DefaultListOptions(title, menuItems)
	options.SmallTitle = true
	options.FooterHelpItems = footerItems
	options.SelectedIndex = input.LastSelectedIndex
	options.VisibleStartIndex = max(0, input.LastSelectedIndex-input.LastSelectedPosition)

	sel, err := gaba.List(options)
	if err != nil {
		if errors.Is(err, gaba.ErrCancelled) {
			return back(output), nil
		}
		return withCode(output, gaba.ExitCodeError), err
	}

	switch sel.Action {
	case gaba.ListActionSelected:
		platform := sel.Items[sel.Selected[0]].Metadata.(romm.Platform)

		output.SelectedPlatform = platform
		output.AllGames = allGames
		output.LastSelectedIndex = sel.Selected[0]
		output.LastSelectedPosition = sel.VisiblePosition
		return success(output), nil

	default:
		return withCode(output, gaba.ExitCodeBack), nil
	}
}
