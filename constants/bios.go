package constants

// BIOSFile represents a single BIOS/firmware file requirement
type BIOSFile struct {
	FileName     string // e.g., "gba_bios.bin"
	RelativePath string // e.g., "gba_bios.bin" or "psx/scph5500.bin"
	MD5Hash      string // e.g., "a860e8c0b6d573d191e4ec7db1b1e4f6" (optional, empty string if unknown)
	Optional     bool   // true if BIOS file is optional for the emulator to function
}

// CoreBIOS represents all BIOS requirements for a Libretro core
type CoreBIOS struct {
	CoreName    string     // e.g., "mgba_libretro"
	DisplayName string     // e.g., "Nintendo - Game Boy Advance (mGBA)"
	Files       []BIOSFile // List of BIOS files for this core
}

// CoreBIOSSubdirectories maps Libretro core names (without _libretro suffix)
// to their required BIOS subdirectory within the system BIOS directory.
// Cores not in this map use the root BIOS directory.
var CoreBIOSSubdirectories = map[string]string{
	"bk":                       "bk",
	"bluemsx":                  "", // Uses multiple: "Databases" and "Machines/Shared Roms"
	"dolphin":                  "dolphin-emu/Sys",
	"ep128emu_core":            "ep128emu/roms",
	"fbneo":                    "fbneo",
	"fbneo_cps12":              "fbneo",
	"fbneo_neogeo":             "fbneo",
	"flycast":                  "dc",
	"flycast_gles2":            "dc",
	"fuse":                     "fuse",
	"galaksija":                "galaksija",
	"higan_sfc":                "", // Uses SGB1.sfc and SGB2.sfc
	"higan_sfc_balanced":       "", // Uses SGB1.sfc and SGB2.sfc
	"kronos":                   "kronos",
	"mkxp-z":                   "mkxp-z/RTP",
	"mupen64plus_next":         "Mupen64plus",
	"mupen64plus_next_develop": "Mupen64plus",
	"mupen64plus_next_gles2":   "Mupen64plus",
	"mupen64plus_next_gles3":   "Mupen64plus",
	"neocd":                    "neocd",
	"np2kai":                   "np2kai",
	"pcsx2":                    "pcsx2",
	"ppsspp":                   "PPSSPP",
	"px68k":                    "keropi",
	"quasi88":                  "quasi88",
	"retrodream":               "dc",
	"same_cdi":                 "same_cdi/bios",
	"scummvm":                  "scummvm", // Uses scummvm/theme and scummvm/extra
	"vice_x128":                "vice",
	"vice_x64":                 "vice",
	"vice_x64dtv":              "vice",
	"vice_x64sc":               "vice",
	"vice_xscpu64":             "vice",
	"x1":                       "xmil",
}
