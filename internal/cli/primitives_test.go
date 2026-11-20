package cli

import (
	"bytes"
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/metadata"
	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/albinmedoc/raidnow-cli/internal/upstream"
)

type primitiveCall struct {
	name      string
	token     string
	raidID    string
	trainer   domain.Trainer
	identity  domain.TrainerIdentity
	selection upstream.QueueSelection
	query     upstream.LobbyQuery
	pokemon   []string
	options   upstream.CallOptions
}

type stubRaidOperations struct {
	calls        []primitiveCall
	err          error
	createErr    error
	pollErr      error
	cancelErr    error
	checkErr     error
	poll         domain.QueuePoll
	polls        []domain.QueuePoll
	check        domain.QueueCheck
	raids        []domain.Raid
	bosses       []domain.RaidBoss
	found        *domain.Raid
	players      domain.RaidPlayers
	afterPoll    func(int)
	cancelCtxErr error
	checkCtxErr  error
}

func (stub *stubRaidOperations) ListCurrentRaidBosses(_ context.Context, token string, options upstream.CallOptions) ([]domain.RaidBoss, error) {
	stub.calls = append(stub.calls, primitiveCall{name: "bosses", token: token, options: options})
	return stub.bosses, stub.err
}

func (stub *stubRaidOperations) CreateMatching(_ context.Context, token string, trainer domain.Trainer, selection upstream.QueueSelection, options upstream.CallOptions) error {
	stub.calls = append(stub.calls, primitiveCall{name: "create", token: token, trainer: trainer, selection: selection, options: options})
	if stub.createErr != nil {
		return stub.createErr
	}
	return stub.err
}
func (stub *stubRaidOperations) PollMatching(_ context.Context, token string, selection upstream.QueueSelection, options upstream.CallOptions) (domain.QueuePoll, error) {
	stub.calls = append(stub.calls, primitiveCall{name: "poll", token: token, selection: selection, options: options})
	pollNumber := 0
	for _, call := range stub.calls {
		if call.name == "poll" {
			pollNumber++
		}
	}
	if stub.afterPoll != nil {
		stub.afterPoll(pollNumber)
	}
	if stub.pollErr != nil {
		return domain.QueuePoll{}, stub.pollErr
	}
	if pollNumber <= len(stub.polls) {
		return stub.polls[pollNumber-1], nil
	}
	return stub.poll, stub.err
}
func (stub *stubRaidOperations) CancelMatching(ctx context.Context, token string, options upstream.CallOptions) error {
	stub.calls = append(stub.calls, primitiveCall{name: "cancel", token: token, options: options})
	stub.cancelCtxErr = ctx.Err()
	if stub.cancelErr != nil {
		return stub.cancelErr
	}
	return stub.err
}
func (stub *stubRaidOperations) CheckMatching(ctx context.Context, token string, pokemon []string, options upstream.CallOptions) (domain.QueueCheck, error) {
	stub.calls = append(stub.calls, primitiveCall{name: "check", token: token, pokemon: pokemon, options: options})
	stub.checkCtxErr = ctx.Err()
	if stub.checkErr != nil {
		return domain.QueueCheck{}, stub.checkErr
	}
	return stub.check, stub.err
}
func (stub *stubRaidOperations) ListRaids(_ context.Context, token string, query upstream.LobbyQuery, options upstream.CallOptions) ([]domain.Raid, error) {
	stub.calls = append(stub.calls, primitiveCall{name: "list", token: token, query: query, options: options})
	return stub.raids, stub.err
}
func (stub *stubRaidOperations) JoinRaid(_ context.Context, token, raidID string, identity domain.TrainerIdentity, options upstream.CallOptions) error {
	stub.calls = append(stub.calls, primitiveCall{name: "join", token: token, raidID: raidID, identity: identity, options: options})
	return stub.err
}
func (stub *stubRaidOperations) FindRaid(_ context.Context, token, raidID string, options upstream.CallOptions) (*domain.Raid, error) {
	stub.calls = append(stub.calls, primitiveCall{name: "find", token: token, raidID: raidID, options: options})
	return stub.found, stub.err
}
func (stub *stubRaidOperations) ListRaidPlayers(_ context.Context, token, raidID string, options upstream.CallOptions) (domain.RaidPlayers, error) {
	stub.calls = append(stub.calls, primitiveCall{name: "players", token: token, raidID: raidID, options: options})
	return stub.players, stub.err
}
func (stub *stubRaidOperations) LeaveRaid(_ context.Context, token, raidID string, options upstream.CallOptions) error {
	stub.calls = append(stub.calls, primitiveCall{name: "leave", token: token, raidID: raidID, options: options})
	return stub.err
}

func runPrimitiveCommand(t *testing.T, operations *stubRaidOperations, input string, args ...string) (string, error) {
	return runPrimitiveCommandWithMetadata(t, operations, &stubMetadataFetcher{catalog: testCatalog(t)}, input, args...)
}

