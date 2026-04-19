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
		},
		Herd: HerdConfig{
			MinSize:           5,
			TotalCPUThreshold: 30.0,
			KnownBadActors:    []string{},
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
		},
	}
}
