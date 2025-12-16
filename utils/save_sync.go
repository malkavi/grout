package utils

import (
	"fmt"
	"grout/romm"
	"os"
	"path/filepath"
	"slices"
	"strings"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

type saveSync struct {
	RomID    int
	Slug     string
	GameBase string
	Local    *localSave
	Remote   romm.Save
	Action   syncAction
}

type syncAction string

const (
	Download syncAction = "DOWNLOAD"
	Upload              = "UPLOAD"
	Skip                = "SKIP"
)

type SyncResult struct {
	GameName string
	Action   syncAction
	Success  bool
	Error    string
	FilePath string
}

func (s saveSync) Execute(host romm.Host) SyncResult {
	result := SyncResult{
		GameName: s.GameBase,
		Action:   s.Action,
		Success:  false,
	}

	var err error
	switch s.Action {
	case Upload:
		result.FilePath, err = s.upload(host)
	case Download:
		if s.Local != nil {
			err = s.Local.backup()
			if err != nil {
				result.Error = err.Error()
				return result
			}
		}
		result.FilePath, err = s.download(host)
	case Skip:
		result.Success = true
		return result
	}

	if err != nil {
		result.Error = err.Error()
	} else {
		result.Success = true
	}

	return result
}

func (s saveSync) download(host romm.Host) (string, error) {
	logger := gaba.GetLogger()
	rc := GetRommClient(host)

	logger.Debug("Downloading save", "saveID", s.Remote.ID, "downloadPath", s.Remote.DownloadPath)

	saveData, err := rc.DownloadSave(s.Remote.DownloadPath)
	if err != nil {
		return "", fmt.Errorf("failed to download save: %w", err)
	}

	var destDir string
	if s.Local != nil {
		destDir = filepath.Dir(s.Local.Path)
	} else {
		var err error
		destDir, err = getSaveDirectoryForSlug(s.Slug, s.Remote.Emulator)
		if err != nil {
			return "", fmt.Errorf("cannot determine save location: %w", err)
		}
	}

	ext := s.Remote.FileExtension
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	filename := s.GameBase + ext
	destPath := filepath.Join(destDir, filename)

	if s.Local != nil && s.Local.Path != destPath {
		defer os.Remove(s.Local.Path)
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

func (s saveSync) upload(host romm.Host) (string, error) {
	if s.Local == nil {
		return "", fmt.Errorf("cannot upload: no local save file")
	}

	rc := GetRommClient(host)

	ext := filepath.Ext(s.Local.Path)
	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}
	filename := s.GameBase + ext
	tmp := filepath.Join(TempDir(), "uploads", filename)

	err := copyFile(s.Local.Path, tmp)
	if err != nil {
		return "", err
	}

	uploadedSave, err := rc.UploadSave(s.RomID, tmp)
	if err != nil {
		return "", err
	}

	err = os.Chtimes(s.Local.Path, uploadedSave.UpdatedAt, uploadedSave.UpdatedAt)
	if err != nil {
		return "", fmt.Errorf("failed to update file timestamp: %w", err)
	}

	return s.Local.Path, nil
}

func FindSaveSyncs(host romm.Host) ([]saveSync, error) {
	logger := gaba.GetLogger()
	rc := GetRommClient(host)

	logger.Debug("FindSaveSyncs: Starting save sync discovery")

	scanLocal := scanAllRoms()
	logger.Debug("FindSaveSyncs: Scanned local ROMs", "platformCount", len(scanLocal))

	allSaves, err := rc.GetSaves(romm.SaveQuery{})
	if err != nil {
		logger.Error("FindSaveSyncs: Could not retrieve saves", "error", err)
		return []saveSync{}, err
	}
	logger.Debug("FindSaveSyncs: Retrieved all saves", "count", len(*allSaves))

	savesByRomID := make(map[int][]romm.Save)
	for _, save := range *allSaves {
		savesByRomID[save.RomID] = append(savesByRomID[save.RomID], save)
	}

	plats, err := rc.GetPlatforms()
	if err != nil {
		logger.Error("FindSaveSyncs: Could not retrieve platforms", "error", err)
		return []saveSync{}, err
	}
	logger.Debug("FindSaveSyncs: Retrieved platforms from API", "count", len(plats))

	for slug, localRoms := range scanLocal {
		logger.Debug("FindSaveSyncs: Processing platform", "slug", slug, "localRomCount", len(localRoms))

		idx := slices.IndexFunc(plats, func(p romm.Platform) bool {
			return p.Slug == slug
		})

		if idx == -1 {
			logger.Warn("FindSaveSyncs: Platform not found in API", "slug", slug)
			continue
		}

		platform := plats[idx]
		logger.Debug("FindSaveSyncs: Found platform match", "slug", slug, "platformID", platform.ID, "platformName", platform.Name)

		roms, err := rc.GetRoms(&romm.GetRomsOptions{PlatformID: &platform.ID})
		if err != nil {
			logger.Error("FindSaveSyncs: Could not retrieve roms", "platform", platform, "error", err)
			continue
		}
		logger.Debug("FindSaveSyncs: Retrieved remote ROMs", "slug", slug, "count", len(roms.Items))

		matchCount := 0
		for idx, localRom := range localRoms {
			hashMatchIdx := slices.IndexFunc(roms.Items, func(rom romm.Rom) bool {
				return rom.Sha1Hash == localRom.SHA1
			})
			if hashMatchIdx == -1 {
				continue
			}

			matchCount++
			remoteRom := roms.Items[hashMatchIdx]
			scanLocal[slug][idx].RomID = remoteRom.ID
			scanLocal[slug][idx].RomName = remoteRom.Name

			if saves, ok := savesByRomID[remoteRom.ID]; ok {
				if len(saves) > 0 {
					scanLocal[slug][idx].RemoteSaves = saves
					logger.Debug("FindSaveSyncs: Found remote saves for ROM", "romName", remoteRom.Name, "saveCount", len(saves))
				}
			}
		}
		logger.Debug("FindSaveSyncs: Finished matching ROMs", "slug", slug, "matchedCount", matchCount)
	}

	var syncs []saveSync

	for slug, roms := range scanLocal {
		for _, r := range roms {
			action := r.syncAction()
			switch action {
			case Upload, Download:
				base := strings.ReplaceAll(r.FileName, filepath.Ext(r.FileName), "")
				lastRemoteSave := r.lastRemoteSave()
				syncs = append(syncs, saveSync{
					RomID:    r.RomID,
					Slug:     r.Slug,
					GameBase: base,
					Local:    r.SaveFile,
					Remote:   lastRemoteSave,
					Action:   action,
				})
				logger.Debug("FindSaveSyncs: Added sync action",
					"slug", slug,
					"localFilename", r.FileName,
					"romName", r.RomName,
					"gameBase", base,
					"action", action)
			}
		}
	}

	logger.Debug("FindSaveSyncs: Completed", "totalSyncs", len(syncs))
	return syncs, nil
}
