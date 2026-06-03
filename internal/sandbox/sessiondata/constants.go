package sessiondata

// File and directory permissions used throughout the package. Imported
// transcripts are private user data; ADR-066 §"Privacy and safety"
// requires 0o600 files and 0o700 directories on both the staging tree
// and the merged destination.
const (
	filePerm = 0o600
	dirPerm  = 0o700
)

// nanosPerSec is the float-to-time.Duration conversion factor for the
// fractional-seconds portion of GNU find's %T@ output.
const nanosPerSec = 1e9

// tsvFieldCount is the number of TAB-separated fields per inventory
// record: <relative-path>\t<size-bytes>\t<mtime-epoch-seconds>.
const tsvFieldCount = 3
