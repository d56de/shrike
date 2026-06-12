package config

import "time"

// DefaultConfig returns the built-in config used when no config file exists
// or when a field is not set.
func DefaultConfig() Config {
	return Config{
		General: GeneralConfig{
			DefaultMode: "doctor",
		},
		Runaway: RunawayConfig{
			CPUThreshold: 50.0,
			MinAge:       Duration(1 * time.Hour),
			Ignore:       []string{"WindowServer", "coreaudiod", "mds", "mdworker"},
		},
		Zombie: ZombieConfig{
			MinAge: Duration(5 * time.Minute),
			// Long-lived helpers whose parent is a real GUI app — reaping
			// them via parent-kill would terminate the user's running
			// session. Add new bad neighbours here as they show up.
			Ignore: []string{"AdpSDKUtil", "AdpFusionMan", "AdSSO"},
		},
		Herd: HerdConfig{
			MinSize:           5,
			TotalCPUThreshold: 30.0,
			KnownBadActors:    []string{},
			Ignore:            []string{},
		},
		Memleak: MemleakConfig{
			RSSThresholdMB: 1024,
			MinAge:         Duration(5 * time.Minute),
			Ignore:         []string{},
		},
		History: HistoryConfig{
			Enabled:      true,
			MaxSizeMB:    10,
			MaxRotations: 3,
		},
		UI: UIConfig{
			SeverityHighColor:   "#ff5555",
			SeverityMediumColor: "#ffa500",
			SeverityLowColor:    "#ffd700",
			// 0 = auto-refresh off by default. User opts in via config or
			// the [a] hotkey in the TUI.
			AutoRefreshInterval: 0,
		},
	}
}
