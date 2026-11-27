package upstream

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/output"
)

func TestPrimitiveWritesPostExactForms(t *testing.T) {
	trainer, _ := domain.NewTrainer("Ash123", "123456789012", 50, "se")
	identity, _ := domain.NewTrainerIdentity("Misty7", 44, "JP")
	selection := QueueSelection{Ranks: []string{"5", "mega"}, PokemonIDs: []string{"150", "6_mega_x"}, GuestEvalStar: "4", MinimumHostRating: 3}
	hostLobby := HostLobbyInput{
		StartDate: "2026/11/27 16:23:35", Rank: "1", PokemonID: "ピカチュウ(太陽の冠)", Span: 45,
		Color: 0, Weather: 0, CurrentWeather: 5, MaxUsers: 4, GymColor: 2, PassType: 0,
		Private: false, Filters: []string{"シャドウマクノシタ", "ヒトモシ"}, Possible: false,
		GuestLevel: 0, GuestScore: 0, InvitationOnly: false, Hot: true, MegaPokemonEntry: false,
		WeatherBoosts: []string{"rain"},
	}
	tests := []struct {
		name string
		want url.Values
		call func(*Client) error
	}{
		{
			name: "create matching",
			want: url.Values{"type": {"line_matching_create"}, "rank[]": {"5", "mega"}, "pokemon[]": {"150", "6_mega_x"}, "code": {"123456789012"}, "tid": {"Ash123"}, "level": {"50"}, "priority": {"0"}, "lang": {"1"}, "gscore": {"4"}, "country_code": {"se"}, "plugs": {"[]"}, "host_country": {"0"}, "weather_boost": {"0"}, "guest6": {"0"}, "regional_raid": {"0"}, "token": {"opaque-token"}, "hosteval[]": {"-1", "0", "1", "2"}, "hosttl": {"0"}},
			call: func(client *Client) error {
				return client.CreateMatching(context.Background(), "opaque-token", trainer, selection, CallOptions{})
			},
		},
		{
			name: "cancel matching",
			want: url.Values{"type": {"line_matching_cancel"}, "lang": {"1"}, "token": {"opaque-token"}},
			call: func(client *Client) error {
				return client.CancelMatching(context.Background(), "opaque-token", CallOptions{})
			},
		},
		{
			name: "join raid",
			want: url.Values{"type": {"child_post_fast"}, "parid": {"raid-9"}, "tid": {"Misty7"}, "show": {"0"}, "level": {"44"}, "color": {"0"}, "memo": {""}, "lang": {"1"}, "country_code": {"jp"}, "regional_raid": {"0"}, "token": {"opaque-token"}},
			call: func(client *Client) error {
				return client.JoinRaid(context.Background(), "opaque-token", "raid-9", identity, CallOptions{})
			},
		},
		{
			name: "leave raid",
			want: url.Values{"type": {"guest_raid_del_post"}, "edit": {"raid-9"}, "lang": {"1"}, "token": {"opaque-token"}},
			call: func(client *Client) error {
				return client.LeaveRaid(context.Background(), "opaque-token", "raid-9", CallOptions{})
			},
		},
		{
			name: "ready raid",
			want: url.Values{"type": {"guest_ready_post"}, "edit": {"raid-9"}, "ready": {"1"}, "lang": {"1"}, "token": {"opaque-token"}},
			call: func(client *Client) error {
				return client.ReadyRaid(context.Background(), "opaque-token", "raid-9", CallOptions{})
			},
		},
		{
			name: "create hosted raid",
			want: url.Values{
				"type": {"par_post"}, "start_date": {"2026/11/27 16:23:35"}, "rank": {"1"}, "pokemon": {"ピカチュウ(太陽の冠)"},
				"span": {"45"}, "code": {"123456789012"}, "level": {"50"}, "color": {"0"}, "tid": {"Ash123"},
				"weather": {"0"}, "current_weather": {"5"}, "maxuser": {"4"}, "gpcolor": {"2"}, "passtype": {"0"},
				"private": {"0"}, "filter[]": {"シャドウマクノシタ", "ヒトモシ"}, "possible": {"false"}, "lang": {"1"},
				"country_code": {"se"}, "glevel": {"0"}, "gescore": {"0"}, "invitation_only": {"0"}, "hot": {"1"},
				"mega_pokemon_entry": {"0"}, "weather_boost[]": {"rain"}, "regional_raid": {"0"}, "token": {"opaque-token"},
			},
			call: func(client *Client) error {
				created, err := client.CreateHostedRaid(context.Background(), "opaque-token", trainer, hostLobby, CallOptions{})
				if err == nil && created.ID != "raid-9" {
					return fmt.Errorf("created raid ID=%q", created.ID)
				}
				return err
			},
		},
		{
			name: "close hosted raid",
			want: url.Values{"type": {"par_delete"}, "parid": {"raid-9"}, "token": {"opaque-token"}},
			call: func(client *Client) error {
				return client.CloseHostedRaid(context.Background(), "opaque-token", "raid-9", CallOptions{})
			},
		},
		{
			name: "start hosted raid",
			want: url.Values{"type": {"host_ready_post"}, "edit": {"raid-9"}, "lang": {"1"}, "token": {"opaque-token"}},
			call: func(client *Client) error {
				return client.StartHostedRaid(context.Background(), "opaque-token", "raid-9", CallOptions{})
			},
		},
		{
			name: "kick hosted guest",
			want: url.Values{"type": {"host_kicks_guest_post"}, "edit": {"raid-9"}, "kick": {"guest-7"}, "lang": {"1"}, "token": {"opaque-token"}},
			call: func(client *Client) error {
				return client.KickHostedGuest(context.Background(), "opaque-token", "raid-9", "guest-7", CallOptions{})
			},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var got url.Values
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				_ = request.ParseForm()
				got = request.PostForm
				if testCase.name == "create hosted raid" {
					_, _ = response.Write([]byte(`{"status":"success","list":[{"rpl_id":"raid-1"},{"rpl_id":"raid-9"}],"now":1}`))
					return
				}
				_, _ = response.Write([]byte(`{"status":"success"}`))
			}))
			defer server.Close()
			client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
			if err := testCase.call(client); err != nil {
				t.Fatalf("call: %v", err)
			}
			if !reflect.DeepEqual(got, testCase.want) {
				t.Fatalf("form=%#v want=%#v", got, testCase.want)
			}
		})
	}
}