type stubMetadataFetcher struct {
	catalog metadata.Catalog
	err     error
	calls   int
	panic   bool
}

func (stub *stubMetadataFetcher) Catalog(context.Context) (metadata.Catalog, error) {
	stub.calls++
	if stub.panic {
		panic("metadata fetch must not be called")
	}
	return stub.catalog, stub.err
}

func testCatalog(t *testing.T) metadata.Catalog {
	t.Helper()
	catalog, err := metadata.Parse([]byte(`<script>const aryPokemon={"150":{"en":"Armored Mewtwo"},"6_x":{"en":"Mega Charizard X"},"25":{"en":"Pikachu"}};</script>`))
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func runPrimitiveCommandWithMetadata(t *testing.T, operations *stubRaidOperations, fetcher *stubMetadataFetcher, input string, args ...string) (string, error) {
	t.Helper()
	var stdout bytes.Buffer
	command := NewRootCommandWithOptions(RootOptions{Bootstrap: &stubTokenBootstrapper{}, Publisher: &stubTrainerPublisher{}, Operations: operations, Metadata: fetcher, TokenFile: "-", RequestTimeout: 3 * time.Second})
	command.SetIn(strings.NewReader(input))
	command.SetOut(&stdout)
	command.SetErr(&bytes.Buffer{})
	command.SetArgs(args)
	err := command.Execute()
	return stdout.String(), err
}

func TestQueueJoinCallsCreateOnceAndRendersHumanSuccess(t *testing.T) {
	operations := &stubRaidOperations{}
	stdout, err := runPrimitiveCommand(t, operations, "opaque-token\n", "queue", "join", "--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-country", "SE", "--trainer-code-file", protectedTrainerCodeFile(t, "123456789012"), "--rank", "5", "--pokemon", "150", "--rank", "mega", "--pokemon", "6_x", "--guest-score", "4", "--min-host-rating", "-1", "--host-country", "1", "--weather-boost", "2", "--host-level", "70")
	if err != nil || stdout != "Kön skapades.\n" {
		t.Fatalf("stdout=%q err=%v", stdout, err)
	}
	if len(operations.calls) != 1 {
		t.Fatalf("calls=%#v", operations.calls)
	}
	call := operations.calls[0]
	if call.name != "create" || call.token != "opaque-token" || call.trainer.CodeFormValue() != "123456789012" || call.options.Timeout != 3*time.Second || len(call.selection.Ranks) != 2 || call.selection.PokemonIDs[1] != "6_x" || call.selection.GuestEvalStar != "4" || call.selection.MinimumHostRating != -1 || call.selection.HostCountry != 1 || call.selection.WeatherBoost != 2 || call.selection.HostLevel != 70 {
		t.Fatalf("call=%+v", call)
	}
}

func TestQueueJoinUsesMatchingFilterDefaults(t *testing.T) {
	operations := &stubRaidOperations{}
	_, err := runPrimitiveCommand(t, operations, "opaque-token\n", "queue", "join", "--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-country", "SE", "--trainer-code-file", protectedTrainerCodeFile(t, "123456789012"), "--rank", "5", "--pokemon", "150")
	if err != nil || len(operations.calls) != 1 {
		t.Fatalf("err=%v calls=%#v", err, operations.calls)
	}
	selection := operations.calls[0].selection
	if selection.MinimumHostRating != 3 || selection.HostCountry != 0 || selection.WeatherBoost != 0 || selection.HostLevel != 0 {
		t.Fatalf("selection=%+v", selection)
	}
}

func TestQueueJoinRejectsInvalidMatchingFiltersBeforeReadingSecrets(t *testing.T) {
	operations := &stubRaidOperations{}
	command := NewRootCommandWithOptions(RootOptions{Bootstrap: &stubTokenBootstrapper{}, Publisher: &stubTrainerPublisher{}, Operations: operations, TokenFile: "-", RequestTimeout: time.Second})
	command.SetIn(panicReader{})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"queue", "join", "--trainer-name", "Ash123", "--trainer-level", "50", "--trainer-country", "SE", "--trainer-code-file", protectedTrainerCodeFile(t, "123456789012"), "--rank", "5", "--pokemon", "150", "--host-country", "2"})
	err := command.Execute()
	if output.ExitCodeFor(err) != output.ExitInvalidArguments || len(operations.calls) != 0 {
		t.Fatalf("exit=%d calls=%#v err=%v", output.ExitCodeFor(err), operations.calls, err)
	}
}

