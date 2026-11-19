package metadata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseExtractsRealisticAryPokemonCatalog(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("testdata", "ary_pokemon.html"))
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	catalog, err := Parse(body)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, want := catalog.Len(), 3; got != want {
		t.Fatalf("Len() = %d, want %d", got, want)
	}
	entry, ok := catalog.Lookup("150_アーマード")
	if !ok {
		t.Fatal("Lookup exact Japanese form ID = false, want true")
	}
	if got, want := entry.ID(), "150_アーマード"; got != want {
		t.Fatalf("ID() = %q, want %q", got, want)
	}
	if got, want := entry.Name(), "Armored Mewtwo"; got != want {
		t.Fatalf("Name() = %q, want %q", got, want)
	}
	if got, want := entry.ImageURL(), "https://example.invalid/150-armored.png"; got != want {
		t.Fatalf("ImageURL() = %q, want %q", got, want)
	}
	escaped, ok := catalog.Lookup("escaped")
	if !ok {
		t.Fatal("Lookup escaped entry = false, want true")
	}
	if got, want := escaped.Name(), "Quote: \" brace: } slash: \\ semicolon: ; 雪"; got != want {
		t.Fatalf("escaped Name() = %q, want %q", got, want)
	}
}

func TestCatalogListIsSortedAndEntriesAreValues(t *testing.T) {
	catalog, err := Parse([]byte(`<script>var aryPokemon={"z":{"en":"Z","image":"z"},"a":{"en":"A","image":"a"}};</script>`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	entries := catalog.List()
	if len(entries) != 2 || entries[0].ID() != "a" || entries[1].ID() != "z" {
		t.Fatalf("List IDs = %#v, want [a z]", entries)
	}
	entries[0] = Entry{}
	again, ok := catalog.Lookup("a")
	if !ok || again.Name() != "A" {
		t.Fatal("caller mutation changed catalog")
	}
}

func TestParseIgnoresNonExecutableAndLexicalDecoys(t *testing.T) {
	body := []byte(`<html><body>
		aryPokemon = {"outside":{"en":"wrong"}}
		<!-- <script>var aryPokemon={"comment":{"en":"wrong"}};</script> -->
		<template><script>var aryPokemon={"template-node":{"en":"wrong"}};</script></template>
		<script src="external.js">var aryPokemon={"external":{"en":"wrong"}};</script>
		<script type="application/json">var aryPokemon={"mime":{"en":"wrong"}};</script>
		<script>
			// var aryPokemon={"line-comment":{"en":"wrong"}};
			/* var aryPokemon={"block-comment":{"en":"wrong"}}; */
			const quoted = "var aryPokemon={\"string\":{\"en\":\"wrong\"}}";
			const template = ` + "`var aryPokemon={\"template\":{\"en\":\"wrong\"}}`" + `;
			const regex = /var aryPokemon=\{\"regex\":\{\}\};/;
			const aryPokemonLonger = {"longer":{"en":"wrong"}};
			const holder = {aryPokemon:{"property":{"en":"wrong"}}};
		</script>
		<script type="text/javascript; charset=utf-8">const aryPokemon={"right":{"en":"Right","image":"right.png"}};</script>
	</body></html>`)
	catalog, err := Parse(body)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if catalog.Len() != 1 {
		t.Fatalf("Len() = %d, want 1", catalog.Len())
	}
	if _, ok := catalog.Lookup("right"); !ok {
		t.Fatal("exact executable candidate missing")
	}
}

func TestParseRequiresExactlyOneObjectAssignment(t *testing.T) {
	testCases := []struct {
		name string
		body string
	}{
		{name: "missing", body: `<script>const other={};</script>`},
		{name: "duplicate_same_script", body: `<script>var aryPokemon={"a":{}}; aryPokemon={"b":{}};</script>`},
		{name: "duplicate_scripts", body: `<script>let aryPokemon={"a":{}};</script><script>const aryPokemon={"b":{}};</script>`},
		{name: "non_object_number", body: `<script>var aryPokemon=1;</script>`},
		{name: "non_object_array", body: `<script>var aryPokemon=[];</script>`},
		{name: "trailing_expression", body: `<script>aryPokemon={"a":{}}+other;</script>`},
		{name: "incomplete", body: `<script>var aryPokemon={"a":{}</script>`},
		{name: "malformed_javascript", body: `<script>const broken=; var aryPokemon={"a":{}};</script>`},
		{name: "malformed_json_single_quotes", body: `<script>var aryPokemon={'a':{"en":"A"}};</script>`},
		{name: "malformed_json_unquoted_key", body: `<script>var aryPokemon={a:{"en":"A"}};</script>`},
		{name: "malformed_json_trailing_comma", body: `<script>var aryPokemon={"a":{"en":"A"},};</script>`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assertRejected(t, []byte(testCase.body))
		})
	}
}

func assertRejected(t *testing.T, body []byte) {
	t.Helper()
	catalog, err := Parse(body)
	if !errors.Is(err, ErrInvalidMetadata) {
		t.Fatalf("Parse error = %v, want ErrInvalidMetadata", err)
	}
	if catalog.Len() != 0 {
		t.Fatalf("rejected catalog Len() = %d, want 0", catalog.Len())
	}
	for _, forbidden := range []string{"SECRET", "aryPokemon", "<script", "pokemon.invalid"} {
		if strings.Contains(fmt.Sprint(err), forbidden) {
			t.Fatalf("error %q leaked %q", err, forbidden)
		}
	}
}

func TestParseNormalizesNullMissingAndBlankFields(t *testing.T) {
	catalog, err := Parse([]byte(`<script>var aryPokemon={"nulls":{"en":null,"image":null},"missing":{},"blank":{"en":"   ","image":" \u00A0 "}};</script>`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	for _, id := range []string{"nulls", "missing", "blank"} {
		entry, ok := catalog.Lookup(id)
		if !ok || entry.Name() != "" || entry.ImageURL() != "" {
			t.Fatalf("Lookup(%q) = %#v, %v; want empty normalized fields", id, entry, ok)
		}
	}
}

func TestParseRejectsBlankCatalogIDsAndPreservesNonblankIDs(t *testing.T) {
	for _, id := range []string{"   ", `\u00A0`} {
		t.Run(fmt.Sprintf("blank_%q", id), func(t *testing.T) {
			assertRejected(t, []byte(`<script>var aryPokemon={"`+id+`":{}};</script>`))
		})
	}

	const exactID = "  exact-id\u00A0 "
	catalog, err := Parse([]byte(`<script>var aryPokemon={"` + exactID + `":{}};</script>`))
	if err != nil {
		t.Fatalf("Parse nonblank ID: %v", err)
	}
	decodedID := "  exact-id\u00A0 "
	if _, ok := catalog.Lookup(decodedID); !ok {
		t.Fatalf("Lookup(%q) = false, want exact untrimmed ID", decodedID)
	}
	entry := catalog.List()[0]
	if entry.ID() != decodedID {
		t.Fatalf("ID() = %q, want exact %q", entry.ID(), decodedID)
	}
}

func TestParseRejectsUnpairedSurrogateEscapesEverywhere(t *testing.T) {
	testCases := []struct {
		name string
		body string
	}{
		{name: "lone_high_id", body: `<script>var aryPokemon={"bad\uD83D":{}};</script>`},
		{name: "lone_low_name", body: `<script>var aryPokemon={"id":{"en":"bad\uDE00"}};</script>`},
		{name: "reversed_image", body: `<script>var aryPokemon={"id":{"image":"bad\uDE00\uD83D"}};</script>`},
		{name: "high_followed_by_non_low", body: `<script>var aryPokemon={"id":{"extra":{"value":"bad\uD83D\u0041"}}};</script>`},
		{name: "high_followed_by_high", body: `<script>var aryPokemon={"id":{"extra":{"value":"bad\uD83D\uD83D"}}};</script>`},
		{name: "nested_unknown_key", body: `<script>var aryPokemon={"id":{"extra":{"bad\uD83D":true}}};</script>`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assertRejected(t, []byte(testCase.body))
		})
	}
}

func TestParseAcceptsValidSurrogatePairsEverywhere(t *testing.T) {
	catalog, err := Parse([]byte(`<script>var aryPokemon={"id\uD83D\uDE00":{"en":"name\uD83D\uDE00","image":"image\uD83D\uDE00","extra":{"key\uD83D\uDE00":"value\uD83D\uDE00"}}};</script>`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	entry, ok := catalog.Lookup("id😀")
	if !ok {
		t.Fatal("Lookup decoded supplementary-plane ID = false, want true")
	}
	if entry.Name() != "name😀" || entry.ImageURL() != "image😀" {
		t.Fatalf("decoded fields = (%q, %q), want supplementary-plane rune", entry.Name(), entry.ImageURL())
	}
}

func TestParseRejectsUnsafeUnicodeInEveryJSONKeyAndString(t *testing.T) {
	testCases := []struct {
		name string
		body string
	}{
		{name: "nested_control_key", body: `<script>var aryPokemon={"id":{"extra":{"bad\u0000key":true}}};</script>`},
		{name: "nested_control_value", body: `<script>var aryPokemon={"id":{"extra":{"value":"bad\u0085value"}}};</script>`},
		{name: "nested_format_key", body: `<script>var aryPokemon={"id":{"extra":{"bad\u2060key":true}}};</script>`},
		{name: "nested_format_value", body: `<script>var aryPokemon={"id":{"extra":["bad\u200Dvalue"]}};</script>`},
		{name: "nested_line_separator", body: `<script>var aryPokemon={"id":{"extra":{"value":"bad\u2028value"}}};</script>`},
		{name: "nested_paragraph_separator", body: `<script>var aryPokemon={"id":{"extra":{"value":"bad\u2029value"}}};</script>`},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assertRejected(t, []byte(testCase.body))
		})
	}
}

func TestParseRejectsDuplicateKeysAtEveryNestingLevel(t *testing.T) {
	for _, body := range []string{
		`<script>var aryPokemon={"same":{},"same":{}};</script>`,
		`<script>var aryPokemon={"id":{"en":"one","en":"two"}};</script>`,
		`<script>var aryPokemon={"id":{"extra":{"same":1,"same":2}}};</script>`,
		`<script>var aryPokemon={"id":{"extra":[{"same":1,"same":2}]}};</script>`,
	} {
		assertRejected(t, []byte(body))
	}
}

func TestParseRejectsUnsafeTypesTextAndBounds(t *testing.T) {
	tooMany := strings.Builder{}
	tooMany.WriteString(`<script>var aryPokemon={`)
	for index := 0; index <= MaxEntries; index++ {
		if index > 0 {
			tooMany.WriteByte(',')
		}
		fmt.Fprintf(&tooMany, `%q:{}`, fmt.Sprintf("id-%d", index))
	}
	tooMany.WriteString(`};</script>`)

	deep := `{"leaf":true}`
	for range MaxJSONDepth {
		deep = `[` + deep + `]`
	}

	testCases := []struct {
		name string
		body []byte
	}{
		{name: "invalid_utf8", body: append([]byte(`<script>var aryPokemon={"`), []byte{0xff}...)},
		{name: "empty_catalog", body: []byte(`<script>var aryPokemon={};</script>`)},
		{name: "empty_id", body: []byte(`<script>var aryPokemon={"":{}};</script>`)},
		{name: "control_id", body: []byte(`<script>var aryPokemon={"bad\u0000id":{}};</script>`)},
		{name: "format_id", body: []byte(`<script>var aryPokemon={"bad\u200Did":{}};</script>`)},
		{name: "control_name", body: []byte(`<script>var aryPokemon={"id":{"en":"bad\u0007name"}};</script>`)},
		{name: "format_name", body: []byte(`<script>var aryPokemon={"id":{"en":"bad\u2060name"}};</script>`)},
		{name: "line_separator_url", body: []byte(`<script>var aryPokemon={"id":{"image":"bad\u2028url"}};</script>`)},
		{name: "entry_null", body: []byte(`<script>var aryPokemon={"id":null};</script>`)},
		{name: "entry_string", body: []byte(`<script>var aryPokemon={"id":"not-object"};</script>`)},
		{name: "entry_array", body: []byte(`<script>var aryPokemon={"id":[]};</script>`)},
		{name: "name_number", body: []byte(`<script>var aryPokemon={"id":{"en":7}};</script>`)},
		{name: "image_boolean", body: []byte(`<script>var aryPokemon={"id":{"image":true}};</script>`)},
		{name: "id_too_long", body: []byte(`<script>var aryPokemon={"` + strings.Repeat("i", MaxIDBytes+1) + `":{}};</script>`)},
		{name: "name_too_long", body: []byte(`<script>var aryPokemon={"id":{"en":"` + strings.Repeat("n", MaxNameBytes+1) + `"}};</script>`)},
		{name: "url_too_long", body: []byte(`<script>var aryPokemon={"id":{"image":"` + strings.Repeat("u", MaxImageURLBytes+1) + `"}};</script>`)},
		{name: "too_many_entries", body: []byte(tooMany.String())},
		{name: "too_deep", body: []byte(`<script>var aryPokemon={"id":{"extra":` + deep + `}};</script>`)},
		{name: "html_too_large", body: []byte(`<script>var aryPokemon={"id":{}};</script>` + strings.Repeat("x", MaxHTMLBytes))},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			assertRejected(t, testCase.body)
		})
	}
}

