package upstream

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/output"
)

func TestProfileReadsAcceptEmptyRawArrays(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		if request.PostForm.Get("type") == "cookie_get" {
			_, _ = response.Write([]byte(`{"status":"success","cookie_id":"secret-cookie"}`))
			return
		}
		_, _ = response.Write([]byte(`[]`))
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
	for name, call := range map[string]func() (int, error){
		"history": func() (int, error) {
			rows, err := client.ListHostedHistory(context.Background(), "token", CallOptions{})
			return len(rows), err
		},
		"blocks": func() (int, error) {
			rows, err := client.ListBlocks(context.Background(), "token", CallOptions{})
			return len(rows), err
		},
	} {
		length, err := call()
		if err != nil || length != 0 {
			t.Fatalf("%s length=%d err=%v", name, length, err)
		}
	}
}

func TestProfileHistoryRejectsExplicitNullOrMalformedNumericFields(t *testing.T) {
	for _, entry := range []string{
		`{"rpl_id":"raid","rpl_pokemon":"150","rpl_level":null}`,
		`{"rpl_id":"raid","rpl_pokemon":"150","rpl_level":"1.5"}`,
		`{"rpl_id":"raid","rpl_pokemon":"150","evaluatescore":null}`,
		`{"rpl_id":"raid","rpl_pokemon":"150","evaluatescore":"1.5"}`,
		`{"rpl_id":"raid","rpl_pokemon":"150","rplc_tid":"Guest","rplc_level":null}`,
		`{"rpl_id":"raid","rpl_pokemon":"150","rplc_tid":"Guest","evaluatescore_guest":null}`,
	} {
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
			_ = request.ParseForm()
			if request.PostForm.Get("type") == "cookie_get" {
				_, _ = response.Write([]byte(`{"status":"success","cookie_id":"secret-cookie"}`))
				return
			}
			_, _ = response.Write([]byte(`[` + entry + `]`))
		}))
		client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
		_, err := client.ListJoinedHistory(context.Background(), "token", CallOptions{})
		server.Close()
		assertExitCode(t, err, output.ExitUpstreamRejection)
	}
}

func TestListKickedAndRemovedHistoryUseExactOperationsWithoutGuest(t *testing.T) {
	for _, testCase := range []struct {
		name      string
		operation string
		call      func(*Client) ([]domain.ProfileHistoryRecord, error)
	}{
		{name: "kicked", operation: "kicklist_get", call: func(client *Client) ([]domain.ProfileHistoryRecord, error) {
			return client.ListKickedHistory(context.Background(), "token", CallOptions{})
		}},
		{name: "removed", operation: "deletelist_get", call: func(client *Client) ([]domain.ProfileHistoryRecord, error) {
			return client.ListRemovedHistory(context.Background(), "token", CallOptions{})
		}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			var forms []url.Values
			server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
				_ = request.ParseForm()
				forms = append(forms, request.PostForm)
				if request.PostForm.Get("type") == "cookie_get" {
					_, _ = response.Write([]byte(`{"status":"success","cookie_id":"secret-cookie"}`))
					return
				}
				_, _ = response.Write([]byte(`[{"rpl_id":"raid-1","rpl_pokemon":"25","rplc_tid":"must-not-appear"}]`))
			}))
			defer server.Close()
			client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})

			got, err := testCase.call(client)
			if err != nil || len(got) != 1 || got[0].Guest != nil {
				t.Fatalf("history=%#v err=%v", got, err)
			}
			assertExactForm(t, forms[1], url.Values{"type": {testCase.operation}, "lang": {"1"}, "token": {"token"}})
		})
	}
}

func TestListBlocksValidatesCookieAndNormalizesRawArray(t *testing.T) {
	var forms []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		forms = append(forms, request.PostForm)
		if request.PostForm.Get("type") == "cookie_get" {
			_, _ = response.Write([]byte(`{"status":"success","cookie_id":"secret-cookie"}`))
			return
		}
		_, _ = response.Write([]byte(`[{"rplb_id":77,"rplb_tid":"Blocked7","cookie_id":"hidden","send":"hidden"}]`))
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})

	got, err := client.ListBlocks(context.Background(), "token", CallOptions{})
	want := []domain.BlockRecord{{BlockID: "77", TrainerName: "Blocked7"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("blocks=%#v want=%#v err=%v", got, want, err)
	}
	assertExactForm(t, forms[1], url.Values{"type": {"block_get"}, "token": {"token"}})
}

