package domain

import (
	"fmt"
	"io"
	"strconv"
)

func writeSafeFormat(state fmt.State, typeName, details string) {
	_, _ = io.WriteString(state, typeName)
	_, _ = io.WriteString(state, "{")
	_, _ = io.WriteString(state, details)
	_, _ = io.WriteString(state, "}")
}

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

func (value QueuePoll) Format(state fmt.State, _ rune) {
	writeSafeFormat(state, "QueuePoll", "state:"+string(value.State)+", raidId:[redacted]")
}

type QueueWaitingEntry struct {
	ID        string `json:"id,omitempty"`
	Rank      string `json:"rank,omitempty"`
	PokemonID string `json:"pokemonId,omitempty"`
}

func (value QueueWaitingEntry) Format(state fmt.State, _ rune) {
	writeSafeFormat(state, "QueueWaitingEntry", "id:[redacted], rank:[redacted], pokemonId:[redacted]")
}

type QueueCheck struct {
	Present  bool                `json:"present"`
	ID       string              `json:"id,omitempty"`
	Priority *int                `json:"priority,omitempty"`
	Waiting  []QueueWaitingEntry `json:"waiting"`
}

func (value QueueCheck) Format(state fmt.State, _ rune) {
	writeSafeFormat(state, "QueueCheck", "present:"+strconv.FormatBool(value.Present)+", id:[redacted], waiting:"+strconv.Itoa(len(value.Waiting)))
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

func (value RaidPlayer) Format(state fmt.State, _ rune) {
	writeSafeFormat(state, "RaidPlayer", "id:[redacted], trainerName:[redacted], level:"+strconv.Itoa(value.Level))
}

type RaidPlayers struct {
	Players     []RaidPlayer `json:"players"`
	Count       int          `json:"count"`
	ReadyCount  int          `json:"readyCount"`
	RaidDeleted bool         `json:"raidDeleted"`
}

func (value RaidPlayers) Format(state fmt.State, _ rune) {
	writeSafeFormat(state, "RaidPlayers", "players:"+strconv.Itoa(len(value.Players))+", count:"+strconv.Itoa(value.Count)+", readyCount:"+strconv.Itoa(value.ReadyCount))
}

// Raid is a bounded normalized lobby or matched-raid view.
type Raid struct {
	ID         string `json:"id"`
	Rank       string `json:"rank"`
	PokemonID  string `json:"pokemonId"`
	HostName   string `json:"hostName,omitempty"`
	HostLevel  int    `json:"hostLevel"`
	Country    string `json:"country,omitempty"`
	Weather    int    `json:"weather"`
	MaxUsers   int    `json:"maxUsers"`
	Count      int    `json:"count"`
	ReadyCount int    `json:"readyCount"`
	JoinState  *int   `json:"joinState,omitempty"`
}

func (value Raid) Format(state fmt.State, _ rune) {
	writeSafeFormat(state, "Raid", "id:[redacted], pokemonId:[redacted], hostName:[redacted], count:"+strconv.Itoa(value.Count))
}

// RaidBoss is one exact species/form selection exposed by Auto Join.
type RaidBoss struct {
	Rank         string `json:"rank"`
	PokemonID    string `json:"pokemonId"`
	WaitingCount int    `json:"waitingCount"`
}

func (value RaidBoss) Format(state fmt.State, _ rune) {
	writeSafeFormat(state, "RaidBoss", "rank:[redacted], pokemonId:[redacted], waitingCount:"+strconv.Itoa(value.WaitingCount))
}
