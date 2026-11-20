package upstream

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/output"
)

func TestListCurrentRaidBossesPostsTokenAndNormalizesEntries(t *testing.T) {
	const token = "anonymous-secret-marker"
	var captured url.Values
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if err := request.ParseForm(); err != nil {
			t.Errorf("parse form: %v", err)
			return
		}
		captured = request.PostForm
		_, _ = response.Write([]byte(`{"status":"success","list":[{"rank":"1","pokemon":"egg","offpokemon":"8"},{"rank":"5","pokemon":"ギラティナ(オリジン)","offpokemon":"14"},{"rank":"5","pokemon":"ギラティナ(オリジン)","offpokemon":"99"},{"rank":11,"pokemon":"キョダイマックスリザードン","offpokemon":-2},{"rank":"4","pokemon":"欠落"},{"rank":"5","pokemon":"bad\u0001"},{"pokemon":"missing-rank"},null,"bad"]}`))
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})

	got, err := client.ListCurrentRaidBosses(context.Background(), token, CallOptions{})
	if err != nil {
		t.Fatalf("list bosses: %v", err)
	}
	want := []domain.RaidBoss{
		{Rank: "5", PokemonID: "ギラティナ(オリジン)", WaitingCount: 14},
		{Rank: "11", PokemonID: "キョダイマックスリザードン", WaitingCount: 0},
		{Rank: "4", PokemonID: "欠落", WaitingCount: 0},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("bosses = %#v, want %#v", got, want)
	}
	assertExactForm(t, captured, url.Values{
		"type":  {"line_matching_get_list"},
		"token": {token},
	})
}

func TestListRaidsPostsExactQueryAndNormalizesObjectList(t *testing.T) {
	const token = "lobby-token-secret-marker"
	var captured url.Values
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		captured = request.PostForm
		_, _ = response.Write([]byte(`{"status":"success","list":[{"rpl_id":"raid-1","rpl_rank":"5","rpl_pokemon":"ギラティナ(オリジン)","rpl_tid":"HostName","rpl_level":"50","rpl_country_code":"se","rpl_weather":1,"rpl_maxuser":"10","cnt":3,"readycnt":"2","join":1}]}`))
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})

	got, err := client.ListRaids(context.Background(), token, LobbyQuery{
		Filters: []string{"egg", "ザシアン", "egg"}, Page: 2, SearchRank: "5", HostLevel: 50, HostEvaluate: []int{-1, 0, 2, 0},
	}, CallOptions{})
	if err != nil {
		t.Fatalf("list raids: %v", err)
	}
	join := 1
	want := []domain.Raid{{ID: "raid-1", Rank: "5", PokemonID: "ギラティナ(オリジン)", HostName: "HostName", HostLevel: 50, Country: "se", Weather: 1, MaxUsers: 10, Count: 3, ReadyCount: 2, JoinState: &join}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("raids = %#v, want %#v", got, want)
	}
	assertExactForm(t, captured, url.Values{
		"type": {"par_list"}, "possible": {"false"}, "filter[]": {"egg", "ザシアン"}, "page": {"2"}, "firstaccess": {"0"},
		"searchrank": {"5"}, "hostevalstar": {""}, "guestevalstar": {""}, "lang": {"1"}, "token": {token}, "hostlevel": {"50"}, "hostevaluate[]": {"-1", "0", "2"},
	})
}

func TestListRaidsAcceptsFullCurrentBossCatalogAsExclusions(t *testing.T) {
	var captured url.Values
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		captured = request.PostForm
		_, _ = response.Write([]byte(`{"status":"success","list":[]}`))
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
	filters := make([]string, 77)
	for index := range filters {
		filters[index] = fmt.Sprintf("boss-%d", index)
	}

	_, err := client.ListRaids(context.Background(), "token", LobbyQuery{Page: 1, Filters: filters}, CallOptions{})
	if err != nil {
		t.Fatalf("list raids with current catalog: %v", err)
	}
	if got := captured["filter[]"]; !reflect.DeepEqual(got, filters) {
		t.Fatalf("filters = %#v, want %#v", got, filters)
	}
}

func TestListRaidsNormalizesLiteSelectionFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`[
			{"rpl_id":"numeric","rpl_deleted":1,"my":0,"rpl_glevel":40},
			{"rpl_id":"string","rpl_deleted":"0","my":"1","rpl_glevel":"50"},
			{"rpl_id":"missing"}
		]`))
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})

	got, err := client.ListRaids(context.Background(), "token", LobbyQuery{Page: 1}, CallOptions{})
	if err != nil {
		t.Fatalf("list raids: %v", err)
	}
	want := []domain.Raid{
		{ID: "numeric", Deleted: true, MinimumGuestLevel: 40},
		{ID: "string", IsMe: true, MinimumGuestLevel: 50},
		{ID: "missing"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("raids = %#v, want %#v", got, want)
	}
}

