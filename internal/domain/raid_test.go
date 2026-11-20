package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRaidJSONNeverIncludesHostTrainerCode(t *testing.T) {
	encoded, err := json.Marshal(Raid{ID: "raid-1", HostTrainerCode: "123456789012"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "123456789012") || strings.Contains(string(encoded), "hostTrainerCode") {
		t.Fatalf("generic raid JSON exposed trainer code: %s", encoded)
	}
}
