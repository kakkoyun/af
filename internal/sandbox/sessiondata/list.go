package sessiondata

import "context"

// List is a read-only counterpart of Pull. It returns the same
// Manifest that Pull would produce without copying or merging.
//
// Equivalent to `Pull` with DryRun=true, but exposed separately so
// `af session-data list` can call it without constructing a full
// PullOptions value.
func List(ctx context.Context, s Slicer, vm string, kinds []AgentKind) (Manifest, error) {
	if len(kinds) == 0 {
		kinds = AllKinds()
	}
	return FetchManifest(ctx, s, vm, kinds)
}
