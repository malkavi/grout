package romm

const (
	endpointLogin = "/api/login"

	endpointPlatforms    = "/api/platforms"
	endpointPlatformByID = "/api/platforms/%d"

	endpointRoms         = "/api/roms"
	endpointRomByID      = "/api/roms/%d"
	endpointRomsDownload = "/api/roms/download"
	endpointRomsByHash   = "/api/roms/by-hash"

	endpointCollections        = "/api/collections"
	endpointCollectionByID     = "/api/collections/%d"
	endpointSmartCollections   = "/api/collections/smart"
	endpointVirtualCollections = "/api/collections/virtual"

	endpointFirmware = "/api/firmware"

	endpointSaves = "/api/saves"
)