func TestCreateMatchingPostsExplicitFilterEdgesInOrder(t *testing.T) {
	trainer, _ := domain.NewTrainer("Ash123", "123456789012", 50, "se")
	tests := []struct {
		name            string
		selection       QueueSelection
		wantHostCountry string
		wantWeather     string
		wantHostLevel   string
		wantHostEval    []string
	}{
		{
			name:            "omit host rating",
			selection:       QueueSelection{Ranks: []string{"5"}, PokemonIDs: []string{"150"}, MinimumHostRating: -1, HostCountry: 1, WeatherBoost: 2, HostLevel: 70},
			wantHostCountry: "1",
			wantWeather:     "2",
			wantHostLevel:   "70",
		},
		{
			name:            "zero host rating",
			selection:       QueueSelection{Ranks: []string{"5"}, PokemonIDs: []string{"150"}, MinimumHostRating: 0},
			wantHostCountry: "0",
			wantWeather:     "0",
			wantHostLevel:   "0",
			wantHostEval:    []string{"-1"},
		},
		{
			name:            "maximum host rating",
			selection:       QueueSelection{Ranks: []string{"5"}, PokemonIDs: []string{"150"}, MinimumHostRating: 5},
			wantHostCountry: "0",
			wantWeather:     "0",
			wantHostLevel:   "0",
			wantHostEval:    []string{"-1", "0", "1", "2", "3", "4"},
		},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var got url.Values
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				_ = request.ParseForm()
				got = request.PostForm
				_, _ = response.Write([]byte(`{"status":"success"}`))
			}))
			defer server.Close()
			client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
			if err := client.CreateMatching(context.Background(), "opaque-token", trainer, testCase.selection, CallOptions{}); err != nil {
				t.Fatalf("create matching: %v", err)
			}
			want := url.Values{
				"type": {"line_matching_create"}, "rank[]": {"5"}, "pokemon[]": {"150"}, "code": {"123456789012"},
				"tid": {"Ash123"}, "level": {"50"}, "priority": {"0"}, "lang": {"1"}, "gscore": {""},
				"country_code": {"se"}, "plugs": {"[]"}, "host_country": {testCase.wantHostCountry}, "weather_boost": {testCase.wantWeather},
				"guest6": {"0"}, "regional_raid": {"0"}, "token": {"opaque-token"}, "hosttl": {testCase.wantHostLevel},
			}
			if testCase.wantHostEval != nil {
				want["hosteval[]"] = testCase.wantHostEval
			}
			if !reflect.DeepEqual(got, want) {
				t.Fatalf("form=%#v want=%#v", got, want)
			}
		})
	}
}

