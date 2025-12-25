package ui

import (
	"fmt"
	"os"
	"path/filepath"

	"grout/constants"
	"grout/romm"
	"grout/utils"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
	"github.com/BrandonKowalski/gabagool/v2/pkg/gabagool/i18n"
)

type BIOSDownloadInput struct {
	Config   utils.Config
	Host     romm.Host
	Platform romm.Platform
}

type BIOSDownloadOutput struct {
	Platform romm.Platform
}

type BIOSDownloadScreen struct{}

func NewBIOSDownloadScreen() *BIOSDownloadScreen {
	return &BIOSDownloadScreen{}
}

func (s *BIOSDownloadScreen) Execute(config utils.Config, host romm.Host, platform romm.Platform) BIOSDownloadOutput {
	result, err := s.draw(BIOSDownloadInput{
		Config:   config,
		Host:     host,
		Platform: platform,
	})

	if err != nil {
		gaba.GetLogger().Error("BIOS download failed", "error", err)
		return BIOSDownloadOutput{Platform: platform}
	}

	return result.Value
}

func (s *BIOSDownloadScreen) draw(input BIOSDownloadInput) (ScreenResult[BIOSDownloadOutput], error) {
	logger := gaba.GetLogger()

	output := BIOSDownloadOutput{
		Platform: input.Platform,
	}

	biosFiles := utils.GetBIOSFilesForPlatform(input.Platform.Slug)

	if len(biosFiles) == 0 {
		logger.Info("No BIOS files required for platform", "platform", input.Platform.Name)
		gaba.ConfirmationMessage(
			i18n.GetString("bios_no_files_required"),
			[]gaba.FooterHelpItem{{ButtonName: "A", HelpText: i18n.GetString("button_continue")}},
			gaba.MessageOptions{},
		)
		return back(output), nil
	}

	client := utils.GetRommClient(input.Host, input.Config.ApiTimeout)
	firmwareList, err := client.GetFirmware(input.Platform.ID)
	if err != nil {
		logger.Error("Failed to fetch firmware from RomM", "error", err, "platform_id", input.Platform.ID)
		gaba.ConfirmationMessage(
			fmt.Sprintf("Failed to fetch BIOS files from RomM: %v", err),
			[]gaba.FooterHelpItem{{ButtonName: "A", HelpText: i18n.GetString("button_continue")}},
			gaba.MessageOptions{},
		)
		return back(output), nil
	}

	logger.Debug("Fetched firmware from RomM", "count", len(firmwareList), "platform_id", input.Platform.ID)

	firmwareByFileName := make(map[string]romm.Firmware)
	firmwareByFilePath := make(map[string]romm.Firmware)
	firmwareByBaseName := make(map[string]romm.Firmware)

	for _, fw := range firmwareList {
		firmwareByFileName[fw.FileName] = fw
		firmwareByFilePath[fw.FilePath] = fw
		baseName := filepath.Base(fw.FilePath)
		firmwareByBaseName[baseName] = fw

		logger.Debug("RomM firmware entry",
			"filename", fw.FileName,
			"filepath", fw.FilePath,
			"size", fw.FileSizeBytes,
			"md5", fw.MD5Hash,
			"verified", fw.IsVerified)
	}

	var availableBIOSFiles []constants.BIOSFile
	for _, biosFile := range biosFiles {
		if s.firmwareExistsInRomM(biosFile, firmwareByFileName, firmwareByFilePath, firmwareByBaseName) {
			availableBIOSFiles = append(availableBIOSFiles, biosFile)
		} else {
			logger.Debug("BIOS file not available in RomM, skipping",
				"file", biosFile.FileName,
				"relativePath", biosFile.RelativePath)
		}
	}

	if len(availableBIOSFiles) == 0 {
		logger.Info("No BIOS files available in RomM for platform", "platform", input.Platform.Name)
		return back(output), nil
	}

	var menuItems []gaba.MenuItem
	var biosStatusMap = make(map[string]utils.BIOSFileStatus)

	for _, biosFile := range availableBIOSFiles {
		status := utils.CheckBIOSFileStatus(biosFile, input.Platform.Slug)
		biosStatusMap[biosFile.FileName] = status

		var statusText string
		switch status.Status {
		case utils.BIOSStatusValid:
			statusText = i18n.GetString("bios_status_ready")
		case utils.BIOSStatusInvalidHash:
			statusText = i18n.GetString("bios_status_wrong_version")
		case utils.BIOSStatusNoHashToVerify:
			statusText = i18n.GetString("bios_status_unverified")
		case utils.BIOSStatusMissing:
			statusText = i18n.GetString("bios_status_not_installed")
		}

		optionalText := ""
		if biosFile.Optional {
			optionalText = " (Optional)"
		}

		displayText := fmt.Sprintf("%s%s - %s", biosFile.FileName, optionalText, statusText)

		menuItems = append(menuItems, gaba.MenuItem{
			Text:     displayText,
			Selected: status.Status == utils.BIOSStatusMissing || status.Status == utils.BIOSStatusInvalidHash,
			Focused:  false,
			Metadata: biosFile,
		})
	}

	options := gaba.DefaultListOptions(fmt.Sprintf("%s - BIOS Files", input.Platform.Name), menuItems)
	options.SmallTitle = true
	options.StartInMultiSelectMode = true
	options.FooterHelpItems = []gaba.FooterHelpItem{
		{ButtonName: "B", HelpText: i18n.GetString("button_back")},
		{ButtonName: "Start", HelpText: i18n.GetString("button_download")},
	}

	sel, err := gaba.List(options)
	if err != nil {
		logger.Error("BIOS selection failed", "error", err)
		return back(output), err
	}

	if sel.Action != gaba.ListActionSelected || len(sel.Selected) == 0 {
		return back(output), nil
	}

	var selectedBIOSFiles []constants.BIOSFile
	for _, idx := range sel.Selected {
		biosFile := sel.Items[idx].Metadata.(constants.BIOSFile)
		selectedBIOSFiles = append(selectedBIOSFiles, biosFile)
	}

	logger.Debug("Selected BIOS files for download", "count", len(selectedBIOSFiles))

	downloads, locationToBIOSMap := s.buildDownloads(input.Host, selectedBIOSFiles, firmwareList)

	if len(downloads) == 0 {
		logger.Warn("No BIOS files available in RomM")
		return back(output), nil
	}

	headers := make(map[string]string)
	headers["Authorization"] = input.Host.BasicAuthHeader()

	res, err := gaba.DownloadManager(downloads, headers, gaba.DownloadManagerOptions{
		AutoContinue: true,
	})
	if err != nil {
		logger.Error("BIOS download failed", "error", err)
		return back(output), err
	}

	logger.Debug("Download results", "completed", len(res.Completed), "failed", len(res.Failed))

	successCount := 0
	warningCount := 0
	for _, download := range res.Completed {
		biosFile := locationToBIOSMap[download.Location]

		data, err := os.ReadFile(download.Location)
		if err != nil {
			logger.Error("Failed to read downloaded BIOS file", "file", biosFile.FileName, "error", err)
			continue
		}

		if biosFile.MD5Hash != "" {
			isValid, actualHash := utils.VerifyBIOSFileMD5(data, biosFile.MD5Hash)
			if !isValid {
				logger.Warn("MD5 hash mismatch for BIOS file",
					"file", biosFile.FileName,
					"expected", biosFile.MD5Hash,
					"actual", actualHash)
				warningCount++
			}
		}

		if err := utils.SaveBIOSFile(biosFile, input.Platform.Slug, data); err != nil {
			logger.Error("Failed to save BIOS file", "file", biosFile.FileName, "error", err)
			continue
		}

		os.Remove(download.Location)
		successCount++
		logger.Debug("Successfully saved BIOS file", "file", biosFile.FileName, "paths", utils.GetBIOSFilePaths(biosFile, input.Platform.Slug))
	}

	// Show completion message to user
	if successCount > 0 && warningCount == 0 {
		logger.Info("BIOS download complete", "success", successCount)
		gaba.ConfirmationMessage(
			fmt.Sprintf(i18n.GetString("bios_download_complete"), successCount),
			[]gaba.FooterHelpItem{{ButtonName: "A", HelpText: i18n.GetString("button_continue")}},
			gaba.MessageOptions{},
		)
	} else if successCount > 0 && warningCount > 0 {
		logger.Warn("BIOS download complete with warnings",
			"success", successCount,
			"warnings", warningCount)
		gaba.ConfirmationMessage(
			fmt.Sprintf(i18n.GetString("bios_download_complete_with_warnings"), successCount, warningCount),
			[]gaba.FooterHelpItem{{ButtonName: "A", HelpText: i18n.GetString("button_continue")}},
			gaba.MessageOptions{},
		)
	} else if len(res.Failed) > 0 {
		logger.Error("BIOS download failed", "failed", len(res.Failed))
		gaba.ConfirmationMessage(
			fmt.Sprintf(i18n.GetString("bios_download_failed"), len(res.Failed)),
			[]gaba.FooterHelpItem{{ButtonName: "A", HelpText: i18n.GetString("button_continue")}},
			gaba.MessageOptions{},
		)
	}

	return back(output), nil
}

