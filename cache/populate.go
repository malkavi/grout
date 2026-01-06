package cache

import (
	"grout/romm"
	"sync"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
	"go.uber.org/atomic"
)

// Configuration constants for API pagination
const (
	// DefaultRomPageSize is the number of ROMs to fetch per API call
	// Kept small for better progress feedback and to avoid timeouts
	// Increase this when the bulk API becomes more performant
	DefaultRomPageSize = 200

	// MaxConcurrentPlatformFetches limits parallel platform API calls
	MaxConcurrentPlatformFetches = 5
)

// populateCache populates the entire cache with platform and collection data
func (cm *CacheManager) populateCache(platforms []romm.Platform, progress *atomic.Float64) error {
	logger := gaba.GetLogger()

	if len(platforms) == 0 {
		if progress != nil {
			progress.Store(1.0)
		}
		return nil
	}

	// Save platforms first
	if err := cm.SavePlatforms(platforms); err != nil {
		return err
	}

	// Calculate total expected games for granular progress tracking
	// Use 90% for games, reserve 10% for collections
	totalExpectedGames := int64(0)
	for _, p := range platforms {
		totalExpectedGames += int64(p.ROMCount)
	}
	if totalExpectedGames == 0 {
		totalExpectedGames = int64(len(platforms)) // Fallback to platform count
	}

	// Track progress based on games fetched
	gamesFetched := &atomic.Int64{}
	updateProgress := func(count int) {
		if progress != nil {
			fetched := gamesFetched.Add(int64(count))
			// Cap at 90% for games phase, reserve 10% for collections
			pct := float64(fetched) / float64(totalExpectedGames) * 0.9
			if pct > 0.9 {
				pct = 0.9
			}
			progress.Store(pct)
		}
	}

	// Use bounded concurrency for platform fetches
	sem := make(chan struct{}, MaxConcurrentPlatformFetches)
	var wg sync.WaitGroup
	var firstErr error
	var errMu sync.Mutex

	for _, platform := range platforms {
		wg.Add(1)
		go func(p romm.Platform) {
			defer wg.Done()
			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			if err := cm.fetchAndCachePlatformGamesWithProgress(p, updateProgress); err != nil {
				logger.Error("Failed to cache platform", "platform", p.Name, "error", err)
				errMu.Lock()
				if firstErr == nil {
					firstErr = err
				}
				errMu.Unlock()
			}
		}(platform)
	}

	// Also fetch BIOS availability in parallel
	wg.Add(1)
	go func() {
		defer wg.Done()
		cm.fetchBIOSAvailability(platforms)
	}()

	wg.Wait()

	// Record games refresh time
	if firstErr == nil {
		cm.RecordRefreshTime(MetaKeyGamesRefreshedAt)
	}

	// Fetch collections after platforms (they may depend on game data)
	// This uses the remaining 10% of progress (90% -> 100%)
	cm.fetchAndCacheCollectionsWithProgress(progress)

	// Record collections refresh time
	cm.RecordRefreshTime(MetaKeyCollectionsRefreshedAt)

	if progress != nil {
		progress.Store(1.0)
	}

	logger.Info("Cache population completed", "platforms", len(platforms), "games", gamesFetched.Load())
	return firstErr
}

// fetchAndCachePlatformGames fetches all games for a platform using paginated API calls
func (cm *CacheManager) fetchAndCachePlatformGames(platform romm.Platform) error {
	return cm.fetchAndCachePlatformGamesWithProgress(platform, nil)
}

// fetchAndCachePlatformGamesWithProgress fetches all games with progress callback per batch
func (cm *CacheManager) fetchAndCachePlatformGamesWithProgress(platform romm.Platform, onProgress func(count int)) error {
	logger := gaba.GetLogger()

	client := romm.NewClientFromHost(cm.host, cm.config.GetApiTimeout())

	var allGames []romm.Rom
	offset := 0
	expectedTotal := 0
	requestCount := 0

	for {
		opt := romm.GetRomsQuery{
			PlatformID: platform.ID,
			Offset:     offset,
			Limit:      DefaultRomPageSize,
		}

		res, err := client.GetRoms(opt)
		if err != nil {
			logger.Error("Failed to fetch games",
				"platform", platform.Name,
				"offset", offset,
				"error", err)
			return err
		}
		requestCount++

		// Capture expected total from first request
		if offset == 0 {
			expectedTotal = res.Total
		}

		logger.Debug("Fetched games batch",
			"platform", platform.Name,
			"offset", offset,
			"received", len(res.Items),
			"total", res.Total,
			"expectedTotal", expectedTotal,
			"accumulated", len(allGames)+len(res.Items))

		allGames = append(allGames, res.Items...)

		// Report progress after each batch
		if onProgress != nil && len(res.Items) > 0 {
			onProgress(len(res.Items))
		}

		// Check if we've fetched all games:
		// 1. We have at least as many as expected
		// 2. OR we received an empty batch (no more items)
		// 3. OR we received fewer items than requested (last batch)
		if len(allGames) >= expectedTotal || len(res.Items) == 0 || len(res.Items) < DefaultRomPageSize {
			break
		}

		offset += len(res.Items)

		// Safety limit to prevent infinite loops
		if requestCount > 1000 {
			logger.Warn("Hit request limit while fetching games",
				"platform", platform.Name,
				"accumulated", len(allGames))
			break
		}
	}

	logger.Info("Fetched all games for platform",
		"platform", platform.Name,
		"expected", expectedTotal,
		"actual", len(allGames))

	// Save to cache (this acquires its own lock)
	if err := cm.SavePlatformGames(platform.ID, allGames); err != nil {
		return err
	}

	logger.Debug("Fetched and cached platform games",
		"platform", platform.Name,
		"count", len(allGames),
		"requests", requestCount)

	return nil
}

