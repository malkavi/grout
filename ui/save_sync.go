package ui

import (
	"grout/romm"
	"grout/utils"
	"time"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

type SaveSyncInput struct {
	Config *utils.Config
	Host   romm.Host
}

type SaveSyncOutput struct{}

type SaveSyncScreen struct{}

func NewSaveSyncScreen() *SaveSyncScreen {
	return &SaveSyncScreen{}
}

func (s *SaveSyncScreen) Draw(input SaveSyncInput) (ScreenResult[SaveSyncOutput], error) {
	output := SaveSyncOutput{}

	syncResults, _ := gaba.ProcessMessage("Scanning save files...", gaba.ProcessMessageOptions{}, func() (interface{}, error) {
		syncs, err := utils.FindSaveSyncs(input.Host)
		if err != nil {
			gaba.GetLogger().Error("Unable to scan save files!", "error", err)
			return nil, nil
		}

		results := make([]utils.SyncResult, 0, len(syncs))
		for _, s := range syncs {
			gaba.GetLogger().Debug("Syncing save file", "save_info", s)
			result := s.Execute(input.Host)
			results = append(results, result)
			if !result.Success {
				gaba.GetLogger().Error("Unable to sync save!", "game", s.GameBase, "error", result.Error)
			} else {
				gaba.GetLogger().Debug("Save synced!", "save_info", s)
			}
		}

		return results, nil
	})

	if len(syncResults.([]utils.SyncResult)) > 0 {
		if results, ok := syncResults.([]utils.SyncResult); ok && len(results) > 0 {
			reportScreen := newSyncReportScreen()
			_, err := reportScreen.draw(syncReportInput{Results: results})
			if err != nil {
				gaba.GetLogger().Error("Error showing sync report", "error", err)
			}
		}
	} else {
		gaba.ProcessMessage("Everything is up to date!\nGo play some games!", gaba.ProcessMessageOptions{}, func() (interface{}, error) {
			time.Sleep(time.Second * 2)
			return nil, nil
		})
	}

	return back(output), nil
}