func TestQueueStatusRendersNormalizedJSON(t *testing.T) {
	operations := &stubRaidOperations{poll: domain.QueuePoll{State: domain.QueueMatched, RaidID: "raid-7"}}
	stdout, err := runPrimitiveCommand(t, operations, "token\n", "--json", "queue", "status", "--rank", "5", "--pokemon", "150")
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"schemaVersion\":1,\"ok\":true,\"command\":\"queue status\",\"status\":\"matched\",\"data\":{\"state\":\"matched\",\"raidId\":\"raid-7\"},\"warnings\":[]}\n"
	if stdout != want || len(operations.calls) != 1 || operations.calls[0].name != "poll" {
		t.Fatalf("stdout=%q calls=%#v", stdout, operations.calls)
	}
}

func TestQueueStatusRendersExactHumanStates(t *testing.T) {
	for _, testCase := range []struct {
		poll domain.QueuePoll
		want string
	}{
		{poll: domain.QueuePoll{State: domain.QueueWaiting}, want: "waiting\n"},
		{poll: domain.QueuePoll{State: domain.QueueMatched, RaidID: "raid-7"}, want: "matched raid-7\n"},
	} {
		stdout, err := runPrimitiveCommand(t, &stubRaidOperations{poll: testCase.poll}, "token\n", "queue", "status", "--rank", "5", "--pokemon", "150")
		if err != nil || stdout != testCase.want {
			t.Fatalf("stdout=%q want=%q err=%v", stdout, testCase.want, err)
		}
	}
}

func TestQueueCheckRendersExactHumanStatesAndCallsCheckOnce(t *testing.T) {
	for _, testCase := range []struct {
		present bool
		want    string
	}{
		{present: true, want: "present\n"},
		{present: false, want: "absent\n"},
	} {
		operations := &stubRaidOperations{check: domain.QueueCheck{Present: testCase.present}}
		stdout, err := runPrimitiveCommand(t, operations, "opaque-token\n", "queue", "check", "--pokemon", "150", "--pokemon", "6_x")
		if err != nil || stdout != testCase.want {
			t.Fatalf("present=%v stdout=%q err=%v", testCase.present, stdout, err)
		}
		if len(operations.calls) != 1 {
			t.Fatalf("calls=%#v", operations.calls)
		}
		call := operations.calls[0]
		if call.name != "check" || call.token != "opaque-token" || call.options.Timeout != 3*time.Second || len(call.pokemon) != 2 || call.pokemon[0] != "150" || call.pokemon[1] != "6_x" {
			t.Fatalf("call=%+v", call)
		}
	}
}

func TestQueueCheckRendersQueueCheckUnchangedAsJSON(t *testing.T) {
	priority := 2
	for _, testCase := range []struct {
		check domain.QueueCheck
		want  string
	}{
		{
			check: domain.QueueCheck{
				Present:  true,
				ID:       "queue-7",
				Priority: &priority,
				Waiting:  []domain.QueueWaitingEntry{{ID: "wait-1", Rank: "5", PokemonID: "150"}},
			},
			want: "{\"schemaVersion\":1,\"ok\":true,\"command\":\"queue check\",\"status\":\"present\",\"data\":{\"present\":true,\"id\":\"queue-7\",\"priority\":2,\"waiting\":[{\"id\":\"wait-1\",\"rank\":\"5\",\"pokemonId\":\"150\"}]},\"warnings\":[]}\n",
		},
		{
			check: domain.QueueCheck{Waiting: []domain.QueueWaitingEntry{}},
			want:  "{\"schemaVersion\":1,\"ok\":true,\"command\":\"queue check\",\"status\":\"absent\",\"data\":{\"present\":false,\"waiting\":[]},\"warnings\":[]}\n",
		},
	} {
		operations := &stubRaidOperations{check: testCase.check}
		stdout, err := runPrimitiveCommand(t, operations, "token\n", "--json", "queue", "check", "--pokemon", "150")
		if err != nil {
			t.Fatal(err)
		}
		if stdout != testCase.want || len(operations.calls) != 1 || operations.calls[0].name != "check" {
			t.Fatalf("stdout=%q calls=%#v", stdout, operations.calls)
		}
	}
}

func TestQueueCheckRequiresPokemonBeforeReadingToken(t *testing.T) {
	operations := &stubRaidOperations{}
	command := NewRootCommandWithOptions(RootOptions{Bootstrap: &stubTokenBootstrapper{}, Publisher: &stubTrainerPublisher{}, Operations: operations, TokenFile: "-", RequestTimeout: time.Second})
	command.SetIn(panicReader{})
	command.SetOut(&bytes.Buffer{})
	command.SetErr(&bytes.Buffer{})
	command.SetArgs([]string{"queue", "check"})
	err := command.Execute()
	if output.ExitCodeFor(err) != output.ExitInvalidArguments || len(operations.calls) != 0 {
		t.Fatalf("exit=%d calls=%#v err=%v", output.ExitCodeFor(err), operations.calls, err)
	}
}

