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
	if catalog.Len() != 3 {
		t.Fatalf("Len() = %d, want 3", catalog.Len())
	}
	entry, ok := catalog.Lookup("150_アーマード")
	if !ok || entry.ID != "150_アーマード" || entry.NameEN != "Armored Mewtwo" || entry.ImageURL != "https://example.invalid/150-armored.png" {
		t.Fatalf("exact form entry = %#v, %v", entry, ok)
	}
	escaped, ok := catalog.Lookup("escaped")
	if !ok || escaped.NameEN != "Quote: \" brace: } slash: \\ semicolon: ; 雪" {
		t.Fatalf("escaped entry = %#v, %v", escaped, ok)
	}
}

func TestParseSupportsConventionalTopLevelDeclarations(t *testing.T) {
	for _, declaration := range []string{"var", "let", "const"} {
		t.Run(declaration, func(t *testing.T) {
			body := []byte(`<script>` + declaration + ` aryPokemon={"id":{"en":" Name ","image":" image "}};</script>`)
			catalog, err := Parse(body)
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			entry, ok := catalog.Lookup("id")
			if !ok || entry.NameEN != " Name " || entry.ImageURL != " image " {
				t.Fatalf("entry = %#v, %v; want exact field strings", entry, ok)
			}
		})
	}
}

func TestParseUsesExecutableScriptsAndIgnoresCheapDecoys(t *testing.T) {
	body := []byte(`<html><body>
		var aryPokemon={"outside":{}}
		<script src="external.js">var aryPokemon={"external":{}};</script>
		<script type="application/json">var aryPokemon={"json":{}};</script>
		<script>
			// var aryPokemon={"line":{}};
			/* const aryPokemon={"block":{}}; */
			const quoted = "var aryPokemon={\"string\":{}}";
			function nested(){ const aryPokemon={"nested":{}}; }
		</script>
		<script>const aryPokemon={"right":{"en":"Right"}};</script>
	</body></html>`)

	catalog, err := Parse(body)
	if err != nil || catalog.Len() != 1 {
		t.Fatalf("Parse Len()=%d err=%v, want one entry", catalog.Len(), err)
	}
	if _, ok := catalog.Lookup("right"); !ok {
		t.Fatal("right entry missing")
	}
}

func TestParseRequiresExactlyOneCompleteObjectDeclaration(t *testing.T) {
	for _, body := range []string{
		`<script>const other={};</script>`,
		`<script>var aryPokemon={"a":{}}; const aryPokemon={"b":{}};</script>`,
		`<script>aryPokemon={"a":{}};</script>`,
		`<script>var aryPokemon=[];</script>`,
		`<script>var aryPokemon={"a":{}</script>`,
		`<script>var aryPokemon={"a":{}} + other;</script>`,
		`<script>var aryPokemon={'a':{}};</script>`,
		`<script>var aryPokemon={"a":{"en":"unterminated}};</script>`,
	} {
		assertRejected(t, []byte(body))
	}
}

func TestParsePreservesExactIDsAndCatalogDefendsItsCollection(t *testing.T) {
	const exactID = "  exact-id  "
	catalog, err := Parse([]byte(`<script>var aryPokemon={"` + exactID + `":{"en":"A"},"z":{}};</script>`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	entry, ok := catalog.Lookup(exactID)
	if !ok || entry.ID != exactID {
		t.Fatalf("Lookup exact ID = %#v, %v", entry, ok)
	}
	entries := catalog.List()
	if len(entries) != 2 || entries[0].ID != exactID || entries[1].ID != "z" {
		t.Fatalf("List() = %#v", entries)
	}
	entries[0] = Entry{}
	again, ok := catalog.Lookup(exactID)
	if !ok || again.NameEN != "A" {
		t.Fatal("caller mutation changed catalog")
	}
}

func TestParseEnforcesInputScriptCatalogAndFieldBounds(t *testing.T) {
	tooMany := strings.Builder{}
	tooMany.WriteString(`<script>var aryPokemon={`)
	for index := 0; index <= MaxEntries; index++ {
		if index > 0 {
			tooMany.WriteByte(',')
		}
		fmt.Fprintf(&tooMany, `%q:{}`, fmt.Sprintf("id-%d", index))
	}
	tooMany.WriteString(`};</script>`)

	for name, body := range map[string][]byte{
		"invalid_utf8":     append([]byte(`<script>var aryPokemon={"`), 0xff),
		"html_too_large":   []byte(`<script>var aryPokemon={"id":{}};</script>` + strings.Repeat("x", MaxHTMLBytes)),
		"script_too_large": []byte(`<script>` + strings.Repeat(" ", MaxExecutableScriptBytes) + `var aryPokemon={"id":{}};</script>`),
		"empty_catalog":    []byte(`<script>var aryPokemon={};</script>`),
		"empty_id":         []byte(`<script>var aryPokemon={"":{}};</script>`),
		"blank_id":         []byte(`<script>var aryPokemon={"   ":{}};</script>`),
		"id_too_long":      []byte(`<script>var aryPokemon={"` + strings.Repeat("i", MaxIDBytes+1) + `":{}};</script>`),
		"name_too_long":    []byte(`<script>var aryPokemon={"id":{"en":"` + strings.Repeat("n", MaxNameBytes+1) + `"}};</script>`),
		"url_too_long":     []byte(`<script>var aryPokemon={"id":{"image":"` + strings.Repeat("u", MaxImageURLBytes+1) + `"}};</script>`),
		"too_many":         []byte(tooMany.String()),
		"entry_null":       []byte(`<script>var aryPokemon={"id":null};</script>`),
		"name_number":      []byte(`<script>var aryPokemon={"id":{"en":7}};</script>`),
		"control_name":     []byte(`<script>var aryPokemon={"id":{"en":"bad\u0007name"}};</script>`),
	} {
		t.Run(name, func(t *testing.T) { assertRejected(t, body) })
	}
}

func TestParseAcceptsExactEntryAndFieldBounds(t *testing.T) {
	fields := []byte(`<script>var aryPokemon={"` + strings.Repeat("i", MaxIDBytes) + `":{"en":"` + strings.Repeat("n", MaxNameBytes) + `","image":"` + strings.Repeat("u", MaxImageURLBytes) + `"}};</script>`)
	catalog, err := Parse(fields)
	if err != nil || catalog.Len() != 1 {
		t.Fatalf("Parse exact field bounds Len()=%d err=%v, want one entry", catalog.Len(), err)
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
		t.Fatalf("Parse exact bounds Len()=%d err=%v, want %d", catalog.Len(), err, MaxEntries)
	}
}

func assertRejected(t *testing.T, body []byte) {
	t.Helper()
	catalog, err := Parse(body)
	if !errors.Is(err, ErrInvalidMetadata) || catalog.Len() != 0 {
		t.Fatalf("Parse Len()=%d error=%v, want sanitized rejection", catalog.Len(), err)
	}
	for _, forbidden := range []string{"SECRET", "aryPokemon", "<script", "pokemon.invalid"} {
		if strings.Contains(fmt.Sprint(err), forbidden) {
			t.Fatalf("error %q leaked %q", err, forbidden)
		}
	}
}
