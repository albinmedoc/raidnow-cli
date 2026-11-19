package upstream

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/albinmedoc/raidnow-cli/internal/output"
)

func TestDecodeJSONRejectsAmbiguousOrUnboundedDocuments(t *testing.T) {
	testCases := []struct {
		name string
		raw  string
	}{
		{name: "duplicate top level key", raw: `{"status":"error","status":"success"}`},
		{name: "duplicate nested key", raw: `{"status":"success","unknown":{"value":1,"value":2}}`},
		{name: "trailing token", raw: `{"status":"success"} []`},
		{name: "malformed object", raw: `{"status":"success"`},
		{name: "excessive depth", raw: strings.Repeat(`[`, 300) + strings.Repeat(`]`, 300)},
		{name: "excessive nodes", raw: `[` + strings.Repeat(`0,`, 100000) + `0]`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := decodeJSON(json.RawMessage(testCase.raw))
			if got := output.ExitCodeFor(err); got != output.ExitUpstreamRejection {
				t.Fatalf("exit=%d err=%v", got, err)
			}
			if err != nil && strings.Contains(err.Error(), "status") {
				t.Fatalf("error leaked document content: %v", err)
			}
		})
	}
}

func TestDecodeJSONEnforcesDepthAndNodeBoundaries(t *testing.T) {
	validDepth := strings.Repeat(`[`, maxJSONDepth) + `0` + strings.Repeat(`]`, maxJSONDepth)
	if _, err := decodeJSON(json.RawMessage(validDepth)); err != nil {
		t.Fatalf("depth boundary rejected: %v", err)
	}
	if _, err := decodeJSON(json.RawMessage(`[` + validDepth + `]`)); output.ExitCodeFor(err) != output.ExitUpstreamRejection {
		t.Fatalf("depth beyond boundary err=%v", err)
	}

	validNodes := `[` + strings.Repeat(`0,`, maxJSONNodes-2) + `0]`
	if _, err := decodeJSON(json.RawMessage(validNodes)); err != nil {
		t.Fatalf("node boundary rejected: %v", err)
	}
	invalidNodes := `[` + strings.Repeat(`0,`, maxJSONNodes-1) + `0]`
	if _, err := decodeJSON(json.RawMessage(invalidNodes)); output.ExitCodeFor(err) != output.ExitUpstreamRejection {
		t.Fatalf("nodes beyond boundary err=%v", err)
	}
}

func TestDecodeJSONPreservesNumbersAndNestedValues(t *testing.T) {
	value, err := decodeJSON(json.RawMessage(`{"outer":[{"number":9007199254740993}],"ok":true}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	object := value.(map[string]any)
	nested := object["outer"].([]any)[0].(map[string]any)
	if got, ok := nested["number"].(json.Number); !ok || got.String() != "9007199254740993" {
		t.Fatalf("number=%#v", nested["number"])
	}
}
