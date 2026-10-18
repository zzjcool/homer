package herdr

import "github.com/zzjcool/homer/internal/adapter"

type ScanError = adapter.ScanError
type ScanOutcome = adapter.ScanOutcome

// ScanAdapter reuses the common scanner; herdr has no adapter-specific walk.
var ScanAdapter = adapter.ScanAdapter
