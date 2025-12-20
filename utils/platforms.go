package utils

import (
	"fmt"

	"grout/romm"
)

func GetMappedPlatforms(host romm.Host, mappings map[string]DirectoryMapping) ([]romm.Platform, error) {
	c := GetRommClient(host)

	rommPlatforms, err := c.GetPlatforms()
	if err != nil {
		return nil, fmt.Errorf("failed to get platforms from RomM: %w", err)
	}

	var platforms []romm.Platform

	for _, platform := range rommPlatforms {
		_, exists := mappings[platform.Slug]
		if exists {
			platforms = append(platforms, romm.Platform{
				Name: platform.Name,
				ID:   platform.ID,
				Slug: platform.Slug,
			})
		}
	}

	return platforms, nil
}