func TestQueueCheckSanitizesOperationErrors(t *testing.T) {
	operations := &stubRaidOperations{checkErr: output.NewError(output.CodeReadNetwork, "secret upstream response")}
	stdout, err := runPrimitiveCommand(t, operations, "token\n", "queue", "check", "--pokemon", "150")
	if output.ExitCodeFor(err) != output.ExitReadNetwork || strings.Contains(err.Error(), "secret") || stdout != "" {
		t.Fatalf("exit=%d stdout=%q err=%v", output.ExitCodeFor(err), stdout, err)
	}
	if len(operations.calls) != 1 || operations.calls[0].name != "check" {
		t.Fatalf("calls=%#v", operations.calls)
	}
}

func TestQueueCancelVerifiesAbsenceAndRejectsStillPresent(t *testing.T) {
	for _, testCase := range []struct {
		present bool
		wantErr int
		wantOut string
	}{
		{wantOut: "Kön avbröts.\n"},
		{present: true, wantErr: output.ExitLifecycleConflict},
	} {
		operations := &stubRaidOperations{check: domain.QueueCheck{Present: testCase.present}}
		stdout, err := runPrimitiveCommand(t, operations, "token\n", "queue", "cancel", "--pokemon", "150")
		if output.ExitCodeFor(err) != testCase.wantErr || stdout != testCase.wantOut {
			t.Fatalf("present=%v exit=%d stdout=%q err=%v", testCase.present, output.ExitCodeFor(err), stdout, err)
		}
		if len(operations.calls) != 2 || operations.calls[0].name != "cancel" || operations.calls[1].name != "check" {
			t.Fatalf("calls=%#v", operations.calls)
		}
	}
}

func TestQueueCancelDefaultVerificationPreservesJSONOutput(t *testing.T) {
	operations := &stubRaidOperations{check: domain.QueueCheck{Waiting: []domain.QueueWaitingEntry{}}}
	stdout, err := runPrimitiveCommand(t, operations, "token\n", "--json", "queue", "cancel", "--pokemon", "150")
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"schemaVersion\":1,\"ok\":true,\"command\":\"queue cancel\",\"status\":\"cancelled\",\"data\":{\"cancelled\":true},\"warnings\":[]}\n"
	if stdout != want || len(operations.calls) != 2 || operations.calls[0].name != "cancel" || operations.calls[1].name != "check" {
		t.Fatalf("stdout=%q calls=%#v", stdout, operations.calls)
	}
}

func TestQueueCancelWithoutVerificationNeedsNoPokemonAndCallsCancelOnce(t *testing.T) {
	operations := &stubRaidOperations{}
	stdout, err := runPrimitiveCommand(t, operations, "token\n", "queue", "cancel", "--verify=false")
	if err != nil || stdout != "Avbrytningsbegäran skickades.\n" {
		t.Fatalf("stdout=%q err=%v", stdout, err)
	}
	if len(operations.calls) != 1 || operations.calls[0].name != "cancel" {
		t.Fatalf("calls=%#v", operations.calls)
	}
}

func TestQueueCancelWithoutVerificationRendersRequestedJSON(t *testing.T) {
	operations := &stubRaidOperations{}
	stdout, err := runPrimitiveCommand(t, operations, "token\n", "--json", "queue", "cancel", "--verify=false")
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"schemaVersion\":1,\"ok\":true,\"command\":\"queue cancel\",\"status\":\"requested\",\"data\":{\"requested\":true,\"verified\":false},\"warnings\":[]}\n"
	if stdout != want || len(operations.calls) != 1 || operations.calls[0].name != "cancel" {
		t.Fatalf("stdout=%q calls=%#v", stdout, operations.calls)
	}
}

func TestQueueCancelWithoutVerificationPreservesUnknownWriteOutcome(t *testing.T) {
	operations := &stubRaidOperations{cancelErr: output.NewError(output.CodeWriteOutcomeUnknown, "secret uncertain write")}
	stdout, err := runPrimitiveCommand(t, operations, "token\n", "queue", "cancel", "--verify=false")
	if output.ExitCodeFor(err) != output.ExitWriteOutcomeUnknown || strings.Contains(err.Error(), "secret") || stdout != "" {
		t.Fatalf("exit=%d stdout=%q err=%v", output.ExitCodeFor(err), stdout, err)
	}
	if len(operations.calls) != 1 || operations.calls[0].name != "cancel" {
		t.Fatalf("calls=%#v", operations.calls)
	}
}

