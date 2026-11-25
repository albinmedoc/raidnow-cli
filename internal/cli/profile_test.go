package cli

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/metadata"
	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/albinmedoc/raidnow-cli/internal/upstream"
)

type profileCall struct {
	name    string
	token   string
	options upstream.CallOptions
}

type stubProfileOperations struct {
	calls   []profileCall
	history []domain.ProfileHistoryRecord
	blocks  []domain.BlockRecord
	err     error
}

func (stub *stubProfileOperations) ListHostedHistory(_ context.Context, token string, options upstream.CallOptions) ([]domain.ProfileHistoryRecord, error) {
	stub.calls = append(stub.calls, profileCall{name: "hosted", token: token, options: options})
	return stub.history, stub.err
}
func (stub *stubProfileOperations) ListJoinedHistory(_ context.Context, token string, options upstream.CallOptions) ([]domain.ProfileHistoryRecord, error) {
	stub.calls = append(stub.calls, profileCall{name: "joined", token: token, options: options})
	return stub.history, stub.err
}
func (stub *stubProfileOperations) ListKickedHistory(_ context.Context, token string, options upstream.CallOptions) ([]domain.ProfileHistoryRecord, error) {
	stub.calls = append(stub.calls, profileCall{name: "kicked", token: token, options: options})
	return stub.history, stub.err
}
func (stub *stubProfileOperations) ListRemovedHistory(_ context.Context, token string, options upstream.CallOptions) ([]domain.ProfileHistoryRecord, error) {
	stub.calls = append(stub.calls, profileCall{name: "removed", token: token, options: options})
	return stub.history, stub.err
}
func (stub *stubProfileOperations) ListBlocks(_ context.Context, token string, options upstream.CallOptions) ([]domain.BlockRecord, error) {
	stub.calls = append(stub.calls, profileCall{name: "blocks", token: token, options: options})
	return stub.blocks, stub.err
}

