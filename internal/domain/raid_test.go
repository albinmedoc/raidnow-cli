package domain

import (
	"fmt"
	"strings"
	"testing"
)

func TestRaidModelsDoNotLeakIdentifiersOrTrainerNamesThroughFormatting(t *testing.T) {
	priority := 2
	rating := 5
	values := []any{
		RaidBoss{Rank: "secret-rank", PokemonID: "secret-pokemon"},
		Raid{ID: "secret-raid", PokemonID: "secret-pokemon", HostName: "secret-host"},
		QueuePoll{State: QueueMatched, RaidID: "secret-raid"},
		QueueWaitingEntry{ID: "secret-wait", PokemonID: "secret-pokemon"},
		QueueCheck{Present: true, ID: "secret-queue", Priority: &priority, Waiting: []QueueWaitingEntry{{ID: "secret-wait", PokemonID: "secret-pokemon"}}},
		RaidPlayer{ID: "secret-player", TrainerName: "secret-trainer", Rating: &rating},
		RaidPlayers{Players: []RaidPlayer{{TrainerName: "secret-trainer"}}},
	}
	for _, value := range values {
		formatted := fmt.Sprintf("%v %+v %#v", value, value, value)
		if strings.Contains(formatted, "secret-") {
			t.Fatalf("format leaked protected upstream value: %s", formatted)
		}
	}
}
