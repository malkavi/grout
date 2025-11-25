package ui

import (
	"fmt"
	"grout/client"
	"grout/models"
	"grout/utils"
	"path/filepath"
	"slices"

	gaba "github.com/UncleJunVIP/gabagool/pkg/gabagool"
	"github.com/UncleJunVIP/nextui-pak-shared-functions/filebrowser"
	shared "github.com/UncleJunVIP/nextui-pak-shared-functions/models"
	"qlova.tech/sum"
)

type PlatformMappingScreen struct {
	Host models.Host
}

func InitPlatformMappingScreen(host models.Host) PlatformMappingScreen {
	return PlatformMappingScreen{
		Host: host,
	}
}

func (p PlatformMappingScreen) Name() sum.Int[models.ScreenName] {
	return models.ScreenNames.SettingsPlatformMapping
}

func (p PlatformMappingScreen) Draw() (settings interface{}, exitCode int, e error) {
	logger := gaba.GetLogger()
	//appState := state.GetAppState()

	c := client.NewRomMClient(p.Host)

	rommPlatforms, err := c.GetPlatforms()
	if err != nil {
		logger.Error("Error loading fetching RomM Platforms", "error", err)
		return nil, 0, err
	}

	fb := filebrowser.NewFileBrowser(logger)
	err = fb.CWD(utils.GetRomDirectory(), false)
	if err != nil {
		logger.Error("Error loading fetching ROM directories", "error", err)
		return nil, 1, err
	}

	unmapped := gaba.Option{
		DisplayName: "Unmapped",
		Value:       "",
	}

	var mappingOptions []gaba.ItemWithOptions

	for _, platform := range rommPlatforms {
		options := []gaba.Option{unmapped}

		for _, romDirectory := range fb.Items {
			options = append(options, gaba.Option{
				DisplayName: fmt.Sprintf("/Roms/%s", filepath.Base(romDirectory.Path)),
				Value:       filepath.Base(romDirectory.Path),
			})
		}

		if !slices.ContainsFunc(fb.Items, func(item shared.Item) bool {
			return platform.Slug == filepath.Base(item.Path)
		}) {
			options = append(options, gaba.Option{
				DisplayName: fmt.Sprintf("Create '%s'", utils.RomMSlugToMuOS(platform.Slug)),
				Value:       utils.RomMSlugToMuOS(platform.Slug),
			})
		}

		mappingOptions = append(mappingOptions, gaba.ItemWithOptions{
			Item: gaba.MenuItem{
				Text:     platform.DisplayName,
				Metadata: platform.Slug,
			},
			Options:        options,
			SelectedOption: 0,
		})

	}

	footerHelpItems := []gaba.FooterHelpItem{
		{ButtonName: "B", HelpText: "Cancel"},
		{ButtonName: "←→", HelpText: "Cycle"},
		{ButtonName: "Start", HelpText: "Save"},
	}

	result, err := gaba.OptionsList(
		"Rom Directory Mapping",
		mappingOptions,
		footerHelpItems,
	)

	if result.IsNone() {
		return nil, 1, nil
	}

	mapping := make(map[string]string)

	for _, m := range result.Unwrap().Items {
		rp := m.Item.Metadata.(string)
		rfd := m.Options[m.SelectedOption].Value.(string)

		if rfd != "" {
			mapping[rp] = rfd
		}
	}

	return nil, 0, nil
}
