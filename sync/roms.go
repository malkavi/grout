package sync

import (
	"grout/cache"
	"grout/cfw"
	"path/filepath"
	"strings"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

// getGameName removes file extensions from a filename.
// Always removes the first extension, then checks if the next extension is known.
// Handles multiple extensions like "Patapon.iso.cso" -> "Patapon"
func getGameName(fileName string) string {
	// Known emulator file extensions
	knownExtensions := map[string]bool{
		".iso": true, ".cso": true, ".bin": true, ".chd": true,
		".cue": true, ".zip": true, ".7z": true, ".rar": true,
		".wad": true, ".gdi": true, ".nrg": true, ".img": true,
	}

	nameNoExt := fileName

	// Always remove the first extension
	ext := filepath.Ext(nameNoExt)
	if ext != "" {
		nameNoExt = strings.TrimSuffix(nameNoExt, ext)
	}

	// Check if there's a second extension and if it's known, remove it
	ext = filepath.Ext(nameNoExt)
	if ext != "" && knownExtensions[strings.ToLower(ext)] {
		nameNoExt = strings.TrimSuffix(nameNoExt, ext)
	}

	return nameNoExt
}

// ResolveLocalRoms scans local ROM files and resolves them against the cache
// to get ROM IDs. Returns a map of ROM ID to LocalRomFile for matched ROMs.
func ResolveLocalRoms(scan cfw.LocalRomScan) map[int]cfw.LocalRomFile {
	logger := gaba.GetLogger()
	cm := cache.GetCacheManager()
	if cm == nil {
		logger.Error("Cache manager not available for ROM resolution")
		return nil
	}

	resolved := make(map[int]cfw.LocalRomFile)
	for fsSlug, files := range scan {
		for _, f := range files {
			nameNoExt := getGameName(f.FileName)
			rom, err := cm.GetRomByFSLookup(fsSlug, nameNoExt)
			if err != nil {
				continue
			}
			f.RomID = rom.ID
			f.RomName = rom.Name
			resolved[rom.ID] = f
		}
	}

	logger.Debug("Resolved local ROMs against cache", "matched", len(resolved))
	return resolved
}
