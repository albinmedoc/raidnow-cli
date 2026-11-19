package upstream

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/output"
)

func TestPollMatchingPostsOrderedSelectionAndFailsClosed(t *testing.T) {
	responses := []string{
		`{"status":"success","rpllm_status":1}`,
		`{"status":"success","rpllm_status":"2","rpllm_rpl_id":"raid-7"}`,
		`{"status":"success","rpllm_status":2,"rpllm_rpl_id":""}`,
		`{"status":"success","rpllm_status":3}`,
	}
	var forms []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		forms = append(forms, request.PostForm)
		_, _ = response.Write([]byte(responses[len(forms)-1]))
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
	selection := QueueSelection{Ranks: []string{"5", "5", "5"}, PokemonIDs: []string{"boss-a", "boss-b", "boss-a"}, GuestEvalStar: "4"}

	waiting, err := client.PollMatching(context.Background(), "token", selection, CallOptions{})
	if err != nil || waiting.State != domain.QueueWaiting || waiting.RaidID != "" {
		t.Fatalf("waiting=%#v err=%v", waiting, err)
	}
	matched, err := client.PollMatching(context.Background(), "token", selection, CallOptions{})
	if err != nil || matched.State != domain.QueueMatched || matched.RaidID != "raid-7" {
		t.Fatalf("matched=%#v err=%v", matched, err)
	}
	for range 2 {
		_, err = client.PollMatching(context.Background(), "token", selection, CallOptions{})
		assertExitCode(t, err, output.ExitUpstreamRejection)
	}
	assertExactForm(t, forms[0], url.Values{"type": {"line_matching_list"}, "rank[]": {"5", "5"}, "pokemon[]": {"boss-a", "boss-b"}, "guestevalstar": {"4"}, "token": {"token"}})
}

func TestPollMatchingRejectsMismatchedSelectionBeforeNetwork(t *testing.T) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
	_, err := client.PollMatching(context.Background(), "token", QueueSelection{Ranks: []string{"5"}, PokemonIDs: []string{"a", "b"}}, CallOptions{})
	assertExitCode(t, err, output.ExitInvalidArguments)
	if called {
		t.Fatal("mismatched selection reached network")
	}
}

func TestCheckMatchingNormalizesPresencePriorityAndWaiting(t *testing.T) {
	var captured url.Values
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		captured = request.PostForm
		_, _ = response.Write([]byte(`{"status":"success","id":"queue-1","priority":"2","list":[{"id":"wait-1","rank":"5","pokemon":"boss-a"},"wait-2"]}`))
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
	got, err := client.CheckMatching(context.Background(), "token", []string{"boss-a", "boss-b", "boss-a"}, CallOptions{})
	if err != nil {
		t.Fatalf("check matching: %v", err)
	}
	priority := 2
	want := domain.QueueCheck{Present: true, ID: "queue-1", Priority: &priority, Waiting: []domain.QueueWaitingEntry{{ID: "wait-1", Rank: "5", PokemonID: "boss-a"}, {ID: "wait-2"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("check=%#v want=%#v", got, want)
	}
	assertExactForm(t, captured, url.Values{"type": {"line_matching_check"}, "pokemon[]": {"boss-a", "boss-b"}, "token": {"token"}})
}

func TestCheckMatchingTreatsEmptyIDAsAbsentAndRejectsMalformedShape(t *testing.T) {
	for _, testCase := range []struct {
		body     string
		wantExit int
	}{
		{body: `{"status":"success","id":"","priority":null,"list":null}`},
		{body: `[]`, wantExit: output.ExitUpstreamRejection},
		{body: `{"status":"success","id":{},"list":[]}`, wantExit: output.ExitUpstreamRejection},
		{body: `{"status":"success","priority":"bad","list":[]}`, wantExit: output.ExitUpstreamRejection},
		{body: `{"status":"success","list":[null]}`, wantExit: output.ExitUpstreamRejection},
		{body: `{"status":"success","list":{}}`, wantExit: output.ExitUpstreamRejection},
	} {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { _, _ = response.Write([]byte(testCase.body)) }))
		client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
		got, err := client.CheckMatching(context.Background(), "token", []string{"boss"}, CallOptions{})
		server.Close()
		if testCase.wantExit != 0 {
			assertExitCode(t, err, testCase.wantExit)
		} else if err != nil || got.Present || got.Priority != nil || len(got.Waiting) != 0 {
			t.Fatalf("check=%#v err=%v", got, err)
		}
	}
}

func TestFindRaidPostsExactFormAndDistinguishesAbsenceFromMalformed(t *testing.T) {
	var forms []url.Values
	responses := []string{
		`[{"rpl_id":"other"},{"rpl_id":"raid-2","rpl_pokemon":"boss"}]`,
		`{"status":"success","list":[]}`,
		`{"list":[{"unexpected":"shape"}]}`,
	}
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		forms = append(forms, request.PostForm)
		_, _ = response.Write([]byte(responses[len(forms)-1]))
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})

	raid, err := client.FindRaid(context.Background(), "token", "raid-2", CallOptions{})
	if err != nil || raid == nil || raid.ID != "raid-2" || raid.PokemonID != "boss" {
		t.Fatalf("raid=%#v err=%v", raid, err)
	}
	missing, err := client.FindRaid(context.Background(), "token", "raid-2", CallOptions{})
	if err != nil || missing != nil {
		t.Fatalf("missing=%#v err=%v", missing, err)
	}
	_, err = client.FindRaid(context.Background(), "token", "raid-2", CallOptions{})
	assertExitCode(t, err, output.ExitUpstreamRejection)
	assertExactForm(t, forms[0], url.Values{"type": {"par_find_first"}, "possible": {"false"}, "firstaccess": {"raid-2"}, "lang": {"1"}, "token": {"token"}, "hostlevel": {"0"}})
}

