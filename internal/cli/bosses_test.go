package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/metadata"
	"github.com/albinmedoc/raidnow-cli/internal/output"
)

func TestBossesListFiltersSortsAndRendersEnglishNames(t *testing.T) {
	operations := &stubRaidOperations{bosses: []domain.RaidBoss{
		{Rank: "3", PokemonID: "25", WaitingCount: 2},
		{Rank: "5", PokemonID: "6_x", WaitingCount: 7},
		{Rank: "5", PokemonID: "150", WaitingCount: 4},
	}}
	fetcher := &stubMetadataFetcher{catalog: testCatalog(t)}
	stdout, err := runPrimitiveCommandWithMetadata(t, operations, fetcher, "secret-token\n", "bosses", "list")
	if err != nil {
		t.Fatal(err)
	}
	want := "Armored Mewtwo rank=5 pokemon=150 waiting=4\nMega Charizard X rank=5 pokemon=6_x waiting=7\nPikachu rank=3 pokemon=25 waiting=2\n"
	if stdout != want || fetcher.calls != 1 || len(operations.calls) != 1 || operations.calls[0].name != "bosses" {
		t.Fatalf("stdout=%q metadata=%d calls=%#v", stdout, fetcher.calls, operations.calls)
	}

	operations.calls = nil
	fetcher.calls = 0
	stdout, err = runPrimitiveCommandWithMetadata(t, operations, fetcher, "secret-token\n", "bosses", "list", "--rank", "3")
	if err != nil || stdout != "Pikachu rank=3 pokemon=25 waiting=2\n" || fetcher.calls != 1 {
		t.Fatalf("filtered stdout=%q err=%v metadata=%d", stdout, err, fetcher.calls)
	}
}

func TestBossesListRendersJSONViewWithCatalogImageURLs(t *testing.T) {
	operations := &stubRaidOperations{bosses: []domain.RaidBoss{
		{Rank: "5", PokemonID: "150", WaitingCount: 4},
		{Rank: "3", PokemonID: "25", WaitingCount: 2},
	}}
	catalog, err := metadata.Parse([]byte(`<script>const aryPokemon={"150":{"en":"Armored Mewtwo","image":"https://images.example/150.png"},"25":{"en":"Pikachu","image":""}};</script>`))
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := runPrimitiveCommandWithMetadata(t, operations, &stubMetadataFetcher{catalog: catalog}, "token\n", "--json", "bosses", "list")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schemaVersion":1,"ok":true,"command":"bosses list","status":"listed","data":[{"rank":"5","pokemonId":"150","pokemonName":"Armored Mewtwo","imageUrl":"https://images.example/150.png","waitingCount":4},{"rank":"3","pokemonId":"25","pokemonName":"Pikachu","imageUrl":"","waitingCount":2}],"warnings":[]}` + "\n"
	if stdout != want {
		t.Fatalf("stdout=%q", stdout)
	}
}

func TestBossesListOmitsOnlyEntriesWithoutEnglishMetadata(t *testing.T) {
	operations := &stubRaidOperations{bosses: []domain.RaidBoss{
		{Rank: "5", PokemonID: "150", WaitingCount: 4},
		{Rank: "11", PokemonID: "missing", WaitingCount: 193},
		{Rank: "4", PokemonID: "blank", WaitingCount: 0},
	}}
	catalog, err := metadata.Parse([]byte(`<script>const aryPokemon={"150":{"en":"Armored Mewtwo","image":""},"blank":{"image":"https://images.example/blank.png"}};</script>`))
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := runPrimitiveCommandWithMetadata(t, operations, &stubMetadataFetcher{catalog: catalog}, "token\n", "--json", "bosses", "list")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schemaVersion":1,"ok":true,"command":"bosses list","status":"listed","data":[{"rank":"5","pokemonId":"150","pokemonName":"Armored Mewtwo","imageUrl":"","waitingCount":4}],"warnings":["2 raidbossar saknar engelsk metadata och utelämnades."]}` + "\n"
	if stdout != want {
		t.Fatalf("stdout=%q", stdout)
	}
}

func TestBossesListSkipsMetadataForEmptyResults(t *testing.T) {
	fetcher := &stubMetadataFetcher{err: errors.New("must not be called")}
	stdout, err := runPrimitiveCommandWithMetadata(t, &stubRaidOperations{}, fetcher, "token\n", "--json", "bosses", "list")
	if err != nil || !strings.Contains(stdout, `"data":[]`) || fetcher.calls != 0 {
		t.Fatalf("stdout=%q err=%v calls=%d", stdout, err, fetcher.calls)
	}
}