// fetchAndCacheCollections fetches and caches all collection types
func (cm *CacheManager) fetchAndCacheCollections() {
	cm.fetchAndCacheCollectionsWithProgress(nil)
}

// fetchAndCacheCollectionsWithProgress fetches and caches all collection types with progress tracking
func (cm *CacheManager) fetchAndCacheCollectionsWithProgress(progress *atomic.Float64) {
	logger := gaba.GetLogger()

	client := romm.NewClientFromHost(cm.host, cm.config.GetApiTimeout())

	var allCollections []romm.Collection
	var mu sync.Mutex
	var wg sync.WaitGroup

	// Fetch regular collections
	// Always fetch all collection types regardless of display settings
	// so data is ready when user enables them
	wg.Add(1)
	go func() {
		defer wg.Done()
		collections, err := client.GetCollections()
		if err != nil {
			logger.Error("Failed to fetch regular collections", "error", err)
			return
		}
		mu.Lock()
		allCollections = append(allCollections, collections...)
		mu.Unlock()
	}()

	// Fetch smart collections
	wg.Add(1)
	go func() {
		defer wg.Done()
		collections, err := client.GetSmartCollections()
		if err != nil {
			logger.Error("Failed to fetch smart collections", "error", err)
			return
		}
		for i := range collections {
			collections[i].IsSmart = true
		}
		mu.Lock()
		allCollections = append(allCollections, collections...)
		mu.Unlock()
	}()

	// Fetch virtual collections
	wg.Add(1)
	go func() {
		defer wg.Done()
		virtualCollections, err := client.GetVirtualCollections()
		if err != nil {
			logger.Error("Failed to fetch virtual collections", "error", err)
			return
		}
		mu.Lock()
		for _, vc := range virtualCollections {
			allCollections = append(allCollections, vc.ToCollection())
		}
		mu.Unlock()
	}()

	wg.Wait()

	// Update progress to 92% after fetching collection metadata
	if progress != nil {
		progress.Store(0.92)
	}

	if len(allCollections) == 0 {
		return
	}

	// Save collection metadata
	if err := cm.SaveCollections(allCollections); err != nil {
		logger.Error("Failed to save collections", "error", err)
	}

	if progress != nil {
		progress.Store(0.94)
	}

	// Save game-collection mappings using ROMs already in collection response
	// No need to fetch games again - they're already included!
	if err := cm.SaveAllCollectionMappings(allCollections); err != nil {
		logger.Error("Failed to save collection mappings", "error", err)
	}

	if progress != nil {
		progress.Store(0.98)
	}

	logger.Debug("Cached collections", "count", len(allCollections))
}

// fetchCollectionGames fetches all games for a collection
func (cm *CacheManager) fetchCollectionGames(collection romm.Collection) ([]romm.Rom, error) {
	logger := gaba.GetLogger()
	client := romm.NewClientFromHost(cm.host, cm.config.GetApiTimeout())

	var allGames []romm.Rom
	offset := 0
	expectedTotal := 0
	requestCount := 0

	for {
		opt := romm.GetRomsQuery{
			Offset: offset,
			Limit:  DefaultRomPageSize,
		}

		if collection.IsVirtual {
			opt.VirtualCollectionID = collection.VirtualID
		} else if collection.IsSmart {
			opt.SmartCollectionID = collection.ID
		} else {
			opt.CollectionID = collection.ID
		}

		res, err := client.GetRoms(opt)
		if err != nil {
			logger.Error("Failed to fetch collection games",
				"collection", collection.Name,
				"offset", offset,
				"error", err)
			return nil, err
		}
		requestCount++

		// Capture expected total from first request
		if offset == 0 {
			expectedTotal = res.Total
		}

		logger.Debug("Fetched collection games batch",
			"collection", collection.Name,
			"offset", offset,
			"received", len(res.Items),
			"total", res.Total,
			"expectedTotal", expectedTotal,
			"accumulated", len(allGames)+len(res.Items))

		allGames = append(allGames, res.Items...)

		// Check if we've fetched all games:
		// 1. We have at least as many as expected
		// 2. OR we received an empty batch (no more items)
		// 3. OR we received fewer items than requested (last batch)
		if len(allGames) >= expectedTotal || len(res.Items) == 0 || len(res.Items) < DefaultRomPageSize {
			break
		}

		offset += len(res.Items)

		// Safety limit to prevent infinite loops
		if requestCount > 1000 {
			logger.Warn("Hit request limit while fetching collection games",
				"collection", collection.Name,
				"accumulated", len(allGames))
			break
		}
	}

	logger.Debug("Fetched all games for collection",
		"collection", collection.Name,
		"expected", expectedTotal,
		"actual", len(allGames))

	return allGames, nil
}