func TestHostedRaidCloseAcceptsSuccessfulEmptyOrTextResponse(t *testing.T) {
	for _, testCase := range []struct {
		status int
		body   string
	}{
		{status: http.StatusOK, body: ""},
		{status: http.StatusOK, body: "deleted"},
		{status: http.StatusNoContent, body: ""},
	} {
		t.Run(fmt.Sprintf("status=%d/body=%q", testCase.status, testCase.body), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				response.WriteHeader(testCase.status)
				_, _ = response.Write([]byte(testCase.body))
			}))
			defer server.Close()
			client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
			if err := client.CloseHostedRaid(context.Background(), "opaque-token", "raid-9", CallOptions{}); err != nil {
				t.Fatalf("close hosted raid: %v", err)
			}
		})
	}
}

func TestHostedRaidCreateRequiresExplicitSuccessAndPreservesSafeRejection(t *testing.T) {
	trainer, _ := domain.NewTrainer("Ash123", "123456789012", 50, "se")
	input := HostLobbyInput{StartDate: "2026/11/27 16:23:35", Rank: "1", PokemonID: "25", Span: 45, MaxUsers: 5}
	for _, testCase := range []struct {
		name string
		body string
		text string
	}{
		{name: "missing status", body: `{"list":[{"rpl_id":"raid-9"}],"now":0}`},
		{name: "rejection detail", body: `{"status":"error","text":"Invalid lobby for opaque-token and 123456789012."}`, text: "Invalid lobby"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
				_, _ = response.Write([]byte(testCase.body))
			}))
			defer server.Close()
			client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
			_, err := client.CreateHostedRaid(context.Background(), "opaque-token", trainer, input, CallOptions{})
			assertExitCode(t, err, output.ExitUpstreamRejection)
			message := err.Error()
			if testCase.text != "" && !strings.Contains(message, testCase.text) {
				t.Fatalf("error=%q missing %q", message, testCase.text)
			}
			if strings.Contains(message, "opaque-token") || strings.Contains(message, "123456789012") {
				t.Fatalf("error exposed credentials: %q", message)
			}
		})
	}
}