func TestLobbiesListUsesFlagsAndIncludesExactIDs(t *testing.T) {
	operations := &stubRaidOperations{raids: []domain.Raid{{ID: "raid-A", Rank: "5", PokemonID: "150"}, {ID: "raid-B", Rank: "mega", PokemonID: "6_x"}}}
	stdout, err := runPrimitiveCommand(t, operations, "token\n", "lobbies", "list", "--page", "2", "--rank", "5", "--host-level", "50", "--exclude-pokemon", "1", "--exclude-pokemon", "2")
	if err != nil || !strings.Contains(stdout, "raid-A rank=5 name=Armored Mewtwo pokemon=150") || !strings.Contains(stdout, "raid-B rank=mega name=Mega Charizard X pokemon=6_x") {
		t.Fatalf("stdout=%q err=%v", stdout, err)
	}
	call := operations.calls[0]
	if call.name != "list" || call.query.Page != 2 || call.query.SearchRank != "5" || call.query.HostLevel != 50 || len(call.query.Filters) != 2 {
		t.Fatalf("call=%+v", call)
	}
}

func TestLobbiesListMapsMinimumHostRating(t *testing.T) {
	for _, testCase := range []struct {
		minimum int
		want    []int
	}{
		{minimum: -1, want: []int{}},
		{minimum: 0, want: []int{-1}},
		{minimum: 3, want: []int{-1, 0, 1, 2}},
		{minimum: 5, want: []int{-1, 0, 1, 2, 3, 4}},
	} {
		t.Run(fmt.Sprint(testCase.minimum), func(t *testing.T) {
			operations := &stubRaidOperations{}
			_, err := runPrimitiveCommand(t, operations, "token\n", "lobbies", "list", "--min-host-rating", fmt.Sprint(testCase.minimum))
			if err != nil || len(operations.calls) != 1 {
				t.Fatalf("err=%v calls=%#v", err, operations.calls)
			}
			if got := operations.calls[0].query.HostEvaluate; !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("host evaluate=%v want=%v", got, testCase.want)
			}
		})
	}
}

func TestLobbiesListRejectsInvalidMinimumHostRatingBeforeTokenOrNetwork(t *testing.T) {
	for _, minimum := range []string{"-2", "6"} {
		operations := &stubRaidOperations{}
		command := NewRootCommandWithOptions(RootOptions{Bootstrap: &stubTokenBootstrapper{}, Publisher: &stubTrainerPublisher{}, Operations: operations, TokenFile: "-", RequestTimeout: time.Second})
		command.SetIn(panicReader{})
		command.SetOut(&bytes.Buffer{})
		command.SetErr(&bytes.Buffer{})
		command.SetArgs([]string{"lobbies", "list", "--min-host-rating", minimum})
		err := command.Execute()
		if output.ExitCodeFor(err) != output.ExitInvalidArguments || len(operations.calls) != 0 {
			t.Fatalf("minimum=%s exit=%d calls=%#v err=%v", minimum, output.ExitCodeFor(err), operations.calls, err)
		}
	}
}

