package metadata

import (
	"fmt"
	"sort"
)

// Entry is an immutable Pokémon metadata value.
type Entry struct {
	id       string
	name     string
	imageURL string
}

func (entry Entry) ID() string       { return entry.id }
func (entry Entry) Name() string     { return entry.name }
func (entry Entry) ImageURL() string { return entry.imageURL }

func (entry Entry) String() string   { return "metadata.Entry{redacted}" }
func (entry Entry) GoString() string { return entry.String() }

// Catalog is an immutable collection keyed by exact upstream Pokémon/form ID.
type Catalog struct {
	entries map[string]Entry
}

func (catalog Catalog) Len() int { return len(catalog.entries) }

func (catalog Catalog) Lookup(id string) (Entry, bool) {
	entry, ok := catalog.entries[id]
	return entry, ok
}

func (catalog Catalog) List() []Entry {
	entries := make([]Entry, 0, len(catalog.entries))
	for _, entry := range catalog.entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].id < entries[j].id })
	return entries
}

func (catalog Catalog) String() string {
	return fmt.Sprintf("metadata.Catalog{entries:%d}", len(catalog.entries))
}
func (catalog Catalog) GoString() string { return catalog.String() }