func runProfileCommand(t *testing.T, operations ProfileOperations, fetcher *stubMetadataFetcher, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	command := NewRootCommandWithOptions(RootOptions{
		Bootstrap: &stubTokenBootstrapper{}, Publisher: &stubTrainerPublisher{}, Operations: &stubRaidOperations{},
		Profiles: operations, Metadata: fetcher, TokenFile: "-", RequestTimeout: 3 * time.Second,
	})
	command.SetIn(strings.NewReader("opaque-token\n"))
	command.SetOut(&stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs(args)
	err := command.Execute()
	return stdout.String(), err
}

func profileCatalog(t *testing.T) metadata.Catalog {
	t.Helper()
	catalog, err := metadata.Parse([]byte(`<script>const aryPokemon={"150":{"en":"Armored Mewtwo","image":"https://img.example/150.png"},"6_x":{"en":"Mega Charizard X"}};</script>`))
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func TestHistoryHostedRendersSafeMetadataEnrichedJSON(t *testing.T) {
	rating := 5
	profiles := &stubProfileOperations{history: []domain.ProfileHistoryRecord{{RaidID: "raid-1", DisplayTime: "3 hours ago", PokemonID: "150", TrainerName: "Host7", Level: 50, Country: "se", Rating: &rating}}}
	stdout, err := runProfileCommand(t, profiles, &stubMetadataFetcher{catalog: profileCatalog(t)}, "--json", "history", "hosted")
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"schemaVersion\":1,\"ok\":true,\"command\":\"history hosted\",\"status\":\"listed\",\"data\":[{\"raidId\":\"raid-1\",\"displayTime\":\"3 hours ago\",\"pokemonId\":\"150\",\"trainerName\":\"Host7\",\"level\":50,\"country\":\"se\",\"rating\":5,\"pokemonName\":\"Armored Mewtwo\",\"imageUrl\":\"https://img.example/150.png\"}],\"warnings\":[]}\n"
	if stdout != want {
		t.Fatalf("stdout=%q want=%q", stdout, want)
	}
	if len(profiles.calls) != 1 || profiles.calls[0].name != "hosted" || profiles.calls[0].token != "opaque-token" || profiles.calls[0].options.Timeout != 3*time.Second {
		t.Fatalf("calls=%#v", profiles.calls)
	}
}

func TestHistoryCommandsDispatchAndRenderHumanRows(t *testing.T) {
	for _, name := range []string{"hosted", "joined", "kicked", "removed"} {
		t.Run(name, func(t *testing.T) {
			guestRating := 4
			record := domain.ProfileHistoryRecord{RaidID: "raid-1", DisplayTime: "Yesterday", PokemonID: "6_x", TrainerName: "Host7", Level: 40, Country: "world"}
			if name == "joined" {
				record.Guest = &domain.ProfileGuest{TrainerName: "Guest8", Level: 44, Country: "jp", Rating: &guestRating}
			}
			profiles := &stubProfileOperations{history: []domain.ProfileHistoryRecord{record}}
			stdout, err := runProfileCommand(t, profiles, &stubMetadataFetcher{catalog: profileCatalog(t)}, "history", name)
			if err != nil || !strings.Contains(stdout, "raid-1 Yesterday name=Mega Charizard X pokemon=6_x trainer=Host7 level=40 country=world rating=none\n") {
				t.Fatalf("stdout=%q err=%v", stdout, err)
			}
			if name == "joined" && !strings.Contains(stdout, "  guest=Guest8 level=44 country=jp rating=4\n") {
				t.Fatalf("stdout=%q", stdout)
			}
			if len(profiles.calls) != 1 || profiles.calls[0].name != name {
				t.Fatalf("calls=%#v", profiles.calls)
			}
		})
	}
}

func TestHistoryAndBlocksAcceptEmptyArraysWithoutMetadata(t *testing.T) {
	profiles := &stubProfileOperations{}
	fetcher := &stubMetadataFetcher{panic: true}
	stdout, err := runProfileCommand(t, profiles, fetcher, "--json", "history", "removed")
	if err != nil || stdout != "{\"schemaVersion\":1,\"ok\":true,\"command\":\"history removed\",\"status\":\"listed\",\"data\":[],\"warnings\":[]}\n" || fetcher.calls != 0 {
		t.Fatalf("history stdout=%q calls=%d err=%v", stdout, fetcher.calls, err)
	}
	stdout, err = runProfileCommand(t, profiles, fetcher, "--json", "blocks", "list")
	if err != nil || stdout != "{\"schemaVersion\":1,\"ok\":true,\"command\":\"blocks list\",\"status\":\"listed\",\"data\":[],\"warnings\":[]}\n" {
		t.Fatalf("blocks stdout=%q err=%v", stdout, err)
	}
}

func TestBlocksListRendersSafeJSONAndHumanRows(t *testing.T) {
	profiles := &stubProfileOperations{blocks: []domain.BlockRecord{{BlockID: "77", TrainerName: "Blocked7"}}}
	stdout, err := runProfileCommand(t, profiles, &stubMetadataFetcher{panic: true}, "--json", "blocks", "list")
	if err != nil || stdout != "{\"schemaVersion\":1,\"ok\":true,\"command\":\"blocks list\",\"status\":\"listed\",\"data\":[{\"blockId\":\"77\",\"trainerName\":\"Blocked7\"}],\"warnings\":[]}\n" {
		t.Fatalf("stdout=%q err=%v", stdout, err)
	}
	stdout, err = runProfileCommand(t, profiles, &stubMetadataFetcher{panic: true}, "blocks", "list")
	if err != nil || stdout != "77 trainer=Blocked7\n" {
		t.Fatalf("stdout=%q err=%v", stdout, err)
	}
}

func TestHistoryAuthenticationFailureProducesNoOutput(t *testing.T) {
	profiles := &stubProfileOperations{err: output.NewError(output.CodeAuthenticationUnsupported, "secret cookie-id")}
	stdout, err := runProfileCommand(t, profiles, &stubMetadataFetcher{panic: true}, "history", "joined")
	if output.ExitCodeFor(err) != output.ExitAuthenticationUnsupported || stdout != "" || strings.Contains(err.Error(), "secret") {
		t.Fatalf("exit=%d stdout=%q err=%v", output.ExitCodeFor(err), stdout, err)
	}
}

func TestHistoryRejectsWhitespaceOnlyEnglishPokemonMetadata(t *testing.T) {
	catalog, err := metadata.Parse([]byte(`<script>const aryPokemon={"150":{"en":"   ","image":"https://img.example/150.png"}};</script>`))
	if err != nil {
		t.Fatal(err)
	}
	profiles := &stubProfileOperations{history: []domain.ProfileHistoryRecord{{RaidID: "raid-1", PokemonID: "150"}}}
	stdout, err := runProfileCommand(t, profiles, &stubMetadataFetcher{catalog: catalog}, "--json", "history", "hosted")
	if output.ExitCodeFor(err) == 0 || stdout != "" {
		t.Fatalf("exit=%d stdout=%q err=%v", output.ExitCodeFor(err), stdout, err)
	}
}