func TestLobbiesListCanSkipMetadata(t *testing.T) {
	raid := domain.Raid{ID: "raid-A", Rank: "5", PokemonID: "150", HostTrainerCode: "123456789012", Deleted: true, IsMe: true, MinimumGuestLevel: 40}
	operations := &stubRaidOperations{raids: []domain.Raid{raid}}
	fetcher := &stubMetadataFetcher{panic: true}
	stdout, err := runPrimitiveCommandWithMetadata(t, operations, fetcher, "token\n", "--json", "lobbies", "list", "--include-metadata=false")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"deleted":true`, `"isMe":true`, `"minimumGuestLevel":40`} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout=%q missing %s", stdout, want)
		}
	}
	if strings.Contains(stdout, "pokemonName") || strings.Contains(stdout, "123456789012") || strings.Contains(stdout, "hostTrainerCode") || fetcher.calls != 0 {
		t.Fatalf("stdout=%q metadata calls=%d", stdout, fetcher.calls)
	}

	stdout, err = runPrimitiveCommandWithMetadata(t, operations, fetcher, "token\n", "lobbies", "list", "--include-metadata=false")
	if err != nil || stdout != "raid-A rank=5 pokemon=150 players=0/0\n" || fetcher.calls != 0 {
		t.Fatalf("stdout=%q err=%v metadata calls=%d", stdout, err, fetcher.calls)
	}
}

func TestLobbiesListAcceptsEmptyJSONList(t *testing.T) {
	stdout, err := runPrimitiveCommand(t, &stubRaidOperations{}, "token\n", "--json", "lobbies", "list")
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"schemaVersion\":1,\"ok\":true,\"command\":\"lobbies list\",\"status\":\"listed\",\"data\":[],\"warnings\":[]}\n"
	if stdout != want {
		t.Fatalf("stdout=%q want=%q", stdout, want)
	}
}

func TestRaidJoinNeedsNoTrainerCodeAndRendersJSON(t *testing.T) {
	operations := &stubRaidOperations{}
	stdout, err := runPrimitiveCommand(t, operations, "token\n", "--json", "raid", "join", "--raid-id", "raid-9", "--trainer-name", "Misty7", "--trainer-level", "44", "--trainer-country", "JP")
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"schemaVersion\":1,\"ok\":true,\"command\":\"raid join\",\"status\":\"joined\",\"data\":{\"joined\":true,\"raidId\":\"raid-9\"},\"warnings\":[]}\n"
	if stdout != want || len(operations.calls) != 1 || operations.calls[0].identity.CountryFormValue() != "jp" {
		t.Fatalf("stdout=%q calls=%#v", stdout, operations.calls)
	}
}

func TestRaidJoinRendersExactHumanSuccess(t *testing.T) {
	stdout, err := runPrimitiveCommand(t, &stubRaidOperations{}, "token\n", "raid", "join", "--raid-id", "raid-9", "--trainer-name", "Misty7", "--trainer-level", "44", "--trainer-country", "JP")
	if err != nil || stdout != "Anslutningen skickades.\n" {
		t.Fatalf("stdout=%q err=%v", stdout, err)
	}
}

func TestRaidPlayersCallsListOnceAndRendersUnchangedJSON(t *testing.T) {
	rating := 5
	players := domain.RaidPlayers{Players: []domain.RaidPlayer{{ID: "hidden-player-id", TrainerName: "Ash123", Level: 50, Country: "se", Rating: &rating, Ready: true, IsMe: true}}, Count: 1, ReadyCount: 1, RaidDeleted: false}
	operations := &stubRaidOperations{players: players}
	fetcher := &stubMetadataFetcher{panic: true}
	stdout, err := runPrimitiveCommandWithMetadata(t, operations, fetcher, "token\n", "--json", "raid", "players", "--raid-id", "raid-9")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"command":"raid players"`, `"status":"listed"`, `"data":{"players":[{"id":"hidden-player-id"`, `"count":1`, `"readyCount":1`, `"raidDeleted":false`} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout=%q missing %s", stdout, want)
		}
	}
	if len(operations.calls) != 1 || operations.calls[0].name != "players" || operations.calls[0].raidID != "raid-9" || operations.calls[0].options.Timeout != 3*time.Second || fetcher.calls != 0 {
		t.Fatalf("calls=%#v metadata calls=%d", operations.calls, fetcher.calls)
	}
}

func TestRaidPlayersHumanOmitsHiddenIDsAndRendersRows(t *testing.T) {
	rating := 5
	players := domain.RaidPlayers{Players: []domain.RaidPlayer{
		{ID: "hidden-player-id", TrainerName: "Ash123", Level: 50, Country: "se", Rating: &rating, Ready: true, IsMe: true},
		{ID: "another-hidden-id", TrainerName: "Misty7", Level: 44},
	}, Count: 2, ReadyCount: 1}
	stdout, err := runPrimitiveCommand(t, &stubRaidOperations{players: players}, "token\n", "raid", "players", "--raid-id", "raid-9")
	want := "players=2 ready=1 deleted=false\nAsh123 level=50 country=se rating=5 ready=true me=true\nMisty7 level=44 country= rating=none ready=false me=false\n"
	if err != nil || stdout != want || strings.Contains(stdout, "hidden") {
		t.Fatalf("stdout=%q err=%v", stdout, err)
	}
}

func TestRaidGetCallsFindOnceWithoutMetadataOrPlayers(t *testing.T) {
	raid := &domain.Raid{ID: "raid-9", Rank: "5", PokemonID: "150", Count: 2}
	operations := &stubRaidOperations{found: raid}
	fetcher := &stubMetadataFetcher{panic: true}
	stdout, err := runPrimitiveCommandWithMetadata(t, operations, fetcher, "token\n", "raid", "get", "--raid-id", "raid-9")
	if err != nil || stdout != "raid-9 pokemon=150 players=2\n" {
		t.Fatalf("stdout=%q err=%v", stdout, err)
	}
	if len(operations.calls) != 1 || operations.calls[0].name != "find" || operations.calls[0].options.Timeout != 3*time.Second || fetcher.calls != 0 {
		t.Fatalf("calls=%#v metadata calls=%d", operations.calls, fetcher.calls)
	}
}

func TestRaidGetReturnsNotFoundForValidAbsence(t *testing.T) {
	operations := &stubRaidOperations{}
	stdout, err := runPrimitiveCommand(t, operations, "token\n", "raid", "get", "--raid-id", "missing")
	if output.ExitCodeFor(err) != output.ExitNotFound || stdout != "" || len(operations.calls) != 1 || operations.calls[0].name != "find" {
		t.Fatalf("exit=%d stdout=%q calls=%#v err=%v", output.ExitCodeFor(err), stdout, operations.calls, err)
	}
}

