package utils

import (
	"grout/romm"
)

func ShowCollections(config *Config, host romm.Host) bool {
	if !config.ShowCollections && !config.ShowVirtualCollections {
		return false
	}

	rc := GetRommClient(host)

	if config.ShowCollections {
		col, err := rc.GetCollections()
		if err == nil && len(col) > 0 {
			return true
		}

		smartCol, err := rc.GetSmartCollections()
		if err == nil && len(smartCol) > 0 {
			return true
		}
	}

	if config.ShowVirtualCollections {
		virtualCol, err := rc.GetVirtualCollections()
		if err == nil && len(virtualCol) > 0 {
			return true
		}
	}

	return false
}
