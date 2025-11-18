package ui

import (
	"fmt"
	"grout/models"
	"grout/utils"
	"time"

	"github.com/UncleJunVIP/gabagool/pkg/gabagool"
	"github.com/UncleJunVIP/nextui-pak-shared-functions/common"
	shared "github.com/UncleJunVIP/nextui-pak-shared-functions/models"
	"qlova.tech/sum"
)

type DownloadArtScreen struct {
	Platform     models.Platform
	Games        shared.Items
	SearchFilter string
}

func InitDownloadArtScreen(platform models.Platform, games shared.Items, searchFilter string) models.Screen {
	return DownloadArtScreen{
		Platform:     platform,
		Games:        games,
		SearchFilter: searchFilter,
	}
}

func (a DownloadArtScreen) Name() sum.Int[models.ScreenName] {
	return models.ScreenNames.DownloadArt
}

func (a DownloadArtScreen) Draw() (value interface{}, exitCode int, e error) {
	var artPaths []string

	gabagool.ProcessMessage("Downloading art...",
		gabagool.ProcessMessageOptions{ShowThemeBackground: true}, func() (interface{}, error) {
			for _, game := range a.Games {
				artPath := utils.FindArt(a.Platform, game)

				if artPath != "" {
					artPaths = append(artPaths, artPath)
				}
			}
			return nil, nil
		})

	if len(artPaths) == 0 {
		gabagool.ProcessMessage("No art downloaded!",
			gabagool.ProcessMessageOptions{ShowThemeBackground: true}, func() (interface{}, error) {
				time.Sleep(time.Millisecond * 1500)
				return nil, nil
			})

		return nil, 404, nil
	} else if len(a.Games) > 1 {
		gabagool.ProcessMessage(fmt.Sprintf("Art downloaded for %d/%d games!", len(artPaths), len(a.Games)),
			gabagool.ProcessMessageOptions{ShowThemeBackground: true}, func() (interface{}, error) {
				time.Sleep(time.Millisecond * 1500)
				return nil, nil
			})
	}

	for _, artPath := range artPaths {
		result, err := gabagool.ConfirmationMessage("Use This Art?",
			[]gabagool.FooterHelpItem{
				{ButtonName: "B", HelpText: "No"},
				{ButtonName: "A", HelpText: "Yes"},
			},
			gabagool.MessageOptions{
				ImagePath: artPath,
			})

		if err != nil || result.IsNone() {
			common.DeleteFile(artPath)
		}
	}

	time.Sleep(time.Millisecond * 100)

	return nil, 0, nil
}
