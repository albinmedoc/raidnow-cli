package metadata

import "sort"

// Entry is a Pokémon metadata value.
type Entry struct {
	ID       string
	NameEN   string
	ImageURL string
}

// Catalog is a collection keyed by exact upstream Pokémon/form ID.
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
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}