func TestRaidGetJSONMarksAndWarnsAboutHostTrainerCode(t *testing.T) {
	raid := &domain.Raid{ID: "raid-9", Rank: "5", PokemonID: "150", Count: 2, HostTrainerCode: "123456789012"}
	operations := &stubRaidOperations{found: raid}
	stdout, err := runPrimitiveCommand(t, operations, "token\n", "--json", "raid", "get", "--raid-id", "raid-9")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"command":"raid get"`, `"status":"found"`, `"hostTrainerCode":"123456789012"`, `"containsSecret":true`, `"warnings":["Värdens tränarkod är hemlig; hantera den säkert."]`} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout=%q missing %s", stdout, want)
		}
	}
	if len(operations.calls) != 1 || operations.calls[0].name != "find" {
		t.Fatalf("calls=%#v", operations.calls)
	}
}

func TestRaidGetJSONWithoutCodeHasNoSecretWarning(t *testing.T) {
	raid := &domain.Raid{ID: "raid-9", PokemonID: "150"}
	stdout, err := runPrimitiveCommand(t, &stubRaidOperations{found: raid}, "token\n", "--json", "raid", "get", "--raid-id", "raid-9")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout, `"containsSecret":false`) || !strings.Contains(stdout, `"warnings":[]`) || strings.Contains(stdout, "hostTrainerCode") {
		t.Fatalf("stdout=%q", stdout)
	}
}

func TestRaidGetHumanNeverPrintsHostTrainerCode(t *testing.T) {
	raid := &domain.Raid{ID: "raid-9", PokemonID: "150", Count: 2, HostTrainerCode: "123456789012"}
	stdout, err := runPrimitiveCommand(t, &stubRaidOperations{found: raid}, "token\n", "raid", "get", "--raid-id", "raid-9")
	if err != nil || stdout != "raid-9 pokemon=150 players=2\n" || strings.Contains(stdout, raid.HostTrainerCode) {
		t.Fatalf("stdout=%q err=%v", stdout, err)
	}
}

func TestRaidStatusFindsRaidThenListsPlayers(t *testing.T) {
	raid := &domain.Raid{ID: "raid-9", PokemonID: "150", Count: 2}
	operations := &stubRaidOperations{found: raid, players: domain.RaidPlayers{Count: 2, ReadyCount: 1, Players: []domain.RaidPlayer{{ID: "p1", TrainerName: "Ash123"}}}}
	stdout, err := runPrimitiveCommand(t, operations, "token\n", "--json", "raid", "status", "--raid-id", "raid-9")
	if err != nil || !strings.Contains(stdout, `"raid":{"id":"raid-9"`) || !strings.Contains(stdout, `"pokemonName":"Armored Mewtwo"`) || !strings.Contains(stdout, `"players":{"players":[`) {
		t.Fatalf("stdout=%q err=%v", stdout, err)
	}
	if len(operations.calls) != 2 || operations.calls[0].name != "find" || operations.calls[1].name != "players" {
		t.Fatalf("calls=%#v", operations.calls)
	}

	operations = &stubRaidOperations{}
	_, err = runPrimitiveCommand(t, operations, "token\n", "raid", "status", "--raid-id", "missing")
	if output.ExitCodeFor(err) != output.ExitNotFound || len(operations.calls) != 1 {
		t.Fatalf("exit=%d calls=%#v", output.ExitCodeFor(err), operations.calls)
	}
}

func TestRaidStatusJSONIncludesMarkedHostTrainerCodeButHumanDoesNot(t *testing.T) {
	raid := &domain.Raid{ID: "raid-9", PokemonID: "150", HostTrainerCode: "123456789012"}
	players := domain.RaidPlayers{Count: 2, ReadyCount: 1}
	operations := &stubRaidOperations{found: raid, players: players}
	stdout, err := runPrimitiveCommand(t, operations, "token\n", "--json", "raid", "status", "--raid-id", "raid-9")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"hostTrainerCode":"123456789012"`, `"containsSecret":true`, `"warnings":["Värdens tränarkod är hemlig; hantera den säkert."]`} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout=%q missing %s", stdout, want)
		}
	}

	operations = &stubRaidOperations{found: raid, players: players}
	stdout, err = runPrimitiveCommand(t, operations, "token\n", "raid", "status", "--raid-id", "raid-9")
	if err != nil || strings.Contains(stdout, raid.HostTrainerCode) {
		t.Fatalf("stdout=%q err=%v", stdout, err)
	}
}

func TestRaidLeaveWithoutVerificationCallsLeaveOnce(t *testing.T) {
	operations := &stubRaidOperations{}
	stdout, err := runPrimitiveCommand(t, operations, "token\n", "raid", "leave", "--raid-id", "raid-9", "--verify=false")
	if err != nil || stdout != "Begäran att lämna raiden skickades.\n" {
		t.Fatalf("stdout=%q err=%v", stdout, err)
	}
	if len(operations.calls) != 1 || operations.calls[0].name != "leave" {
		t.Fatalf("calls=%#v", operations.calls)
	}
}

