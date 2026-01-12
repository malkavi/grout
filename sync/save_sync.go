package sync

import (
	"errors"
	"fmt"
	"grout/cache"
	"grout/internal"
	"grout/internal/fileutil"
	"grout/internal/stringutil"
	"grout/romm"
	"os"
	"path/filepath"
	"strings"
	gosync "sync"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

var ErrOrphanRom = errors.New("orphan ROM")

type SaveSync struct {
	RomID    int
	RomName  string
	FSSlug   string
	GameBase string
	Local    *LocalSave
	Remote   romm.Save
	Action   SyncAction
}

type SyncAction string

const (
	Download SyncAction = "DOWNLOAD"
	Upload   SyncAction = "UPLOAD"
	Skip     SyncAction = "SKIP"
)

type SyncResult struct {
	GameName       string
	RomDisplayName string
	Action         SyncAction
	Success        bool
	Error          string
	Err            error
	FilePath       string
	UnmatchedSaves []UnmatchedSave
}

type UnmatchedSave struct {
	SavePath string
	FSSlug   string
}

type PendingFuzzyMatch struct {
	LocalFilename string
	LocalPath     string
	SavePath      string
	FSSlug        string
	MatchedRomID  int
	MatchedName   string
	Similarity    float64
}

func (s *SaveSync) Execute(host romm.Host, config *internal.Config) SyncResult {
	logger := gaba.GetLogger()

	displayName := s.RomName
	if displayName != "" {
		displayName = strings.TrimSuffix(displayName, filepath.Ext(displayName))
	}

	result := SyncResult{
		GameName:       s.GameBase,
		RomDisplayName: displayName,
		Action:         s.Action,
		Success:        false,
	}

	logger.Debug("Executing sync",
		"action", s.Action,
		"gameBase", s.GameBase,
		"romName", s.RomName,
		"romID", s.RomID)

	var err error
	switch s.Action {
	case Upload:
		result.FilePath, err = s.upload(host, config)
		logger.Debug("Upload complete", "filePath", result.FilePath, "err", err)
	case Download:
		if s.Local != nil {
			err = s.Local.backup()
			if err != nil {
				result.Error = err.Error()
				return result
			}
		}
		result.FilePath, err = s.download(host, config)
	case Skip:
		result.Success = true
		return result
	}

	if err != nil {
		result.Err = err
		result.Error = err.Error()
	} else {
		result.Success = true
	}

	return result
}

func (s *SaveSync) download(host romm.Host, config *internal.Config) (string, error) {
	logger := gaba.GetLogger()
	if config == nil {
		return "", fmt.Errorf("config is nil")
	}
	if s.RomID == 0 && s.Local == nil {
		return "", ErrOrphanRom
	}
	rc := romm.NewClientFromHost(host, config.ApiTimeout)

	logger.Debug("Downloading save", "saveID", s.Remote.ID, "downloadPath", s.Remote.DownloadPath)

	saveData, err := rc.DownloadSave(s.Remote.DownloadPath)
	if err != nil {
		return "", fmt.Errorf("failed to download save: %w", err)
	}

	var destDir string
	if s.Local != nil {
		// If there's already a local save, use its directory
		destDir = filepath.Dir(s.Local.Path)
	} else {
		var err error
		destDir, err = ResolveSavePath(s.FSSlug, s.RomID, config)
		if err != nil {
			return "", fmt.Errorf("cannot determine save location: %w", err)
		}
	}

	ext := normalizeExt(s.Remote.FileExtension)
	filename := s.GameBase + ext
	destPath := filepath.Join(destDir, filename)

	if s.Local != nil && s.Local.Path != destPath {
		defer func() { _ = os.Remove(s.Local.Path) }()
	}

	err = os.WriteFile(destPath, saveData, 0644)
	if err != nil {
		return "", fmt.Errorf("failed to write save file: %w", err)
	}

	err = os.Chtimes(destPath, s.Remote.UpdatedAt, s.Remote.UpdatedAt)
	if err != nil {
		return "", fmt.Errorf("failed to update file timestamp: %w", err)
	}

	logger.Debug("Downloaded save and set timestamp",
		"path", destPath,
		"remoteUpdatedAt", s.Remote.UpdatedAt)

	return destPath, nil
}

func (s *SaveSync) upload(host romm.Host, config *internal.Config) (string, error) {
	if s.Local == nil {
		return "", fmt.Errorf("cannot upload: no local save file")
	}
	if config == nil {
		return "", fmt.Errorf("config is nil")
	}
	if s.RomID == 0 {
		return "", ErrOrphanRom
	}

	rc := romm.NewClientFromHost(host, config.ApiTimeout)

	ext := normalizeExt(filepath.Ext(s.Local.Path))

	fileInfo, err := os.Stat(s.Local.Path)
	if err != nil {
		return "", fmt.Errorf("failed to get file info: %w", err)
	}
	modTime := fileInfo.ModTime()
	timestamp := modTime.Format("[2006-01-02 15-04-05-000]")

	filename := s.GameBase + " " + timestamp + ext
	tmp := filepath.Join(fileutil.TempDir(), "uploads", filename)

	err = fileutil.CopyFile(s.Local.Path, tmp)
	if err != nil {
		return "", err
	}

	// Get emulator from the save folder path
	emulator := filepath.Base(filepath.Dir(s.Local.Path))

	uploadedSave, err := rc.UploadSave(s.RomID, tmp, emulator)
	if err != nil {
		return "", err
	}

	err = os.Chtimes(s.Local.Path, uploadedSave.UpdatedAt, uploadedSave.UpdatedAt)
	if err != nil {
		return "", fmt.Errorf("failed to update file timestamp: %w", err)
	}

	return s.Local.Path, nil
}

func lookupRomID(romFile *LocalRomFile) (int, string) {
	logger := gaba.GetLogger()

	// Look up from the games cache
	if romID, romName, found := cache.GetCachedRomIDByFilename(romFile.FSSlug, romFile.FileName); found {
		logger.Debug("ROM lookup from cache", "fsSlug", romFile.FSSlug, "file", romFile.FileName, "romID", romID, "name", romName)
		return romID, romName
	}

	logger.Debug("No ROM found for file", "fsSlug", romFile.FSSlug, "file", romFile.FileName)
	return 0, ""
}

func lookupRomByHash(rc *romm.Client, romFile *LocalRomFile) (int, string) {
	logger := gaba.GetLogger()

	if romFile.FilePath == "" {
		return 0, ""
	}

	if !cache.ShouldAttemptLookup(romFile.FSSlug, romFile.FileName) {
		logger.Debug("Skipping hash lookup (cooldown active)", "file", romFile.FileName, "fsSlug", romFile.FSSlug)
		return 0, ""
	}

	crcHash, err := fileutil.ComputeCRC32(romFile.FilePath)
	if err != nil {
		logger.Debug("Failed to compute CRC32 hash", "file", romFile.FileName, "error", err)
		return 0, ""
	}

	logger.Debug("Looking up ROM by CRC32 hash", "file", romFile.FileName, "crc", crcHash)

	rom, err := rc.GetRomByHash(romm.GetRomByHashQuery{CrcHash: crcHash})
	if err == nil && rom.ID > 0 {
		logger.Info("Found ROM by CRC32 hash",
			"file", romFile.FileName,
			"crc", crcHash,
			"romID", rom.ID,
			"romName", rom.Name)
		_ = cache.SaveFilenameMapping(romFile.FSSlug, romFile.FileName, rom.ID, rom.Name)
		_ = cache.ClearFailedLookup(romFile.FSSlug, romFile.FileName)
		return rom.ID, rom.Name
	}

	sha1Hash, err := fileutil.ComputeSHA1(romFile.FilePath)
	if err != nil {
		logger.Debug("Failed to compute SHA1 hash", "file", romFile.FileName, "error", err)
		_ = cache.RecordFailedLookup(romFile.FSSlug, romFile.FileName)
		return 0, ""
	}

	logger.Debug("Looking up ROM by SHA1 hash", "file", romFile.FileName, "sha1", sha1Hash)

	rom, err = rc.GetRomByHash(romm.GetRomByHashQuery{Sha1Hash: sha1Hash})
	if err == nil && rom.ID > 0 {
		logger.Info("Found ROM by SHA1 hash",
			"file", romFile.FileName,
			"sha1", sha1Hash,
			"romID", rom.ID,
			"romName", rom.Name)
		_ = cache.SaveFilenameMapping(romFile.FSSlug, romFile.FileName, rom.ID, rom.Name)
		_ = cache.ClearFailedLookup(romFile.FSSlug, romFile.FileName)
		return rom.ID, rom.Name
	}

	// Both lookups failed - don't record yet, let fuzzy matching try first
	logger.Debug("ROM not found by hash", "file", romFile.FileName, "crc", crcHash, "sha1", sha1Hash)
	return 0, ""
}

const FuzzyMatchThreshold = 0.80

func lookupRomByFuzzyTitle(romFile *LocalRomFile) *PendingFuzzyMatch {
	logger := gaba.GetLogger()

	if romFile.FSSlug == "" || romFile.FileName == "" {
		return nil
	}

	logger.Debug("Starting fuzzy title search",
		"file", romFile.FileName,
		"fsSlug", romFile.FSSlug)

	games, err := cache.GetGamesForPlatform(romFile.FSSlug)
	if err != nil || len(games) == 0 {
		logger.Debug("No games in cache for fuzzy matching", "fsSlug", romFile.FSSlug, "error", err)
		return nil
	}

	localNormalized := stringutil.NormalizeForComparison(romFile.FileName)
	if localNormalized == "" {
		return nil
	}

	logger.Debug("Fuzzy search comparing against cached games",
		"file", romFile.FileName,
		"normalized", localNormalized,
		"candidateCount", len(games))

	var bestMatch *PendingFuzzyMatch
	var bestSimilarity float64

	for _, game := range games {
		remoteNormalized := stringutil.NormalizeForComparison(game.Name)
		if remoteNormalized == "" {
			continue
		}

		similarity := stringutil.BestSimilarity(localNormalized, remoteNormalized)
		if similarity >= FuzzyMatchThreshold && similarity > bestSimilarity {
			bestSimilarity = similarity
			bestMatch = &PendingFuzzyMatch{
				LocalFilename: stringutil.StripExtension(romFile.FileName),
				LocalPath:     romFile.FilePath,
				FSSlug:        romFile.FSSlug,
				MatchedRomID:  game.ID,
				MatchedName:   game.Name,
				Similarity:    similarity,
			}
		}
	}

	if bestMatch != nil {
		logger.Info("Fuzzy match found",
			"local", romFile.FileName,
			"matched", bestMatch.MatchedName,
			"similarity", fmt.Sprintf("%.0f%%", bestMatch.Similarity*100))
	} else {
		logger.Debug("No fuzzy match found above threshold",
			"file", romFile.FileName,
			"threshold", fmt.Sprintf("%.0f%%", FuzzyMatchThreshold*100))
	}

	return bestMatch
}

func FindSaveSyncs(host romm.Host, config *internal.Config) ([]SaveSync, []UnmatchedSave, []PendingFuzzyMatch, error) {
	return FindSaveSyncsFromScan(host, config, ScanRoms())
}

func FindSaveSyncsFromScan(host romm.Host, config *internal.Config, scanLocal LocalRomScan) ([]SaveSync, []UnmatchedSave, []PendingFuzzyMatch, error) {
	logger := gaba.GetLogger()
	if config == nil {
		return nil, nil, nil, fmt.Errorf("config is nil")
	}
	rc := romm.NewClientFromHost(host, config.ApiTimeout)

	logger.Debug("FindSaveSyncs: Scanned local ROMs", "platformCount", len(scanLocal))

	cm := cache.GetCacheManager()
	var platforms []romm.Platform
	var err error

	if cm != nil {
		platforms, err = cm.GetPlatforms()
	}
	if err != nil || len(platforms) == 0 {
		platforms, err = rc.GetPlatforms()
		if err != nil {
			logger.Error("FindSaveSyncs: Could not retrieve platforms", "error", err)
			return []SaveSync{}, nil, nil, err
		}
	}

	fsSlugToPlatformID := make(map[string]int)
	for _, p := range platforms {
		fsSlugToPlatformID[p.FSSlug] = p.ID
	}

	type platformFetchResult struct {
		fsSlug   string
		saves    []romm.Save
		hasError bool
	}

	resultChan := make(chan platformFetchResult, len(scanLocal))
	var wg gosync.WaitGroup

	for fsSlug := range scanLocal {
		platformID, ok := fsSlugToPlatformID[fsSlug]
		if !ok {
			logger.Debug("FindSaveSyncs: No platform ID for fsSlug", "fsSlug", fsSlug)
			continue
		}

		wg.Add(1)
		go func(fsSlug string, platformID int) {
			defer wg.Done()

			result := platformFetchResult{
				fsSlug: fsSlug,
			}

			platformSaves, err := rc.GetSaves(romm.SaveQuery{PlatformID: platformID})
			if err != nil {
				logger.Warn("FindSaveSyncs: Could not retrieve saves for platform", "fsSlug", fsSlug, "error", err)
				result.hasError = true
				resultChan <- result
				return
			}
			result.saves = platformSaves
			logger.Debug("FindSaveSyncs: Retrieved saves for platform", "fsSlug", fsSlug, "count", len(platformSaves))

			resultChan <- result
		}(fsSlug, platformID)
	}

	go func() {
		wg.Wait()
		close(resultChan)
	}()

	savesByRomID := make(map[int][]romm.Save)
	for result := range resultChan {
		if result.hasError {
			continue
		}

		for _, s := range result.saves {
			savesByRomID[s.RomID] = append(savesByRomID[s.RomID], s)
		}
	}

	var unmatched []UnmatchedSave
	var pendingFuzzy []PendingFuzzyMatch
	for fsSlug, localRoms := range scanLocal {
		for idx := range localRoms {
			romFile := &scanLocal[fsSlug][idx]

			if romFile.SaveFile == nil && len(savesByRomID) == 0 {
				continue
			}

			romID, romName := lookupRomID(romFile)

			if romID == 0 && romFile.SaveFile != nil {
				romID, romName = lookupRomByHash(rc, romFile)
			}

			if romID == 0 && romFile.SaveFile != nil {
				logger.Debug("Attempting fuzzy title match",
					"file", romFile.FileName,
					"fsSlug", romFile.FSSlug)
				fuzzyMatch := lookupRomByFuzzyTitle(romFile)
				if fuzzyMatch != nil {
					fuzzyMatch.SavePath = romFile.SaveFile.Path
					pendingFuzzy = append(pendingFuzzy, *fuzzyMatch)
					romFile.PendingFuzzyMatch = true // Mark to skip in sync building
					logger.Info("Fuzzy match candidate found",
						"local", romFile.FileName,
						"matched", fuzzyMatch.MatchedName,
						"similarity", fmt.Sprintf("%.0f%%", fuzzyMatch.Similarity*100))
				} else {
					_ = cache.RecordFailedLookup(romFile.FSSlug, romFile.FileName)
					unmatched = append(unmatched, UnmatchedSave{
						SavePath: romFile.SaveFile.Path,
						FSSlug:   fsSlug,
					})
					logger.Info("Save has local ROM but not in RomM",
						"save", filepath.Base(romFile.SaveFile.Path),
						"romFile", romFile.FileName,
						"fsSlug", fsSlug)
				}
				continue
			}

			if romID == 0 {
				continue
			}

			romFile.RomID = romID
			romFile.RomName = romName

			if saves, ok := savesByRomID[romID]; ok {
				romFile.RemoteSaves = saves
				logger.Debug("Found remote saves for ROM", "romName", romName, "saveCount", len(saves))
			}
		}
	}

	syncMap := make(map[string]SaveSync)
	for fsSlug, roms := range scanLocal {
		for _, r := range roms {
			if r.PendingFuzzyMatch {
				continue
			}
			// Skip unmatched ROMs - they're already in the unmatched list
			if r.RomID == 0 {
				continue
			}
			logger.Debug("Evaluating ROM for sync",
				"romName", r.RomName,
				"romID", r.RomID,
				"hasLocalSave", r.SaveFile != nil,
				"remoteSaveCount", len(r.RemoteSaves))
			action := r.syncAction()
			if action == Upload || action == Download {
				baseName := strings.TrimSuffix(r.FileName, filepath.Ext(r.FileName))

				var key string
				if r.SaveFile != nil {
					key = r.SaveFile.Path
				} else {
					key = fmt.Sprintf("download_%d_%s", r.RomID, baseName)
				}

				if _, exists := syncMap[key]; exists {
					continue
				}

				syncMap[key] = SaveSync{
					RomID:    r.RomID,
					RomName:  r.RomName,
					FSSlug:   fsSlug,
					GameBase: baseName,
					Local:    r.SaveFile,
					Remote:   r.lastRemoteSaveForBaseName(baseName),
					Action:   action,
				}
			}
		}
	}

	var syncs []SaveSync
	for _, s := range syncMap {
		syncs = append(syncs, s)
	}

	if len(unmatched) > 0 {
		logger.Info("Unmatched saves", "count", len(unmatched))
	}

	if len(pendingFuzzy) > 0 {
		logger.Info("Pending fuzzy matches", "count", len(pendingFuzzy))
	}

	return syncs, unmatched, pendingFuzzy, nil
}

func normalizeExt(ext string) string {
	if ext != "" && !strings.HasPrefix(ext, ".") {
		return "." + ext
	}
	return ext
}
