package sync

import (
	"os"
	"strings"
	"time"
)

// DirectorySavePlatforms maps RomM fs_slug to platform slugs where saves are
// stored as directories (e.g., PPSSPP uses Game ID folders containing save files)
// rather than individual files alongside other saves.
//
// These require special handling during sync: the entire directory must be zipped
// for upload and unzipped on download. Matching saves to ROMs requires Game ID
// resolution rather than filename matching.
var DirectorySavePlatforms = map[string]bool{
	"psp": true,
}

// IsDirectorySavePlatform returns true if the platform stores saves as directories.
func IsDirectorySavePlatform(fsSlug string) bool {
	return DirectorySavePlatforms[fsSlug]
}

// extractGameIDPrefix finds the longest common prefix among directory names,
// trimming any trailing underscore or separator. For example:
//
//	["UCES00995_DATA00", "UCES00995_DATA01"] → "UCES00995"
//	["ULUS10391"] → "ULUS10391"
func extractGameIDPrefix(dirNames []string) string {
	if len(dirNames) == 0 {
		return ""
	}
	if len(dirNames) == 1 {
		return dirNames[0]
	}

	prefix := dirNames[0]
	for _, name := range dirNames[1:] {
		i := 0
		for i < len(prefix) && i < len(name) && prefix[i] == name[i] {
			i++
		}
		prefix = prefix[:i]
	}

	// Trim trailing underscore/separator, then trim back to the last
	// clean boundary (underscore or hyphen) if we stopped mid-token.
	prefix = strings.TrimRight(prefix, "_- ")
	if prefix == "" {
		return dirNames[0]
	}

	// If the prefix ends mid-token (e.g., "UCES00995_DATA0" from DATA00/DATA01),
	// trim back to the last separator to get a clean Game ID.
	if len(prefix) < len(dirNames[0]) {
		if idx := strings.LastIndexAny(prefix, "_-"); idx > 0 {
			prefix = prefix[:idx]
		}
	}

	return prefix
}

// latestMtime returns the most recent modification time across multiple paths.
// Falls back to the zero time if no path can be statted.
func latestMtime(paths []string) time.Time {
	var latest time.Time
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			continue
		}
		if mt := info.ModTime(); mt.After(latest) {
			latest = mt
		}
	}
	return latest
}