func TestBossesListValidatesBeforeTokenOperationsOrMetadata(t *testing.T) {
	operations := &stubRaidOperations{}
	fetcher := &stubMetadataFetcher{err: errors.New("must not be called")}
	stdout, err := runPrimitiveCommandWithMetadata(t, operations, fetcher, "SECRET-TOKEN\n", "bosses", "list", "--rank", "-1")
	if output.ExitCodeFor(err) != output.ExitInvalidArguments || stdout != "" || len(operations.calls) != 0 || fetcher.calls != 0 {
		t.Fatalf("exit=%d stdout=%q calls=%#v metadata=%d", output.ExitCodeFor(err), stdout, operations.calls, fetcher.calls)
	}
}

func TestLobbiesAndRaidStatusExposeEnglishNameAndExactID(t *testing.T) {
	fetcher := &stubMetadataFetcher{catalog: testCatalog(t)}
	operations := &stubRaidOperations{raids: []domain.Raid{{ID: "r", Rank: "5", PokemonID: "150"}}}
	stdout, err := runPrimitiveCommandWithMetadata(t, operations, fetcher, "token\n", "--json", "lobbies", "list")
	if err != nil || !strings.Contains(stdout, `"pokemonId":"150"`) || !strings.Contains(stdout, `"pokemonName":"Armored Mewtwo"`) || fetcher.calls != 1 {
		t.Fatalf("lobbies stdout=%q err=%v metadata=%d", stdout, err, fetcher.calls)
	}

	fetcher.calls = 0
	operations = &stubRaidOperations{found: &domain.Raid{ID: "r", PokemonID: "150"}}
	stdout, err = runPrimitiveCommandWithMetadata(t, operations, fetcher, "token\n", "raid", "status", "--raid-id", "r")
	if err != nil || stdout != "r name=Armored Mewtwo pokemon=150 players=0 ready=0\n" || fetcher.calls != 1 {
		t.Fatalf("raid stdout=%q err=%v metadata=%d", stdout, err, fetcher.calls)
	}
}

func TestEmptyAndMissingResultsSkipMetadata(t *testing.T) {
	fetcher := &stubMetadataFetcher{err: errors.New("must not be called")}
	stdout, err := runPrimitiveCommandWithMetadata(t, &stubRaidOperations{}, fetcher, "token\n", "--json", "lobbies", "list")
	if err != nil || !strings.Contains(stdout, `"data":[]`) || fetcher.calls != 0 {
		t.Fatalf("lobbies stdout=%q err=%v calls=%d", stdout, err, fetcher.calls)
	}

	stdout, err = runPrimitiveCommandWithMetadata(t, &stubRaidOperations{}, fetcher, "token\n", "raid", "status", "--raid-id", "missing")
	if output.ExitCodeFor(err) != output.ExitNotFound || stdout != "" || fetcher.calls != 0 {
		t.Fatalf("raid stdout=%q exit=%d calls=%d", stdout, output.ExitCodeFor(err), fetcher.calls)
	}
}

func TestMetadataFetchFailureUsesMetadataUnavailable(t *testing.T) {
	operations := &stubRaidOperations{bosses: []domain.RaidBoss{{Rank: "5", PokemonID: "150"}}}
	fetcher := &stubMetadataFetcher{err: errors.New("SECRET-UPSTREAM-PAGE")}
	stdout, err := runPrimitiveCommandWithMetadata(t, operations, fetcher, "SECRET-TOKEN\n", "bosses", "list")
	if output.ExitCodeFor(err) != output.ExitMetadataUnavailable || stdout != "" || strings.Contains(errorString(err), "SECRET") {
		t.Fatalf("exit=%d stdout=%q err=%v", output.ExitCodeFor(err), stdout, err)
	}
}

func TestPokemonNameCommandsFailClosedWithoutMetadataAndHideToken(t *testing.T) {
	for _, testCase := range []struct {
		operations *stubRaidOperations
		args       []string
	}{
		{operations: &stubRaidOperations{bosses: []domain.RaidBoss{{Rank: "5", PokemonID: "missing"}}}, args: []string{"bosses", "list"}},
		{operations: &stubRaidOperations{raids: []domain.Raid{{ID: "r", PokemonID: "missing"}}}, args: []string{"lobbies", "list"}},
		{operations: &stubRaidOperations{found: &domain.Raid{ID: "r", PokemonID: "missing"}}, args: []string{"raid", "status", "--raid-id", "r"}},
	} {
		fetcher := &stubMetadataFetcher{catalog: testCatalog(t)}
		stdout, err := runPrimitiveCommandWithMetadata(t, testCase.operations, fetcher, "SECRET-TOKEN\n", testCase.args...)
		if output.ExitCodeFor(err) != output.ExitMetadataUnavailable || strings.Contains(stdout+errorString(err), "SECRET-TOKEN") {
			t.Fatalf("args=%v exit=%d stdout=%q err=%v", testCase.args, output.ExitCodeFor(err), stdout, err)
		}
	}
}

func errorString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
