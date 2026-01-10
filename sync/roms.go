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
// Format: " [YYYY-MM-DD HH-MM-SS-mmm]" e.g., " [2024-01-02 15-04-05-000]"
var timestampPattern = regexp.MustCompile(` \[\d{4}-\d{2}-\d{2} \d{2}-\d{2}-\d{2}-\d{3}\]$`)

// extractSaveBaseName strips the timestamp suffix from a remote save's filename
// to get the original base name for comparison with local saves.
// e.g., "Pokemon Red [2024-01-02 15-04-05-000]" -> "Pokemon Red"
func extractSaveBaseName(fileNameNoExt string) string {
	return timestampPattern.ReplaceAllString(fileNameNoExt, "")
}

type LocalRomFile struct {
	RomID       int
	RomName     string
	FSSlug      string
	FileName    string
	FilePath    string
	RemoteSaves []romm.Save
	SaveFile    *LocalSave
}

// baseName returns the ROM filename without extension, used for matching saves
func (lrf LocalRomFile) baseName() string {
	return strings.TrimSuffix(lrf.FileName, filepath.Ext(lrf.FileName))
}

func (lrf LocalRomFile) syncAction() SyncAction {
	hasLocal := lrf.SaveFile != nil
	baseName := lrf.baseName()

	// Check for remote saves that match this ROM's base name
	hasRemote := lrf.hasRemoteSaveForBaseName(baseName)

	switch {
	case !hasLocal && !hasRemote:
		return Skip
	case hasLocal && !hasRemote:
		return Upload
	case !hasLocal && hasRemote:
		return Download
	}

	// Both local and remote exist - compare timestamps
	// Truncate to second precision to avoid timestamp precision issues
	// API timestamps are typically second/millisecond precision, but filesystem is nanosecond
	localTime := lrf.SaveFile.LastModified.Truncate(time.Second)
	remoteSave := lrf.lastRemoteSaveForBaseName(baseName)
	remoteTime := remoteSave.UpdatedAt.Truncate(time.Second)

	switch localTime.Compare(remoteTime) {
	case -1:
		return Download
	case 1:
		return Upload
	default:
		return Skip
	}
}

// lastRemoteSaveForBaseName returns the most recent remote save that matches
// the given base name (after stripping timestamps from remote save filenames).
// This allows multiple local ROM files with different names but the same CRC32
// to each sync with their own set of remote saves.
func (lrf LocalRomFile) lastRemoteSaveForBaseName(baseName string) romm.Save {
	if len(lrf.RemoteSaves) == 0 {
		return romm.Save{}
	}

	// Filter saves to only those matching the base name
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

// hasRemoteSaveForBaseName checks if there's any remote save matching the given base name
func (lrf LocalRomFile) hasRemoteSaveForBaseName(baseName string) bool {
	for _, s := range lrf.RemoteSaves {
		if extractSaveBaseName(s.FileNameNoExt) == baseName {
			return true
		}
	}
	return false
}

// LocalRomScan holds the results of scanning local ROMs, keyed by platform fs_slug
type LocalRomScan map[string][]LocalRomFile

// ScanRoms scans all local ROM directories and matches with save files
func ScanRoms() LocalRomScan {
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

	config, _ := internal.LoadConfig()

	result = scanRomsByPlatform(baseRomDir, platformMap, config, currentCFW)

	totalRoms := 0
	for _, roms := range result {
		totalRoms += len(roms)
	}
	logger.Debug("Completed ROM scan", "platforms", len(result), "totalRoms", totalRoms)

	return result
}

func buildSaveFileMap(fsSlug string) map[string]*LocalSave {
	saveFiles := findSaveFiles(fsSlug)
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
					saveFileMap := buildSaveFileMap(fsSlug)
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

				romFolderName := ""
				if config != nil {
					if mapping, ok := config.DirectoryMappings[s]; ok && mapping.RelativePath != "" {
						romFolderName = mapping.RelativePath
					}
				}

				if romFolderName == "" {
					romFolderName = cfw.RomMFSSlugToCFW(s)
				}

				if romFolderName == "" {
					logger.Debug("No ROM folder mapping for fsSlug", "fsSlug", s)
					resultChan <- platformResult{fsSlug: s, roms: nil}
					return
				}

				romDir := filepath.Join(baseRomDir, romFolderName)

				if !fileutil.FileExists(romDir) {
					resultChan <- platformResult{fsSlug: s, roms: nil}
					return
				}

				saveFileMap := buildSaveFileMap(s)
				roms := scanRomDirectory(s, romDir, saveFileMap)
				resultChan <- platformResult{fsSlug: s, roms: roms}
				if len(roms) > 0 {
					logger.Debug("Found ROMs for platform", "fsSlug", s, "count", len(roms))
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