func TestRaidLeaveWithoutVerificationRendersRequestedJSON(t *testing.T) {
	operations := &stubRaidOperations{}
	stdout, err := runPrimitiveCommand(t, operations, "token\n", "--json", "raid", "leave", "--raid-id", "raid-9", "--verify=false")
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"schemaVersion\":1,\"ok\":true,\"command\":\"raid leave\",\"status\":\"requested\",\"data\":{\"requested\":true,\"verified\":false,\"raidId\":\"raid-9\"},\"warnings\":[]}\n"
	if stdout != want || len(operations.calls) != 1 || operations.calls[0].name != "leave" {
		t.Fatalf("stdout=%q calls=%#v", stdout, operations.calls)
	}
}

func TestRaidLeaveWithoutVerificationPreservesUnknownWriteOutcome(t *testing.T) {
	operations := &stubRaidOperations{err: output.NewError(output.CodeWriteOutcomeUnknown, "secret uncertain write")}
	stdout, err := runPrimitiveCommand(t, operations, "token\n", "raid", "leave", "--raid-id", "raid-9", "--verify=false")
	if output.ExitCodeFor(err) != output.ExitWriteOutcomeUnknown || strings.Contains(err.Error(), "secret") || stdout != "" {
		t.Fatalf("exit=%d stdout=%q err=%v", output.ExitCodeFor(err), stdout, err)
	}
	if len(operations.calls) != 1 || operations.calls[0].name != "leave" {
		t.Fatalf("calls=%#v", operations.calls)
	}
}

func TestRaidLeaveDefaultVerificationPreservesJSONOutput(t *testing.T) {
	operations := &stubRaidOperations{}
	stdout, err := runPrimitiveCommand(t, operations, "token\n", "--json", "raid", "leave", "--raid-id", "raid-9")
	if err != nil {
		t.Fatal(err)
	}
	want := "{\"schemaVersion\":1,\"ok\":true,\"command\":\"raid leave\",\"status\":\"left\",\"data\":{\"left\":true,\"raidId\":\"raid-9\"},\"warnings\":[]}\n"
	if stdout != want || len(operations.calls) != 2 || operations.calls[0].name != "leave" || operations.calls[1].name != "find" {
		t.Fatalf("stdout=%q calls=%#v", stdout, operations.calls)
	}
}

func TestRaidLeaveVerifiesAbsenceAndDoesNotVerifyUncertainWrite(t *testing.T) {
	operations := &stubRaidOperations{}
	stdout, err := runPrimitiveCommand(t, operations, "token\n", "raid", "leave", "--raid-id", "raid-9")
	if err != nil || stdout != "Raiden lämnades.\n" || len(operations.calls) != 2 || operations.calls[0].name != "leave" || operations.calls[1].name != "find" {
		t.Fatalf("stdout=%q err=%v calls=%#v", stdout, err, operations.calls)
	}

	operations = &stubRaidOperations{err: output.NewError(output.CodeWriteOutcomeUnknown, "secret")}
	_, err = runPrimitiveCommand(t, operations, "token\n", "raid", "leave", "--raid-id", "raid-9")
	if output.ExitCodeFor(err) != output.ExitWriteOutcomeUnknown || len(operations.calls) != 1 || operations.calls[0].name != "leave" {
		t.Fatalf("exit=%d calls=%#v", output.ExitCodeFor(err), operations.calls)
	}

	raid := &domain.Raid{ID: "raid-9"}
	operations = &stubRaidOperations{found: raid}
	_, err = runPrimitiveCommand(t, operations, "token\n", "raid", "leave", "--raid-id", "raid-9")
	if output.ExitCodeFor(err) != output.ExitLifecycleConflict || len(operations.calls) != 2 {
		t.Fatalf("exit=%d calls=%#v", output.ExitCodeFor(err), operations.calls)
	}
}

func TestPrimitiveCommandsValidateBeforeCallingOperations(t *testing.T) {
	for _, args := range [][]string{
		{"queue", "status", "--rank", "5"},
		{"queue", "check"},
		{"queue", "cancel"},
		{"lobbies", "list", "--page", "0"},
		{"lobbies", "list", "--host-level", "41"},
		{"raid", "join", "--raid-id", "raid", "--trainer-name", "A!", "--trainer-level", "50", "--trainer-country", "se"},
		{"raid", "get"},
		{"raid", "players"},
		{"raid", "status"},
		{"raid", "leave"},
	} {
		operations := &stubRaidOperations{}
		_, err := runPrimitiveCommand(t, operations, "token\n", args...)
		if output.ExitCodeFor(err) == output.ExitSuccess || len(operations.calls) != 0 {
			t.Fatalf("args=%v exit=%d calls=%#v", args, output.ExitCodeFor(err), operations.calls)
		}
	}
}