// fetchBIOSAvailability fetches BIOS availability for all platforms
func (cm *CacheManager) fetchBIOSAvailability(platforms []romm.Platform) {
	logger := gaba.GetLogger()

	client := romm.NewClientFromHost(cm.host, cm.config.GetApiTimeout())

	var wg sync.WaitGroup
	sem := make(chan struct{}, MaxConcurrentPlatformFetches)

	for _, platform := range platforms {
		wg.Add(1)
		go func(p romm.Platform) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			firmware, err := client.GetFirmware(p.ID)
			if err != nil {
				logger.Debug("Failed to fetch BIOS info", "platform", p.Name, "error", err)
				cm.SetBIOSAvailability(p.ID, false)
				return
			}

			hasBIOS := len(firmware) > 0
			cm.SetBIOSAvailability(p.ID, hasBIOS)
		}(platform)
	}

	wg.Wait()
}

// RefreshPlatformGames fetches and updates games for a single platform
// Useful for refreshing a specific platform without full cache rebuild
func (cm *CacheManager) RefreshPlatformGames(platform romm.Platform) error {
	if cm == nil || !cm.initialized {
		return ErrNotInitialized
	}

	return cm.fetchAndCachePlatformGames(platform)
}

// RefreshPlatformGamesWithProgress fetches and updates games for a single platform with progress tracking
// Progress is reported as 0.0 to 1.0 based on games fetched vs expected total
func (cm *CacheManager) RefreshPlatformGamesWithProgress(platform romm.Platform, progress *atomic.Float64) error {
	if cm == nil || !cm.initialized {
		return ErrNotInitialized
	}

	logger := gaba.GetLogger()
	client := romm.NewClientFromHost(cm.host, cm.config.GetApiTimeout())

	var allGames []romm.Rom
	offset := 0
	expectedTotal := 0
	requestCount := 0

	for {
		opt := romm.GetRomsQuery{
			PlatformID: platform.ID,
			Offset:     offset,
			Limit:      DefaultRomPageSize,
		}

		res, err := client.GetRoms(opt)
		if err != nil {
			logger.Error("Failed to fetch games",
				"platform", platform.Name,
				"offset", offset,
				"error", err)
			return err
		}
		requestCount++

		// Capture expected total from first request
		if offset == 0 {
			expectedTotal = res.Total
		}

		logger.Debug("Fetched games batch",
			"platform", platform.Name,
			"offset", offset,
			"received", len(res.Items),
			"total", res.Total,
			"expectedTotal", expectedTotal,
			"accumulated", len(allGames)+len(res.Items))

		allGames = append(allGames, res.Items...)

		// Update progress after each batch
		if progress != nil && expectedTotal > 0 {
			pct := float64(len(allGames)) / float64(expectedTotal)
			if pct > 1.0 {
				pct = 1.0
			}
			progress.Store(pct)
		}

		// Check if we've fetched all games
		if len(allGames) >= expectedTotal || len(res.Items) == 0 || len(res.Items) < DefaultRomPageSize {
			break
		}

		offset += len(res.Items)

		// Safety limit to prevent infinite loops
		if requestCount > 1000 {
			logger.Warn("Hit request limit while fetching games",
				"platform", platform.Name,
				"accumulated", len(allGames))
			break
		}
	}

	logger.Info("Fetched all games for platform",
		"platform", platform.Name,
		"expected", expectedTotal,
		"actual", len(allGames))

	// Save to cache
	if err := cm.SavePlatformGames(platform.ID, allGames); err != nil {
		return err
	}

	// Ensure progress is at 100% when done
	if progress != nil {
		progress.Store(1.0)
	}

	return nil
}

// RefreshCollectionGames fetches and updates games for a single collection
func (cm *CacheManager) RefreshCollectionGames(collection romm.Collection) error {
	if cm == nil || !cm.initialized {
		return ErrNotInitialized
	}

	games, err := cm.fetchCollectionGames(collection)
	if err != nil {
		return err
	}

	return cm.SaveCollectionGames(collection, games)
}