func TestParseSupportsDirectVarLetConstAndAssignment(t *testing.T) {
	for _, declaration := range []string{"var aryPokemon=", "let aryPokemon=", "const aryPokemon=", "aryPokemon="} {
		t.Run(strings.Fields(declaration)[0], func(t *testing.T) {
			catalog, err := Parse([]byte(`<script>` + declaration + `{"id":{"en":"Name","image":"image"}};</script>`))
			if err != nil || catalog.Len() != 1 {
				t.Fatalf("Parse(%q) Len()=%d err=%v, want one entry", declaration, catalog.Len(), err)
			}
		})
	}
}

func TestParseAcceptsExactCatalogAndFieldBounds(t *testing.T) {
	deep := `true`
	for range MaxJSONDepth - 3 {
		deep = `[` + deep + `]`
	}
	body := []byte(`<script>var aryPokemon={"` + strings.Repeat("i", MaxIDBytes) + `":{"en":"` + strings.Repeat("n", MaxNameBytes) + `","image":"` + strings.Repeat("u", MaxImageURLBytes) + `","extra":` + deep + `}};</script>`)
	catalog, err := Parse(body)
	if err != nil || catalog.Len() != 1 {
		t.Fatalf("Parse exact bounds Len()=%d err=%v, want one entry", catalog.Len(), err)
	}

	var entries strings.Builder
	entries.WriteString(`<script>var aryPokemon={`)
	for index := 0; index < MaxEntries; index++ {
		if index > 0 {
			entries.WriteByte(',')
		}
		fmt.Fprintf(&entries, `%q:{}`, fmt.Sprintf("id-%d", index))
	}
	entries.WriteString(`};</script>`)
	catalog, err = Parse([]byte(entries.String()))
	if err != nil || catalog.Len() != MaxEntries {
		t.Fatalf("Parse exact entry bound Len()=%d err=%v, want %d", catalog.Len(), err, MaxEntries)
	}
}

func TestCatalogAndEntryFormattingRedactsValues(t *testing.T) {
	const secretID = "SECRET-ID"
	const secretName = "SECRET-NAME"
	const secretURL = "https://pokemon.invalid/SECRET-URL"
	catalog, err := Parse([]byte(`<script>var aryPokemon={"` + secretID + `":{"en":"` + secretName + `","image":"` + secretURL + `"}};</script>`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	entry, _ := catalog.Lookup(secretID)
	for _, formatted := range []string{
		fmt.Sprint(catalog), fmt.Sprintf("%#v", catalog),
		fmt.Sprint(entry), fmt.Sprintf("%#v", entry),
	} {
		for _, secret := range []string{secretID, secretName, secretURL} {
			if strings.Contains(formatted, secret) {
				t.Fatalf("formatted value %q leaked secret", formatted)
			}
		}
	}
	if got := fmt.Sprint(catalog); !strings.Contains(got, "entries:1") {
		t.Fatalf("Catalog format = %q, want count", got)
	}
}
