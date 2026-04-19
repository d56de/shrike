package history

// WriterConfig captures the history knobs needed to write + rotate a run
// from places that don't have direct access to the config package.
type WriterConfig struct {
	Enabled      bool
	MaxSizeMB    int
	MaxRotations int
}
