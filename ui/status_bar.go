package ui

import (
	gaba "github.com/BrandonKowalski/gabagool/v2/pkg/gabagool"
)

var defaultStatusBar = gaba.StatusBarOptions{
	Enabled:    true,
	ShowTime:   true,
	TimeFormat: gaba.TimeFormat24Hour,
	Icons:      []gaba.StatusBarIcon{},
}

func StatusBar() gaba.StatusBarOptions {
	return defaultStatusBar
}

func AddStatusBarIcon(icon gaba.StatusBarIcon) {
	defaultStatusBar.Icons = append([]gaba.StatusBarIcon{icon}, defaultStatusBar.Icons...)
}