func TestPrimitiveWriteRejectionPreservesUsefulTextWithoutCredentials(t *testing.T) {
	trainer, _ := domain.NewTrainer("Ash123", "123456789012", 50, "se")
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"status":"error","text":"Lobby is full for opaque-token and 1234 5678 9012."}`))
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
	err := client.CreateMatching(context.Background(), "opaque-token", trainer, QueueSelection{Ranks: []string{"5"}, PokemonIDs: []string{"150"}}, CallOptions{})
	assertExitCode(t, err, output.ExitUpstreamRejection)
	message := err.Error()
	if !strings.Contains(message, "Lobby is full") {
		t.Fatalf("error omitted useful upstream detail: %q", message)
	}
	for _, secret := range []string{"opaque-token", "123456789012", "1234 5678 9012"} {
		if strings.Contains(message, secret) {
			t.Fatalf("error exposed credential %q: %q", secret, message)
		}
	}
}

func TestReadyRaidRejectionPreservesUsefulTextWithoutToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		_, _ = response.Write([]byte(`{"status":"error","text":"Raid already started for opaque-token."}`))
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
	err := client.ReadyRaid(context.Background(), "opaque-token", "raid-9", CallOptions{})
	assertExitCode(t, err, output.ExitUpstreamRejection)
	if message := err.Error(); !strings.Contains(message, "Raid already started") || strings.Contains(message, "opaque-token") {
		t.Fatalf("error=%q", message)
	}
}

func TestUpstreamRejectionRedactsCredentialsBeforeTruncation(t *testing.T) {
	prefix := strings.Repeat("a", 250)
	for _, testCase := range []struct {
		name    string
		secret  string
		marker  string
		secrets []string
	}{
		{name: "token", secret: "opaque-token", marker: "opaque", secrets: []string{"opaque-token"}},
		{name: "trainer code", secret: "123456789012", marker: "123456", secrets: []string{"123456789012"}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			message := upstreamRejectionDetail(prefix+testCase.secret, testCase.secrets...).Error()
			if strings.Contains(message, testCase.marker) {
				t.Fatalf("truncation leaked credential prefix %q: %q", testCase.marker, message)
			}
		})
	}
}

func TestPrimitiveWritesRequireValidAcknowledgement(t *testing.T) {
	trainer, _ := domain.NewTrainer("Ash123", "123456789012", 50, "se")
	for _, body := range []string{"", `{}`, `{"status":"error"}`, `[]`, `not-json`} {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) { _, _ = response.Write([]byte(body)) }))
		client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
		err := client.CreateMatching(context.Background(), "token", trainer, QueueSelection{Ranks: []string{"5"}, PokemonIDs: []string{"150"}}, CallOptions{})
		server.Close()
		assertExitCode(t, err, output.ExitUpstreamRejection)
	}
}

func TestPrimitiveWritesRejectInvalidInputsBeforeNetwork(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls.Add(1) }))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
	trainer, _ := domain.NewTrainer("Ash123", "123456789012", 50, "se")
	identity, _ := domain.NewTrainerIdentity("Ash123", 50, "se")
	checks := []error{
		client.CreateMatching(nil, "token", trainer, QueueSelection{Ranks: []string{"5"}, PokemonIDs: []string{"150"}}, CallOptions{}),
		client.CreateMatching(context.Background(), "token", trainer, QueueSelection{Ranks: []string{"5"}, PokemonIDs: nil}, CallOptions{}),
		client.CreateMatching(context.Background(), "token", domain.Trainer{}, QueueSelection{Ranks: []string{"5"}, PokemonIDs: []string{"150"}}, CallOptions{}),
		client.CancelMatching(context.Background(), "\n", CallOptions{}),
		client.JoinRaid(context.Background(), "token", "", identity, CallOptions{}),
		client.JoinRaid(context.Background(), "token", "raid", domain.TrainerIdentity{}, CallOptions{}),
		client.LeaveRaid(context.Background(), "token", "\n", CallOptions{}),
		client.LeaveRaid(context.Background(), "token", "raid", CallOptions{Timeout: -time.Second}),
		client.ReadyRaid(context.Background(), "token", "\n", CallOptions{}),
		client.ReadyRaid(context.Background(), "token", "raid", CallOptions{Timeout: -time.Second}),
		func() error {
			_, err := client.CreateHostedRaid(context.Background(), "token", trainer, HostLobbyInput{}, CallOptions{})
			return err
		}(),
		client.CloseHostedRaid(context.Background(), "token", "\n", CallOptions{}),
		client.StartHostedRaid(context.Background(), "token", "\n", CallOptions{}),
		client.KickHostedGuest(context.Background(), "token", "raid", "\n", CallOptions{}),
	}
	for _, selection := range []QueueSelection{
		{Ranks: []string{"5"}, PokemonIDs: []string{"150"}, MinimumHostRating: -2},
		{Ranks: []string{"5"}, PokemonIDs: []string{"150"}, MinimumHostRating: 6},
		{Ranks: []string{"5"}, PokemonIDs: []string{"150"}, HostCountry: -1},
		{Ranks: []string{"5"}, PokemonIDs: []string{"150"}, HostCountry: 2},
		{Ranks: []string{"5"}, PokemonIDs: []string{"150"}, WeatherBoost: -1},
		{Ranks: []string{"5"}, PokemonIDs: []string{"150"}, WeatherBoost: 3},
		{Ranks: []string{"5"}, PokemonIDs: []string{"150"}, HostLevel: 41},
	} {
		checks = append(checks, client.CreateMatching(context.Background(), "token", trainer, selection, CallOptions{}))
	}
	for _, input := range []HostLobbyInput{
		{StartDate: "2026/11/27 16:23:35", Rank: "1", PokemonID: "25", Span: 45, MaxUsers: 5, CurrentWeather: 10},
		{StartDate: "2026/11/27 16:23:35", Rank: "1", PokemonID: "25", Span: 45, MaxUsers: 5, GymColor: 4},
		{StartDate: "2026/11/27 16:23:35", Rank: "1", PokemonID: "25", Span: 45, MaxUsers: 5, PassType: 2},
		{StartDate: "2026/11/27 16:23:35", Rank: "1", PokemonID: "25", Span: 45, MaxUsers: 5, GuestLevel: 41},
		{StartDate: "2026/11/27 16:23:35", Rank: "1", PokemonID: "25", Span: 45, MaxUsers: 5, GuestScore: -2},
		{StartDate: "2026/11/27 16:23:35", Rank: "1", PokemonID: "25", Span: 45, MaxUsers: 5, GuestScore: 6},
		{StartDate: "2026/11/27 16:23:35", Rank: "1", PokemonID: "25", Span: 45, MaxUsers: 10, PassType: 0},
	} {
		_, err := client.CreateHostedRaid(context.Background(), "token", trainer, input, CallOptions{})
		checks = append(checks, err)
	}
	for _, err := range checks {
		assertExitCode(t, err, output.ExitInvalidArguments)
	}
	if calls.Load() != 0 {
		t.Fatalf("calls=%d want=0", calls.Load())
	}
}

func TestPrimitiveWritesDoNotRetryAndClassifyDispatchedFailureAsUncertain(t *testing.T) {
	trainer, _ := domain.NewTrainer("Ash123", "123456789012", 50, "se")
	identity, _ := domain.NewTrainerIdentity("Ash123", 50, "se")
	tests := []struct {
		name string
		call func(*Client) error
	}{
		{name: "create", call: func(client *Client) error {
			return client.CreateMatching(context.Background(), "token", trainer, QueueSelection{Ranks: []string{"5"}, PokemonIDs: []string{"150"}}, CallOptions{Timeout: 10 * time.Millisecond})
		}},
		{name: "cancel", call: func(client *Client) error {
			return client.CancelMatching(context.Background(), "token", CallOptions{Timeout: 10 * time.Millisecond})
		}},
		{name: "join", call: func(client *Client) error {
			return client.JoinRaid(context.Background(), "token", "raid", identity, CallOptions{Timeout: 10 * time.Millisecond})
		}},
		{name: "leave", call: func(client *Client) error {
			return client.LeaveRaid(context.Background(), "token", "raid", CallOptions{Timeout: 10 * time.Millisecond})
		}},
		{name: "ready", call: func(client *Client) error {
			return client.ReadyRaid(context.Background(), "token", "raid", CallOptions{Timeout: 10 * time.Millisecond})
		}},
		{name: "host create", call: func(client *Client) error {
			_, err := client.CreateHostedRaid(context.Background(), "token", trainer, HostLobbyInput{StartDate: "2026/11/27 16:23:35", Rank: "1", PokemonID: "25", Span: 45, MaxUsers: 5}, CallOptions{Timeout: 10 * time.Millisecond})
			return err
		}},
		{name: "host close", call: func(client *Client) error {
			return client.CloseHostedRaid(context.Background(), "token", "raid", CallOptions{Timeout: 10 * time.Millisecond})
		}},
		{name: "host start", call: func(client *Client) error {
			return client.StartHostedRaid(context.Background(), "token", "raid", CallOptions{Timeout: 10 * time.Millisecond})
		}},
		{name: "host kick", call: func(client *Client) error {
			return client.KickHostedGuest(context.Background(), "token", "raid", "guest", CallOptions{Timeout: 10 * time.Millisecond})
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				calls.Add(1)
				response.WriteHeader(http.StatusOK)
				response.(http.Flusher).Flush()
				<-request.Context().Done()
			}))
			defer server.Close()
			client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
			assertExitCode(t, testCase.call(client), output.ExitWriteOutcomeUnknown)
			if calls.Load() != 1 {
				t.Fatalf("calls=%d want=1", calls.Load())
			}
		})
	}
}
