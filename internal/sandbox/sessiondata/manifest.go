package sessiondata

import (
	"context"
	"fmt"
	"sort"
)

// Manifest is the per-VM inventory of allowlisted files grouped by
// agent kind. Items[kind] is sorted by relative path for deterministic
// output.
type Manifest struct {
	// Items maps agent kind → file entries in stable order.
	Items map[AgentKind][]FileEntry
	// VM is the slicer VM name the manifest was taken from.
	VM string
}

// Count returns the total file count across every kind in the manifest.
func (m Manifest) Count() int {
	n := 0
	for _, entries := range m.Items {
		n += len(entries)
	}
	return n
}

// NonEmptyKinds returns kinds that have at least one file in the
// manifest, in AllKinds order.
func (m Manifest) NonEmptyKinds() []AgentKind {
	out := make([]AgentKind, 0, len(m.Items))
	for _, kind := range AllKinds() {
		if len(m.Items[kind]) > 0 {
			out = append(out, kind)
		}
	}
	return out
}

// FetchManifest enumerates allowlisted source files in the VM for each
// requested kind. The Slicer's Inventory method is called once with
// the union of all roots; the returned entries are then bucketed back
// by kind based on which root each entry's path falls under.
//
// Empty kinds (no files in any of their roots) appear in Items with an
// empty slice — callers that iterate via NonEmptyKinds skip them.
func FetchManifest(ctx context.Context, s Slicer, vm string, kinds []AgentKind) (Manifest, error) {
	if vm == "" {
		return Manifest{}, fmt.Errorf("%w: empty vm name", ErrInventoryFailed)
	}
	if len(kinds) == 0 {
		return Manifest{VM: vm, Items: map[AgentKind][]FileEntry{}}, nil
	}

	rootList := unionRoots(kinds)
	entries, err := s.Inventory(ctx, vm, rootList)
	if err != nil {
		return Manifest{}, fmt.Errorf("fetch manifest: %w", err)
	}

	manifest := buildManifest(vm, kinds, entries)
	sortManifest(manifest)
	return manifest, nil
}

// unionRoots returns the union of every kind's source roots in the
// order they are first encountered. Duplicates (across overlapping
// kinds) appear once.
func unionRoots(kinds []AgentKind) []string {
	const initialCap = 4
	rootList := make([]string, 0, initialCap)
	seen := make(map[string]struct{})
	for _, kind := range kinds {
		for _, root := range SourceRoots(kind) {
			if _, dup := seen[root]; dup {
				continue
			}
			seen[root] = struct{}{}
			rootList = append(rootList, root)
		}
	}
	return rootList
}

// buildManifest buckets entries back by kind based on which kind's
// source roots cover the entry's path. The first kind in AllKinds
// order wins when multiple kinds share a root prefix.
func buildManifest(vm string, kinds []AgentKind, entries []FileEntry) Manifest {
	manifest := Manifest{VM: vm, Items: make(map[AgentKind][]FileEntry, len(kinds))}
	for _, kind := range kinds {
		manifest.Items[kind] = nil
	}
	for _, entry := range entries {
		kind, ok := matchRoot(entry.Path, kinds)
		if !ok {
			continue
		}
		manifest.Items[kind] = append(manifest.Items[kind], entry)
	}
	return manifest
}

func sortManifest(m Manifest) {
	for kind := range m.Items {
		sort.Slice(m.Items[kind], func(i, j int) bool {
			return m.Items[kind][i].Path < m.Items[kind][j].Path
		})
	}
}

// matchRoot returns the kind whose source roots include a prefix of
// path. If multiple kinds match, the first kind in kinds (= AllKinds)
// order wins.
func matchRoot(path string, kinds []AgentKind) (AgentKind, bool) {
	for _, kind := range kinds {
		for _, root := range SourceRoots(kind) {
			if hasRootPrefix(path, root) {
				return kind, true
			}
		}
	}
	return "", false
}

func hasRootPrefix(path, root string) bool {
	if path == root {
		return true
	}
	if len(path) > len(root) && path[:len(root)] == root && path[len(root)] == '/' {
		return true
	}
	return false
}
