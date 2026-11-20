package domain

// QueueState is the fail-closed normalized state of one queue poll.
type QueueState string

const (
	QueueWaiting QueueState = "waiting"
	QueueMatched QueueState = "matched"
)

type QueuePoll struct {
	State  QueueState `json:"state"`
	RaidID string     `json:"raidId,omitempty"`
}

type QueueWaitingEntry struct {
	ID        string `json:"id,omitempty"`
	Rank      string `json:"rank,omitempty"`
	PokemonID string `json:"pokemonId,omitempty"`
}

type QueueCheck struct {
	Present  bool                `json:"present"`
	ID       string              `json:"id,omitempty"`
	Priority *int                `json:"priority,omitempty"`
	Waiting  []QueueWaitingEntry `json:"waiting"`
}

type RaidPlayer struct {
	ID          string `json:"id,omitempty"`
	TrainerName string `json:"trainerName,omitempty"`
	Level       int    `json:"level"`
	Country     string `json:"country,omitempty"`
	Rating      *int   `json:"rating"`
	Ready       bool   `json:"ready"`
	IsMe        bool   `json:"isMe"`
}

type RaidPlayers struct {
	Players     []RaidPlayer `json:"players"`
	Count       int          `json:"count"`
	ReadyCount  int          `json:"readyCount"`
	RaidDeleted bool         `json:"raidDeleted"`
}

// Raid is a bounded normalized lobby or matched-raid view.
type Raid struct {
	ID                string `json:"id"`
	HostTrainerCode   string `json:"-"`
	Rank              string `json:"rank"`
	PokemonID         string `json:"pokemonId"`
	HostName          string `json:"hostName,omitempty"`
	HostLevel         int    `json:"hostLevel"`
	Country           string `json:"country,omitempty"`
	Weather           int    `json:"weather"`
	MaxUsers          int    `json:"maxUsers"`
	Count             int    `json:"count"`
	ReadyCount        int    `json:"readyCount"`
	JoinState         *int   `json:"joinState,omitempty"`
	Deleted           bool   `json:"deleted"`
	IsMe              bool   `json:"isMe"`
	MinimumGuestLevel int    `json:"minimumGuestLevel"`
}

// RaidBoss is one exact species/form selection exposed by Auto Join.
type RaidBoss struct {
	Rank         string `json:"rank"`
	PokemonID    string `json:"pokemonId"`
	WaitingCount int    `json:"waitingCount"`
}