func TestListRaidsRejectsMalformedLiteSelectionFields(t *testing.T) {
	for _, testCase := range []struct {
		name  string
		field string
	}{
		{name: "deleted negative", field: `"rpl_deleted":-1`},
		{name: "deleted above range", field: `"rpl_deleted":2`},
		{name: "deleted boolean", field: `"rpl_deleted":true`},
		{name: "deleted blank", field: `"rpl_deleted":""`},
		{name: "deleted padded", field: `"rpl_deleted":" 1 "`},
		{name: "deleted null", field: `"rpl_deleted":null`},
		{name: "is me negative", field: `"my":-1`},
		{name: "is me above range", field: `"my":"2"`},
		{name: "is me object", field: `"my":{}`},
		{name: "minimum level null", field: `"rpl_glevel":null`},
		{name: "minimum level negative", field: `"rpl_glevel":-1`},
		{name: "minimum level fractional", field: `"rpl_glevel":"1.5"`},
		{name: "minimum level blank", field: `"rpl_glevel":""`},
		{name: "minimum level boolean", field: `"rpl_glevel":false`},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			body := `[{"rpl_id":"raid",` + testCase.field + `}]`
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				_, _ = response.Write([]byte(body))
			}))
			client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
			_, err := client.ListRaids(context.Background(), "token", LobbyQuery{Page: 1}, CallOptions{})
			server.Close()
			assertExitCode(t, err, output.ExitUpstreamRejection)
		})
	}
}

func TestListRaidsAcceptsTopLevelArrayAndRejectsMalformedEntries(t *testing.T) {
	for _, testCase := range []struct {
		name     string
		body     string
		wantExit int
	}{
		{name: "array", body: `[{"rpl_id":9,"rpl_pokemon":"boss"}]`},
		{name: "missing list", body: `{}`, wantExit: output.ExitUpstreamRejection},
		{name: "wrong list", body: `{"list":{}}`, wantExit: output.ExitUpstreamRejection},
		{name: "entry missing id", body: `[{"rpl_pokemon":"boss"}]`, wantExit: output.ExitUpstreamRejection},
		{name: "fractional id", body: `[{"rpl_id":1.5}]`, wantExit: output.ExitUpstreamRejection},
		{name: "malformed numeric field", body: `[{"rpl_id":"raid","cnt":"1.5"}]`, wantExit: output.ExitUpstreamRejection},
		{name: "non object", body: `["raid"]`, wantExit: output.ExitUpstreamRejection},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { _, _ = response.Write([]byte(testCase.body)) }))
			defer server.Close()
			client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
			raids, err := client.ListRaids(context.Background(), "token", LobbyQuery{Page: 1}, CallOptions{})
			if testCase.wantExit != 0 {
				assertExitCode(t, err, testCase.wantExit)
				return
			}
			if err != nil || len(raids) != 1 || raids[0].ID != "9" {
				t.Fatalf("raids=%#v err=%v", raids, err)
			}
		})
	}
}

func TestListRaidsRejectsInvalidQueryBeforeNetwork(t *testing.T) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
	oversized := make([]string, maxLobbyFilters+1)
	for index := range oversized {
		oversized[index] = "boss"
	}
	for _, query := range []LobbyQuery{{Page: 0}, {Page: 1, HostLevel: 5}, {Page: 1, HostEvaluate: []int{6}}, {Page: 1, Filters: []string{"bad\u200dvalue"}}, {Page: 1, Filters: []string{""}}, {Page: 1, Filters: oversized}} {
		_, err := client.ListRaids(context.Background(), "token", query, CallOptions{})
		assertExitCode(t, err, output.ExitInvalidArguments)
	}
	if called {
		t.Fatal("invalid lobby query reached network")
	}
}

func TestListCurrentRaidBossesRejectsInvalidTokenBeforeNetwork(t *testing.T) {
	var called bool
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})

	_, err := client.ListCurrentRaidBosses(context.Background(), "bad\nsecret-marker", CallOptions{})
	assertExitCode(t, err, output.ExitInvalidArguments)
	assertNoSecret(t, err)
	if called {
		t.Fatal("invalid token reached network")
	}
}

func assertExactForm(t *testing.T, got, want url.Values) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("form = %#v, want %#v", got, want)
	}
}
