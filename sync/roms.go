package sync

import (
	"grout/cfw"
	"grout/internal"
	"grout/internal/fileutil"
	"grout/internal/stringutil"
	"grout/romm"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	gosync "sync"
	"time"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

// timestampPattern matches the timestamp suffix appended to save files
// Format: " [YYYY-MM-DD HH-MM-SS-mmm]" or " [YYYY-MM-DD HH-MM-SS-mmmZ]" (UTC)
var timestampPattern = regexp.MustCompile(` \[\d{4}-\d{2}-\d{2} \d{2}-\d{2}-\d{2}-\d{3}Z?\]$`)

// timestampExtractPattern captures the timestamp portion for parsing
// Group 1: timestamp, Group 2: optional Z indicator
var timestampExtractPattern = regexp.MustCompile(`\[(\d{4}-\d{2}-\d{2} \d{2}-\d{2}-\d{2})-\d{3}(Z?)\]$`)

// extractSaveBaseName strips the timestamp suffix from a remote save's filename
// to get the original base name for comparison with local saves.
// e.g., "Pokemon Red [2024-01-02 15-04-05-000]" -> "Pokemon Red"
func extractSaveBaseName(fileNameNoExt string) string {
	return timestampPattern.ReplaceAllString(fileNameNoExt, "")
}

// extractSaveTimestamp extracts and parses the timestamp from a remote save's filename.
// Returns the parsed time and true if successful, or zero time and false if no timestamp found.
// New format with Z suffix is UTC, old format without Z is local time (for backwards compat).
// e.g., "Pokemon Red [2024-01-02 15-04-05-000Z]" -> 2024-01-02 15:04:05 UTC
// e.g., "Pokemon Red [2024-01-02 15-04-05-000]" -> 2024-01-02 15:04:05 local
func extractSaveTimestamp(fileNameNoExt string) (time.Time, bool) {
	matches := timestampExtractPattern.FindStringSubmatch(fileNameNoExt)
	if len(matches) < 2 {
		return time.Time{}, false
	}
	// Parse format: "2024-01-02 15-04-05" (note: dashes instead of colons for time)
	// Convert to standard format for parsing
	tsStr := strings.ReplaceAll(matches[1], "-", ":")
	// Now it's "2024:01:02 15:04:05", need to fix the date part
	tsStr = strings.Replace(tsStr, ":", "-", 2) // First two colons back to dashes for date
	// Now it's "2024-01-02 15:04:05"

	// Check if Z indicator is present (UTC) or not (local time for backwards compat)
	isUTC := len(matches) >= 3 && matches[2] == "Z"
	var t time.Time
	var err error
	if isUTC {
		t, err = time.Parse("2006-01-02 15:04:05", tsStr)
	} else {
		t, err = time.ParseInLocation("2006-01-02 15:04:05", tsStr, time.Local)
	}
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

type LocalRomFile struct {
	RomID             int
	RomName           string
	FSSlug            string
	FileName          string
	FilePath          string
	RemoteSaves       []romm.Save
	SaveFile          *LocalSave
	PendingFuzzyMatch bool
}

func (lrf LocalRomFile) baseName() string {
	return strings.TrimSuffix(lrf.FileName, filepath.Ext(lrf.FileName))
}

func (lrf LocalRomFile) syncAction() Action {
	logger := gaba.GetLogger()
	hasLocal := lrf.SaveFile != nil
	baseName := lrf.baseName()

	hasRemote := lrf.hasRemoteSaveForBaseName(baseName)

	switch {
	case !hasLocal && !hasRemote:
		return Skip
	case hasLocal && !hasRemote:
		return Upload
	case !hasLocal:
		return Download
	}

	// Both local and remote exist - compare timestamps
	// Use the timestamp embedded in the remote save's filename (original save time)
	// rather than UpdatedAt (when uploaded to RomM) to avoid precision issues
	localTimeRaw := lrf.SaveFile.LastModified
	localTime := localTimeRaw.Truncate(time.Second)
	remoteSave := lrf.lastRemoteSaveForBaseName(baseName)

	remoteTime, parsedFromFilename := extractSaveTimestamp(remoteSave.FileNameNoExt)
	remoteTimeSource := "filename"
	if !parsedFromFilename {
		remoteTime = remoteSave.UpdatedAt
		remoteTimeSource = "UpdatedAt"
	}
	remoteTime = remoteTime.Truncate(time.Second)

	diff := localTime.Sub(remoteTime)
	cmp := localTime.Compare(remoteTime)

	var action Action
	var reason string
	switch cmp {
	case -1:
		action = Download
		reason = "local older"
	case 1:
		action = Upload
		reason = "local newer"
	default:
		action = Skip
		reason = "equal"
	}

	logger.Debug("Comparing save timestamps",
		"baseName", baseName,
		"localPath", lrf.SaveFile.Path,
		"localMtimeRaw", localTimeRaw.Format(time.RFC3339Nano),
		"localTime", localTime.Format(time.RFC3339),
		"localUnix", localTime.Unix(),
		"remoteFileName", remoteSave.FileNameNoExt,
		"remoteTimeSource", remoteTimeSource,
		"remoteTime", remoteTime.Format(time.RFC3339),
		"remoteUnix", remoteTime.Unix(),
		"diffSeconds", diff.Seconds(),
		"comparison", cmp,
		"action", action,
		"reason", reason)

	return action
}

func (lrf LocalRomFile) lastRemoteSaveForBaseName(baseName string) romm.Save {
	if len(lrf.RemoteSaves) == 0 {
		return romm.Save{}
	}

	var matching []romm.Save
	for _, s := range lrf.RemoteSaves {
		remoteBaseName := extractSaveBaseName(s.FileNameNoExt)
		if remoteBaseName == baseName {
			matching = append(matching, s)
		}
	}

	if len(matching) == 0 {
		return romm.Save{}
	}

	slices.SortFunc(matching, func(s1 romm.Save, s2 romm.Save) int {
		return s2.UpdatedAt.Compare(s1.UpdatedAt)
	})

	return matching[0]
}

func (lrf LocalRomFile) hasRemoteSaveForBaseName(baseName string) bool {
	for _, s := range lrf.RemoteSaves {
		if extractSaveBaseName(s.FileNameNoExt) == baseName {
			return true
		}
	}
	return false
}

type LocalRomScan map[string][]LocalRomFile

func ScanRoms(config *internal.Config) LocalRomScan {
	logger := gaba.GetLogger()
	result := make(map[string][]LocalRomFile)
	currentCFW := cfw.GetCFW()

	platformMap := cfw.GetPlatformMap(currentCFW)
	if platformMap == nil {
		logger.Warn("Unknown CFW, cannot scan ROMs")
		return result
	}

	baseRomDir := cfw.GetRomDirectory()
	logger.Debug("Starting ROM scan", "baseDir", baseRomDir)

	if config == nil {
		config, _ = internal.LoadConfig()
	}

	result = scanRomsByPlatform(baseRomDir, platformMap, config, currentCFW)

	totalRoms := 0
	for _, roms := range result {
		totalRoms += len(roms)
	}
	logger.Debug("Completed ROM scan", "platforms", len(result), "totalRoms", totalRoms)

	return result
}

func buildSaveFileMap(fsSlug string, config *internal.Config) map[string]*LocalSave {
	saveFiles := findSaveFiles(fsSlug, config)
	saveFileMap := make(map[string]*LocalSave)
	for i := range saveFiles {
		baseName := strings.TrimSuffix(filepath.Base(saveFiles[i].Path), filepath.Ext(saveFiles[i].Path))
		saveFileMap[baseName] = &saveFiles[i]
	}
	return saveFileMap
}

func scanRomsByPlatform(baseRomDir string, platformMap map[string][]string, config *internal.Config, currentCFW cfw.CFW) map[string][]LocalRomFile {
	logger := gaba.GetLogger()
	result := make(map[string][]LocalRomFile)

	if currentCFW == cfw.NextUI {
		entries, err := os.ReadDir(baseRomDir)
		if err != nil {
			logger.Error("Failed to read ROM directory", "path", baseRomDir, "error", err)
			return result
		}

		for _, entry := range entries {
			if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			dirName := entry.Name()
			tag := stringutil.ParseTag(dirName)
			if tag == "" {
				logger.Debug("No tag found in directory", "dir", dirName)
				continue
			}

			for fsSlug, cfwDirs := range platformMap {
				matched := false
				for _, cfwDir := range cfwDirs {
					cfwTag := stringutil.ParseTag(cfwDir)
					if cfwTag == tag {
						matched = true
						break
					}
				}

				if !matched {
					if config != nil {
						if mapping, ok := config.DirectoryMappings[fsSlug]; ok {
							if stringutil.ParseTag(mapping.RelativePath) == tag {
								matched = true
							}
						}
					}
				}

				if matched {
					romDir := filepath.Join(baseRomDir, dirName)
					saveFileMap := buildSaveFileMap(fsSlug, config)
					roms := scanRomDirectory(fsSlug, romDir, saveFileMap)
					if len(roms) > 0 {
						result[fsSlug] = append(result[fsSlug], roms...)
						logger.Debug("Found ROMs for platform", "fsSlug", fsSlug, "dir", dirName, "count", len(roms))
					}
				}
			}
		}
	} else {
		// Parallelize platform scanning for MuOS and Knulli
		type platformResult struct {
			fsSlug string
			roms   []LocalRomFile
		}

		resultChan := make(chan platformResult, len(platformMap))
		var wg gosync.WaitGroup

		for fsSlug := range platformMap {
			wg.Add(1)
			go func(s string) {
				defer wg.Done()

				// Resolve CFW platform key to RomM fs_slug via inverse platform binding
				// e.g., CFW "sms" -> RomM "ms" when binding is {"ms": "sms"}
				rommFSSlug := s
				if config != nil {
					rommFSSlug = config.ResolveRommFSSlug(s)
				}

				romFolderName := ""
				if config != nil {
					if mapping, ok := config.DirectoryMappings[rommFSSlug]; ok && mapping.RelativePath != "" {
						romFolderName = mapping.RelativePath
					}
				}

				if romFolderName == "" {
					romFolderName = cfw.RomMFSSlugToCFW(s)
				}

				if romFolderName == "" {
					logger.Debug("No ROM folder mapping for fsSlug", "fsSlug", rommFSSlug)
					resultChan <- platformResult{fsSlug: rommFSSlug, roms: nil}
					return
				}

				romDir := filepath.Join(baseRomDir, romFolderName)

				if !fileutil.FileExists(romDir) {
					resultChan <- platformResult{fsSlug: rommFSSlug, roms: nil}
					return
				}

				saveFileMap := buildSaveFileMap(rommFSSlug, config)
				roms := scanRomDirectory(rommFSSlug, romDir, saveFileMap)
				resultChan <- platformResult{fsSlug: rommFSSlug, roms: roms}
				if len(roms) > 0 {
					logger.Debug("Found ROMs for platform", "fsSlug", rommFSSlug, "count", len(roms))
				}
			}(fsSlug)
		}

		// Close channel once all goroutines complete
		go func() {
			wg.Wait()
			close(resultChan)
		}()

		// Collect results from all platforms
		for pr := range resultChan {
			if len(pr.roms) > 0 {
				result[pr.fsSlug] = pr.roms
			}
		}
	}

	return result
}

func scanRomDirectory(fsSlug, romDir string, saveFileMap map[string]*LocalSave) []LocalRomFile {
	logger := gaba.GetLogger()
	var roms []LocalRomFile

	entries, err := os.ReadDir(romDir)
	if err != nil {
		logger.Error("Failed to read ROM directory", "path", romDir, "error", err)
		return roms
	}

	visibleFiles := fileutil.FilterVisibleFiles(entries)
	for _, entry := range visibleFiles {
		baseName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		var saveFile *LocalSave
		if sf, found := saveFileMap[baseName]; found {
			saveFile = sf
		}

		rom := LocalRomFile{
			FSSlug:   fsSlug,
			FileName: entry.Name(),
			FilePath: filepath.Join(romDir, entry.Name()),
			SaveFile: saveFile,
		}

		roms = append(roms, rom)
	}

	return roms
}