func TestListRaidPlayersNormalizesCountryRatingsAndCounts(t *testing.T) {
	fixture, err := os.ReadFile("testdata/child_list_mixed.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	var captured url.Values
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		captured = request.PostForm
		_, _ = response.Write(fixture)
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
	got, err := client.ListRaidPlayers(context.Background(), "token", "raid-1", CallOptions{})
	if err != nil {
		t.Fatalf("list players: %v", err)
	}
	if got.Count != 14 || got.ReadyCount != 2 || !got.RaidDeleted {
		t.Fatalf("summary=%#v", got)
	}
	countries := []string{"se", "jp", "world", "", "", "", "world", "jp", "", "", "", "", "", ""}
	ratings := []*int{intPointer(5), intPointer(0), intPointer(4), nil, nil, nil, nil, nil, nil, intPointer(1), intPointer(1), intPointer(2), nil, nil}
	for index := range got.Players {
		if got.Players[index].Country != countries[index] || !reflect.DeepEqual(got.Players[index].Rating, ratings[index]) {
			t.Fatalf("player %d=%#v", index, got.Players[index])
		}
	}
	if !got.Players[0].Ready || !got.Players[0].IsMe || got.Players[0].Level != 50 {
		t.Fatalf("first player=%#v", got.Players[0])
	}
	assertExactForm(t, captured, url.Values{"type": {"child_list"}, "parid": {"raid-1"}, "lang": {"1"}, "token": {"token"}})
}

func TestListRaidPlayersRejectsMalformedListAndPlayerShapes(t *testing.T) {
	for index, body := range []string{
		`[]`,
		`{"list":null}`,
		`{"list":["player"]}`,
		`{"list":[{"rplc_tid":{}}]}`,
		`{"list":[{"rplc_tid":"name","rplc_country_code":{},"rplc_lang":0}]}`,
		`{"list":[],"count":"1.5"}`,
		`{"list":[],"readycount":" "}`,
	} {
		t.Run(fmt.Sprint(index), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { _, _ = response.Write([]byte(body)) }))
			client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
			_, err := client.ListRaidPlayers(context.Background(), "token", "raid", CallOptions{})
			server.Close()
			assertExitCode(t, err, output.ExitUpstreamRejection)
			assertNoSecret(t, err)
		})
	}
}

func TestReadOperationsClassifyStructuredStatusAndTransportFailures(t *testing.T) {
	for _, body := range []string{
		`{"status":"error","text":"The session has expired. Please restart. secret-marker"}`,
		`{"status":"error","text":"Restart the browser. secret-marker"}`,
		`{"status":"login_required","text":"ignored secret-marker"}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { _, _ = response.Write([]byte(body)) }))
		client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
		_, err := client.ListRaids(context.Background(), "token", LobbyQuery{Page: 1}, CallOptions{})
		server.Close()
		if strings.Contains(body, "expired") || strings.Contains(body, "Restart") {
			assertExitCode(t, err, output.ExitAuthenticationUnsupported)
		} else {
			assertExitCode(t, err, output.ExitUpstreamRejection)
		}
		assertNoSecret(t, err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.WriteHeader(http.StatusOK)
		response.(http.Flusher).Flush()
		<-request.Context().Done()
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
	_, err := client.ListRaidPlayers(context.Background(), "token", "raid", CallOptions{Timeout: time.Millisecond})
	assertExitCode(t, err, output.ExitReadNetwork)
}

func TestReadOperationsAreStatelessAcrossConcurrentTokens(t *testing.T) {
	var mutex sync.Mutex
	seen := make(map[string]string)
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		token := request.PostForm.Get("token")
		mutex.Lock()
		seen[token] = request.PostForm.Get("type")
		mutex.Unlock()
		_, _ = fmt.Fprintf(response, `{"list":[{"rank":"5","pokemon":%q}]}`, token)
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})

	var wait sync.WaitGroup
	for _, token := range []string{"token-a", "token-b"} {
		wait.Add(1)
		go func(token string) {
			defer wait.Done()
			bosses, err := client.ListCurrentRaidBosses(context.Background(), token, CallOptions{})
			if err != nil || len(bosses) != 1 || bosses[0].PokemonID != token {
				t.Errorf("token %q bosses=%#v err=%v", token, bosses, err)
			}
		}(token)
	}
	wait.Wait()
	if !reflect.DeepEqual(seen, map[string]string{"token-a": "line_matching_get_list", "token-b": "line_matching_get_list"}) {
		t.Fatalf("seen=%#v", seen)
	}
}

func intPointer(value int) *int { return &value }