func TestInvalidProfileCookieStopsBeforeHistoryOrBlocks(t *testing.T) {
	for _, body := range []string{`{"status":"error","cookie_id":"secret"}`, `{"status":"SUCCESS","cookie_id":"secret"}`, `{"status":" success ","cookie_id":"secret"}`, `{"status":"success","cookie_id":""}`, `{"status":"success","cookie_id":123}`, `{"status":"success","cookie_id":true}`, `{"status":"success","cookie_id":[]}`, `{"status":"success","cookie_id":{}}`, `{"status":"success","cookie_id":null}`, `{"status":"success"}`, `[]`} {
		var requests int
		server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
			requests++
			_, _ = response.Write([]byte(body))
		}))
		client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})
		_, historyErr := client.ListJoinedHistory(context.Background(), "token", CallOptions{})
		_, blocksErr := client.ListBlocks(context.Background(), "token", CallOptions{})
		server.Close()
		if got := requests; got != 2 {
			t.Fatalf("body=%s requests=%d want cookie-only calls", body, got)
		}
		assertExitCode(t, historyErr, output.ExitAuthenticationUnsupported)
		assertExitCode(t, blocksErr, output.ExitAuthenticationUnsupported)
	}
}

func TestListJoinedHistoryIncludesOptionalGuestAndUsesJoinOperation(t *testing.T) {
	var forms []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		forms = append(forms, request.PostForm)
		if request.PostForm.Get("type") == "cookie_get" {
			_, _ = response.Write([]byte(`{"status":"success","cookie_id":"secret-cookie"}`))
			return
		}
		_, _ = response.Write([]byte(`[{"rpl_id":"raid-7","rpl_insert":"Yesterday","rpl_pokemon":"6_x","rpl_tid":"Host7","rpl_level":40,"rpl_lang":"1","evaluatescore":-1,"rplc_tid":"Guest8","rplc_level":"44","rplc_country_code":"JP","evaluatescore_guest":"4","send":"hidden","rple_id":"hidden"}]`))
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})

	got, err := client.ListJoinedHistory(context.Background(), "token", CallOptions{})
	if err != nil {
		t.Fatalf("list joined history: %v", err)
	}
	zero, four := 0, 4
	want := []domain.ProfileHistoryRecord{{
		RaidID: "raid-7", DisplayTime: "Yesterday", PokemonID: "6_x", TrainerName: "Host7", Level: 40, Country: "world", Rating: &zero,
		Guest: &domain.ProfileGuest{TrainerName: "Guest8", Level: 44, Country: "jp", Rating: &four},
	}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("history=%#v want=%#v", got, want)
	}
	assertExactForm(t, forms[1], url.Values{"type": {"joinlist_get"}, "lang": {"1"}, "token": {"token"}})
}

func TestListHostedHistoryValidatesCookieThenNormalizesRawArray(t *testing.T) {
	var forms []url.Values
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		_ = request.ParseForm()
		forms = append(forms, request.PostForm)
		switch request.PostForm.Get("type") {
		case "cookie_get":
			_, _ = response.Write([]byte(`{"status":"success","cookie_id":"secret-cookie"}`))
		case "mylist_get":
			_, _ = response.Write([]byte(`[{"rpl_id":123,"rpl_insert":"3 hours ago","rpl_pokemon":"150","rpl_tid":"Host7","rpl_level":"50","rpl_country_code":"SE","evaluatescore":"5","send":"secret-chat","rple_id":"secret-evaluation","cookie_id":"secret-cookie"}]`))
		}
	}))
	defer server.Close()
	client := mustClient(t, Config{Endpoint: server.URL, HTTPClient: server.Client(), AllowInsecureHTTP: true})

	got, err := client.ListHostedHistory(context.Background(), "token", CallOptions{})
	if err != nil {
		t.Fatalf("list hosted history: %v", err)
	}
	rating := 5
	want := []domain.ProfileHistoryRecord{{RaidID: "123", DisplayTime: "3 hours ago", PokemonID: "150", TrainerName: "Host7", Level: 50, Country: "se", Rating: &rating}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("history=%#v want=%#v", got, want)
	}
	if len(forms) != 2 {
		t.Fatalf("forms=%#v", forms)
	}
	assertExactForm(t, forms[0], url.Values{"type": {"cookie_get"}, "token": {"token"}})
	assertExactForm(t, forms[1], url.Values{"type": {"mylist_get"}, "lang": {"1"}, "token": {"token"}})
}
