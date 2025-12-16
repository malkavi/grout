package ui

import (
	"errors"
	"fmt"
	"grout/constants"
	"grout/utils"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"grout/romm"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

type PlatformMappingInput struct {
	Host           romm.Host
	ApiTimeout     time.Duration
	CFW            constants.CFW
	RomDirectory   string
	AutoSelect     bool
	HideBackButton bool
}

type PlatformMappingOutput struct {
	Mappings map[string]utils.DirectoryMapping
}

type PlatformMappingScreen struct{}

func NewPlatformMappingScreen() *PlatformMappingScreen {
	return &PlatformMappingScreen{}
}

func (s *PlatformMappingScreen) Draw(input PlatformMappingInput) (ScreenResult[PlatformMappingOutput], error) {
	logger := gaba.GetLogger()
	output := PlatformMappingOutput{Mappings: make(map[string]utils.DirectoryMapping)}

	rommPlatforms, err := s.fetchPlatforms(input)
	if err != nil {
		logger.Error("Error fetching RomM Platforms", "error", err)
		return withCode(output, gaba.ExitCodeError), err
	}

	romDirectories, err := s.getRomDirectories(input.RomDirectory)
	if err != nil {
		logger.Error("Error fetching ROM directories", "error", err)
		return withCode(output, gaba.ExitCodeBack), err
	}

	mappingOptions := s.buildMappingOptions(rommPlatforms, romDirectories, input)

	footerItems := []gaba.FooterHelpItem{
		{ButtonName: "←→", HelpText: "Cycle"},
		{ButtonName: "Start", HelpText: "Save"},
	}
	if !input.HideBackButton {
		footerItems = slices.Insert(footerItems, 0, gaba.FooterHelpItem{ButtonName: "B", HelpText: "Cancel"})
	}

	result, err := gaba.OptionsList(
		"Rom Directory Mapping",
		gaba.OptionListSettings{
			FooterHelpItems:   footerItems,
			DisableBackButton: input.HideBackButton,
		},
		mappingOptions,
	)

	if err != nil {
		if errors.Is(err, gaba.ErrCancelled) {
			return back(PlatformMappingOutput{}), nil
		}
		return withCode(PlatformMappingOutput{}, gaba.ExitCodeError), err
	}

	output.Mappings = s.buildMappingsFromResult(result.Items)

	if err := s.createDirectories(output.Mappings, input.RomDirectory, romDirectories); err != nil {
		logger.Error("Error creating directories", "error", err)
		return withCode(output, gaba.ExitCodeError), err
	}

	return success(output), nil
}

func (s *PlatformMappingScreen) fetchPlatforms(input PlatformMappingInput) ([]romm.Platform, error) {
	client := romm.NewClient(
		input.Host.URL(),
		romm.WithBasicAuth(input.Host.Username, input.Host.Password),
		romm.WithTimeout(input.ApiTimeout),
	)
	return client.GetPlatforms()
}

func (s *PlatformMappingScreen) getRomDirectories(romDir string) ([]os.DirEntry, error) {
	entries, err := os.ReadDir(romDir)
	if err != nil {
		gaba.ConfirmationMessage("ROM Directory Could Not Be Found!", []gaba.FooterHelpItem{
			{ButtonName: "B", HelpText: "Quit"},
		}, gaba.MessageOptions{})
		return nil, fmt.Errorf("failed to read ROM directory: %w", err)
	}

	var dirs []os.DirEntry
	for _, entry := range entries {
		if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") {
			dirs = append(dirs, entry)
		}
	}

	return dirs, nil
}

func (s *PlatformMappingScreen) buildMappingOptions(
	platforms []romm.Platform,
	romDirectories []os.DirEntry,
	input PlatformMappingInput,
) []gaba.ItemWithOptions {
	options := make([]gaba.ItemWithOptions, 0, len(platforms))

	for _, platform := range platforms {
		platformOptions, selectedIndex := s.buildPlatformOptions(platform, romDirectories, input)

		options = append(options, gaba.ItemWithOptions{
			Item: gaba.MenuItem{
				Text:     platform.Name,
				Metadata: platform.Slug,
			},
			Options:        platformOptions,
			SelectedOption: selectedIndex,
		})
	}

	return options
}

func (s *PlatformMappingScreen) buildPlatformOptions(
	platform romm.Platform,
	romDirectories []os.DirEntry,
	input PlatformMappingInput,
) ([]gaba.Option, int) {
	options := []gaba.Option{{DisplayName: "Skip", Value: ""}}
	selectedIndex := 0

	cfwDirectories := s.getCFWDirectoriesForPlatform(platform.Slug, input.CFW)

	createOptionAdded := false
	for _, cfwDir := range cfwDirectories {
		dirExists := false
		for _, romDir := range romDirectories {
			if s.directoriesMatch(cfwDir, romDir.Name(), input.CFW) {
				dirExists = true
				break
			}
		}

		if !dirExists {
			displayName := cfwDir
			if input.CFW == constants.NextUI {
				displayName = utils.ParseTag(cfwDir)
			}
			options = append(options, gaba.Option{
				DisplayName: fmt.Sprintf("Create '%s'", displayName),
				Value:       cfwDir,
			})
			createOptionAdded = true
		}
	}

	for _, romDir := range romDirectories {
		dirName := romDir.Name()

		if s.isValidDirectoryForPlatform(dirName, input.CFW, cfwDirectories) {
			displayName := dirName
			if input.CFW == constants.NextUI {
				displayName = utils.ParseTag(dirName)
			}

			options = append(options, gaba.Option{
				DisplayName: fmt.Sprintf("/%s", displayName),
				Value:       dirName,
			})

			if s.directoryMatchesPlatform(platform, romDir.Name(), input.CFW) {
				selectedIndex = len(options) - 1
			}
		}
	}

	if selectedIndex == 0 && createOptionAdded && input.AutoSelect {
		selectedIndex = 1
	}

	return options, selectedIndex
}

