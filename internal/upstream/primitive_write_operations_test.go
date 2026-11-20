package upstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
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