func (s *BIOSDownloadScreen) firmwareExistsInRomM(
	biosFile constants.BIOSFile,
	firmwareByFileName map[string]romm.Firmware,
	firmwareByFilePath map[string]romm.Firmware,
	firmwareByBaseName map[string]romm.Firmware,
) bool {
	// Try exact filename match
	if _, found := firmwareByFileName[biosFile.FileName]; found {
		return true
	}

	// Try relative path match
	if _, found := firmwareByFilePath[biosFile.RelativePath]; found {
		return true
	}

	// Try basename from filename
	if _, found := firmwareByBaseName[biosFile.FileName]; found {
		return true
	}

	// Try basename from relative path
	baseName := filepath.Base(biosFile.RelativePath)
	if _, found := firmwareByBaseName[baseName]; found {
		return true
	}

	return false
}

func (s *BIOSDownloadScreen) buildDownloads(host romm.Host, biosFiles []constants.BIOSFile, firmwareList []romm.Firmware) ([]gaba.Download, map[string]constants.BIOSFile) {
	var downloads []gaba.Download
	locationToBIOSMap := make(map[string]constants.BIOSFile)

	logger := gaba.GetLogger()
	baseURL := host.URL()

	firmwareByFileName := make(map[string]romm.Firmware)
	firmwareByFilePath := make(map[string]romm.Firmware)
	firmwareByBaseName := make(map[string]romm.Firmware)

	for _, fw := range firmwareList {
		firmwareByFileName[fw.FileName] = fw
		firmwareByFilePath[fw.FilePath] = fw
		baseName := filepath.Base(fw.FilePath)
		firmwareByBaseName[baseName] = fw
	}

	for _, biosFile := range biosFiles {
		var firmware romm.Firmware
		var found bool
		var matchStrategy string

		firmware, found = firmwareByFileName[biosFile.FileName]
		if found {
			matchStrategy = "exact_filename"
		}
		if !found {
			firmware, found = firmwareByFilePath[biosFile.RelativePath]
			if found {
				matchStrategy = "relative_path"
			}
		}
		if !found {
			firmware, found = firmwareByBaseName[biosFile.FileName]
			if found {
				matchStrategy = "basename_filename"
			}
		}
		if !found {
			baseName := filepath.Base(biosFile.RelativePath)
			firmware, found = firmwareByBaseName[baseName]
			if found {
				matchStrategy = "basename_relativepath"
			}
		}

		if !found {
			logger.Warn("BIOS file not found in RomM firmware list",
				"file", biosFile.FileName,
				"relativePath", biosFile.RelativePath)
			continue
		}

		logger.Debug("Matched BIOS file with RomM firmware",
			"biosFile", biosFile.FileName,
			"firmware", firmware.FileName,
			"strategy", matchStrategy)

		downloadURL := baseURL + firmware.DownloadURL

		tempPath := filepath.Join(utils.TempDir(), fmt.Sprintf("bios_%s", biosFile.FileName))

		downloads = append(downloads, gaba.Download{
			URL:         downloadURL,
			Location:    tempPath,
			DisplayName: biosFile.FileName,
		})

		locationToBIOSMap[tempPath] = biosFile

		logger.Debug("Added BIOS file to download queue",
			"file", biosFile.FileName,
			"url", downloadURL,
			"size", firmware.FileSizeBytes)
	}

	return downloads, locationToBIOSMap
}
