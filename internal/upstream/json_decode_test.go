package upstream

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/albinmedoc/raidnow-cli/internal/output"
)

func TestDecodeJSONRejectsMalformedOrTrailingDocuments(t *testing.T) {
	testCases := []struct {
		name string
		raw  string
	}{
		{name: "trailing token", raw: `{"status":"success"} []`},
		{name: "malformed object", raw: `{"status":"success"`},
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
