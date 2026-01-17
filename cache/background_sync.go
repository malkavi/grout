package cache

import (
	"grout/romm"
	"sync/atomic"

	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

const (
	iconSynced  = "\U000F0AA9"
	iconSyncing = "\U000F0CFF"
	iconAlert   = "\U000F163A"
)

type BackgroundSync struct {
	platforms []romm.Platform
	icon      *gaba.DynamicStatusBarIcon
	running   atomic.Bool
	done      chan struct{}
}

func NewBackgroundSync(platforms []romm.Platform) *BackgroundSync {
	return &BackgroundSync{
		platforms: platforms,
		icon:      gaba.NewDynamicStatusBarIcon(iconSyncing),
		done:      make(chan struct{}),
	}
}

func (b *BackgroundSync) Icon() gaba.StatusBarIcon {
	return gaba.StatusBarIcon{
		Dynamic: b.icon,
	}
}

func (b *BackgroundSync) Start() {
	b.running.Store(true)
	b.done = make(chan struct{})
	go b.run()
}

func (b *BackgroundSync) IsRunning() bool {
	return b.running.Load()
}

func (b *BackgroundSync) Wait() {
	<-b.done
}

func (b *BackgroundSync) run() {
	logger := gaba.GetLogger()
	defer func() {
		if r := recover(); r != nil {
			logger.Error("BackgroundSync: Panic recovered", "panic", r)
			b.icon.SetText(iconAlert)
		}
		b.running.Store(false)
		close(b.done)
	}()

	b.icon.SetText(iconSyncing)
	logger.Debug("BackgroundSync: Starting cache update")

	cm := GetCacheManager()
	if cm == nil {
		logger.Error("BackgroundSync: Cache manager not initialized")
		b.icon.SetText(iconAlert)
		return
	}

	stats, err := cm.PopulateFullCacheWithProgress(b.platforms, nil)
	if err != nil {
		logger.Error("BackgroundSync: Cache update failed", "error", err)
		b.icon.SetText(iconAlert)
		return
	}

	b.icon.SetText(iconSynced)
	logger.Info("Background cache sync completed",
		"platforms", stats.Platforms,
		"games_updated", stats.GamesUpdated,
		"collections", stats.Collectionssynced)
}