func (s *PlatformMappingScreen) findMatchingDirectory(
	platform romm.Platform,
	romDirectories []os.DirEntry,
	cfw constants.CFW,
) int {
	for i, entry := range romDirectories {
		if s.directoryMatchesPlatform(platform, entry.Name(), cfw) {
			return i
		}
	}
	return -1
}

func (s *PlatformMappingScreen) directoryMatchesPlatform(
	platform romm.Platform,
	dirName string,
	cfw constants.CFW,
) bool {
	cfwSlug := utils.RomMSlugToCFW(platform.Slug)
	romFolderBase := utils.RomFolderBase(dirName)

	switch cfw {
	case constants.NextUI:
		return utils.ParseTag(cfwSlug) == romFolderBase
	default:
		return cfwSlug == romFolderBase
	}
}

func (s *PlatformMappingScreen) getCFWDirectoriesForPlatform(slug string, cfw constants.CFW) []string {
	switch cfw {
	case constants.MuOS:
		return constants.MuOSPlatforms[slug]
	case constants.NextUI:
		return constants.NextUIPlatforms[slug]
	default:
		return []string{}
	}
}

func (s *PlatformMappingScreen) getSaveDirectoriesForPlatform(slug string, cfw constants.CFW) []string {
	switch cfw {
	case constants.MuOS:
		return constants.MuOSSaveDirectories[slug]
	case constants.NextUI:
		return constants.NextUISaves[slug]
	default:
		return []string{}
	}
}

func (s *PlatformMappingScreen) directoriesMatch(dir1, dir2 string, cfw constants.CFW) bool {
	if cfw == constants.NextUI {
		return utils.ParseTag(dir1) == utils.ParseTag(dir2)
	}
	return dir1 == dir2
}

func (s *PlatformMappingScreen) isValidDirectoryForPlatform(dirName string, cfw constants.CFW, cfwDirectories []string) bool {
	for _, cfwDir := range cfwDirectories {
		if s.directoriesMatch(cfwDir, dirName, cfw) {
			return true
		}
	}
	return false
}

func (s *PlatformMappingScreen) getCreateDisplayName(slug string, cfw constants.CFW) string {
	displayName := utils.RomMSlugToCFW(slug)
	if cfw == constants.NextUI {
		displayName = utils.ParseTag(displayName)
	}
	return displayName
}

func (s *PlatformMappingScreen) buildMappingsFromResult(items []gaba.ItemWithOptions) map[string]utils.DirectoryMapping {
	mappings := make(map[string]utils.DirectoryMapping)
	cfw := utils.GetCFW()

	for _, item := range items {
		rommSlug := item.Item.Metadata.(string)
		relativePath := item.Options[item.SelectedOption].Value.(string)

		if relativePath == "" {
			continue
		}

		saveDir := s.inferSaveDirectory(rommSlug, relativePath, cfw)

		mappings[rommSlug] = utils.DirectoryMapping{
			RomMSlug:      rommSlug,
			RelativePath:  relativePath,
			SaveDirectory: saveDir,
		}
	}

	return mappings
}

func (s *PlatformMappingScreen) inferSaveDirectory(slug, romDir string, cfw constants.CFW) string {
	saveDirectories := s.getSaveDirectoriesForPlatform(slug, cfw)

	for _, saveDir := range saveDirectories {
		if s.directoriesMatch(romDir, saveDir, cfw) {
			return saveDir
		}
	}

	if len(saveDirectories) > 0 {
		return saveDirectories[0]
	}

	return romDir
}

func (s *PlatformMappingScreen) createDirectories(
	mappings map[string]utils.DirectoryMapping,
	romDirectory string,
	existingDirs []os.DirEntry,
) error {
	logger := gaba.GetLogger()

	existingDirMap := make(map[string]bool)
	for _, dir := range existingDirs {
		existingDirMap[dir.Name()] = true
	}

	for _, mapping := range mappings {
		if existingDirMap[mapping.RelativePath] {
			continue
		}

		fullPath := filepath.Join(romDirectory, mapping.RelativePath)
		logger.Debug("Creating new ROM directory", "path", fullPath)

		if err := os.MkdirAll(fullPath, 0755); err != nil {
			logger.Error("Failed to create directory", "path", fullPath, "error", err)
			return fmt.Errorf("failed to create directory %s: %w", fullPath, err)
		}

		logger.Info("Created ROM directory", "path", fullPath)
	}

	return nil
}
