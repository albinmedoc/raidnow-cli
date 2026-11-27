package metadata

import "sort"

// Entry is a Pokémon metadata value.
type Entry struct {
	ID               string
	NameEN           string
	ImageURL         string
	ShinyAvailable   bool
	MaxCP20          *int
	MaxCP25          *int
	RaidRank         string
	CurrentRaidRank  string
	WeatherBoosts    []string
	HasWeatherBoosts bool
}

// Catalog is a collection keyed by exact upstream Pokémon/form ID.
type Catalog struct {
	entries map[string]Entry
}

func (catalog Catalog) Len() int { return len(catalog.entries) }

func (catalog Catalog) Lookup(id string) (Entry, bool) {
	entry, ok := catalog.entries[id]
	return cloneEntry(entry), ok
}

func (catalog Catalog) List() []Entry {
	entries := make([]Entry, 0, len(catalog.entries))
	for _, entry := range catalog.entries {
		entries = append(entries, cloneEntry(entry))
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries
}

func cloneEntry(entry Entry) Entry {
	clone := entry
	if entry.MaxCP20 != nil {
		value := *entry.MaxCP20
		clone.MaxCP20 = &value
	}
	if entry.MaxCP25 != nil {
		value := *entry.MaxCP25
		clone.MaxCP25 = &value
	}
	clone.WeatherBoosts = append([]string(nil), entry.WeatherBoosts...)
	return clone
}
