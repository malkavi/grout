package ui

import (
	"errors"
	"fmt"
	"grout/utils"
	"path/filepath"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

type syncReportInput struct {
	Results []utils.SyncResult
}

type syncReportOutput struct{}

type SyncReportScreen struct{}

func newSyncReportScreen() *SyncReportScreen {
	return &SyncReportScreen{}
}

func (s *SyncReportScreen) draw(input syncReportInput) (ScreenResult[syncReportOutput], error) {
	logger := gaba.GetLogger()
	output := syncReportOutput{}

	sections := s.buildSections(input.Results)

	options := gaba.DefaultInfoScreenOptions()
	options.Sections = sections
	options.ShowThemeBackground = false
	options.ShowScrollbar = true

	result, err := gaba.DetailScreen("Save Sync Summary", options, []gaba.FooterHelpItem{
		{ButtonName: "B", HelpText: "Close"},
	})

	if err != nil {
		if errors.Is(err, gaba.ErrCancelled) {
			return back(output), nil
		}
		logger.Error("Detail screen error", "error", err)
		return withCode(output, gaba.ExitCodeError), err
	}

	if result.Action == gaba.DetailActionCancelled {
		return back(output), nil
	}

	return success(output), nil
}

func (s *SyncReportScreen) buildSections(results []utils.SyncResult) []gaba.Section {
	sections := make([]gaba.Section, 0)

	uploadedCount := 0
	downloadedCount := 0
	skippedCount := 0
	failedCount := 0

	for _, r := range results {
		if !r.Success {
			failedCount++
			continue
		}
		switch r.Action {
		case utils.Upload:
			uploadedCount++
		case utils.Download:
			downloadedCount++
		case utils.Skip:
			skippedCount++
		}
	}

	summary := []gaba.MetadataItem{
		{Label: "Total Processed", Value: fmt.Sprintf("%d", len(results))},
	}

	if downloadedCount > 0 {
		summary = append(summary, gaba.MetadataItem{Label: "Downloaded", Value: fmt.Sprintf("%d", downloadedCount)})
	}

	if uploadedCount > 0 {
		summary = append(summary, gaba.MetadataItem{
			Label: "Uploaded", Value: fmt.Sprintf("%d", uploadedCount)})
	}

	if skippedCount > 0 {
		summary = append(summary, gaba.MetadataItem{
			Label: "Skipped", Value: fmt.Sprintf("%d", skippedCount)})
	}

	if failedCount > 0 {
		summary = append(summary, gaba.MetadataItem{
			Label: "Failed", Value: fmt.Sprintf("%d", failedCount)})
	}

	sections = append(sections, gaba.NewInfoSection("Summary", summary))

	if downloadedCount > 0 {
		downloadedFiles := ""
		for _, r := range results {
			if r.Success && r.Action == utils.Download {
				if downloadedFiles != "" {
					downloadedFiles += "\n"
				}
				downloadedFiles += filepath.Base(r.FilePath)
			}
		}
		sections = append(sections, gaba.NewDescriptionSection("Downloaded", downloadedFiles))
	}

	if uploadedCount > 0 {
		uploadedFiles := ""
		for _, r := range results {
			if r.Success && r.Action == utils.Upload {
				if uploadedFiles != "" {
					uploadedFiles += "\n"
				}
				uploadedFiles += filepath.Base(r.FilePath)
			}
		}
		sections = append(sections, gaba.NewDescriptionSection("Uploaded", uploadedFiles))
	}

	if failedCount > 0 {
		failedFiles := ""
		for _, r := range results {
			if !r.Success {
				if failedFiles != "" {
					failedFiles += "\n"
				}
				errorMsg := r.Error
				if errorMsg == "" {
					errorMsg = "Unknown error"
				}
				failedFiles += fmt.Sprintf("%s (%s): %s", r.GameName, r.Action, errorMsg)
			}
		}
		sections = append(sections, gaba.NewDescriptionSection("Failed", failedFiles))
	}

	return sections
}
