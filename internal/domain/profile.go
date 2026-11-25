package domain

// ProfileHistoryRecord is a safe normalized profile history row.
type ProfileHistoryRecord struct {
	RaidID      string        `json:"raidId"`
	DisplayTime string        `json:"displayTime"`
	PokemonID   string        `json:"pokemonId"`
	TrainerName string        `json:"trainerName,omitempty"`
	Level       int           `json:"level"`
	Country     string        `json:"country,omitempty"`
	Rating      *int          `json:"rating"`
	Guest       *ProfileGuest `json:"guest,omitempty"`
}

// ProfileGuest is present only for joined history records.
type ProfileGuest struct {
	TrainerName string `json:"trainerName,omitempty"`
	Level       int    `json:"level"`
	Country     string `json:"country,omitempty"`
	Rating      *int   `json:"rating"`
}

// BlockRecord is one safe normalized block-list row.
type BlockRecord struct {
	BlockID     string `json:"blockId"`
	TrainerName string `json:"trainerName"`
}
