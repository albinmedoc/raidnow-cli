package metadata

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
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

func TestParseExtractsHostLobbyMetadata(t *testing.T) {
	catalog, err := Parse([]byte(`<script>jQuery(function($){ const aryPokemon={
		"ピカチュウ(太陽の冠)":{"en":"Pikachu sun crown","poke_raid_rank":"1","weather":["rain"]},
		"メガスピアー":{"en":"Mega Beedrill","poke_raid_rank":4,"weather":["rain","cloudy"]},
		"unboosted":{"en":"Unboosted","poke_raid_rank":"5","weather":[]},
		"stale":{"en":"Stale","poke_raid_rank":"1","weather":["sunny"]}
	}; const aryPokeRaid={"1":["ピカチュウ(太陽の冠)"],"4":["メガスピアー"],"5":["unboosted"]}; });</script>`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	pikachu, ok := catalog.Lookup("ピカチュウ(太陽の冠)")
	if !ok || pikachu.RaidRank != "1" || pikachu.CurrentRaidRank != "1" || !pikachu.HasWeatherBoosts || !reflect.DeepEqual(pikachu.WeatherBoosts, []string{"rain"}) {
		t.Fatalf("pikachu=%#v ok=%v", pikachu, ok)
	}
	mega, ok := catalog.Lookup("メガスピアー")
	if !ok || mega.RaidRank != "4" || mega.CurrentRaidRank != "4" || !mega.HasWeatherBoosts || !reflect.DeepEqual(mega.WeatherBoosts, []string{"rain", "cloudy"}) {
		t.Fatalf("mega=%#v ok=%v", mega, ok)
	}
	unboosted, ok := catalog.Lookup("unboosted")
	if !ok || unboosted.CurrentRaidRank != "5" || !unboosted.HasWeatherBoosts || len(unboosted.WeatherBoosts) != 0 {
		t.Fatalf("unboosted=%#v ok=%v", unboosted, ok)
	}
	stale, ok := catalog.Lookup("stale")
	if !ok || stale.CurrentRaidRank != "" {
		t.Fatalf("stale=%#v ok=%v", stale, ok)
	}
	mega.WeatherBoosts[0] = "changed"
	again, _ := catalog.Lookup("メガスピアー")
	if again.WeatherBoosts[0] != "rain" {
		t.Fatal("caller mutation changed weather metadata")
	}
}

func TestParseRejectsMalformedHostLobbyMetadata(t *testing.T) {
	for name, body := range map[string]string{
		"rank blank":          `<script>const aryPokemon={"id":{"poke_raid_rank":" "}};</script>`,
		"rank object":         `<script>const aryPokemon={"id":{"poke_raid_rank":{}}};</script>`,
		"weather not array":   `<script>const aryPokemon={"id":{"weather":"rain"}};</script>`,
		"weather blank":       `<script>const aryPokemon={"id":{"weather":[""]}};</script>`,
		"weather unsafe":      `<script>const aryPokemon={"id":{"weather":["rain\u0007"]}};</script>`,
		"weather wrong shape": `<script>const aryPokemon={"id":{"weather":[{}]}};</script>`,
	} {
		t.Run(name, func(t *testing.T) { assertRejected(t, []byte(body)) })
	}
}

func TestParseRejectsMalformedCurrentRaidMetadata(t *testing.T) {
	pokemon := `const aryPokemon={"25":{"poke_raid_rank":"1","weather":["rain"]}};`
	for name, raid := range map[string]string{
		"unknown pokemon":       `const aryPokeRaid={"1":["missing"]};`,
		"duplicate membership":  `const aryPokeRaid={"1":["25"],"3":["25"]};`,
		"blank rank":            `const aryPokeRaid={"":["25"]};`,
		"invalid rank":          `const aryPokeRaid={"one":["25"]};`,
		"not array":             `const aryPokeRaid={"1":"25"};`,
		"duplicate declaration": `const aryPokeRaid={"1":["25"]}; const aryPokeRaid={"1":["25"]};`,
	} {
		t.Run(name, func(t *testing.T) { assertRejected(t, []byte(`<script>`+pokemon+raid+`</script>`)) })
	}
}

func TestParseExtractsShinyAndEncounterCPMetadata(t *testing.T) {
	catalog, err := Parse([]byte(`<script>const aryPokemon={
		"attack":{"en":"Deoxys Attack Forme","shiny":1,"maxcp20":1474,"maxcp25":1842},
		"legacy":{"en":"Legacy Boss","shiny":"1","maxcp20":900,"maxcp25":""},
		"ordinary":{"en":"Ordinary Boss"}
	};</script>`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	attack, ok := catalog.Lookup("attack")
	if !ok || !attack.ShinyAvailable || attack.MaxCP20 == nil || *attack.MaxCP20 != 1474 || attack.MaxCP25 == nil || *attack.MaxCP25 != 1842 {
		t.Fatalf("attack entry = %#v, %v", attack, ok)
	}
	legacy, ok := catalog.Lookup("legacy")
	if !ok || !legacy.ShinyAvailable || legacy.MaxCP20 == nil || *legacy.MaxCP20 != 900 || legacy.MaxCP25 != nil {
		t.Fatalf("legacy entry = %#v, %v", legacy, ok)
	}
	ordinary, ok := catalog.Lookup("ordinary")
	if !ok || ordinary.ShinyAvailable || ordinary.MaxCP20 != nil || ordinary.MaxCP25 != nil {
		t.Fatalf("ordinary entry = %#v, %v", ordinary, ok)
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

func TestParseDoesNotRejectValidCatalogForUnrelatedLaterJavaScript(t *testing.T) {
	body := []byte(`<script>var aryPokemon={"right":{"en":"Right"}}; const pattern = /{/; function later(){</script>`)
	catalog, err := Parse(body)
	if err != nil || catalog.Len() != 1 {
		t.Fatalf("Parse Len()=%d err=%v, want valid catalog", catalog.Len(), err)
	}
}

func TestParseAcceptsCurrentJQueryReadyWrapper(t *testing.T) {
	body := []byte(`<script>jQuery(function($){ var aryPokemon={"right":{"en":"Right"}}; });</script>`)
	catalog, err := Parse(body)
	if err != nil || catalog.Len() != 1 {
		t.Fatalf("Parse Len()=%d err=%v, want wrapped catalog", catalog.Len(), err)
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
	catalog, err := Parse([]byte(`<script>var aryPokemon={"` + exactID + `":{"en":"A","maxcp20":100},"z":{}};</script>`))
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
	mutable, ok := catalog.Lookup(exactID)
	if !ok || mutable.MaxCP20 == nil {
		t.Fatal("exact entry missing CP metadata")
	}
	*mutable.MaxCP20 = 200
	again, ok := catalog.Lookup(exactID)
	if !ok || again.NameEN != "A" || again.MaxCP20 == nil || *again.MaxCP20 != 100 {
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
		"invalid_utf8":       append([]byte(`<script>var aryPokemon={"`), 0xff),
		"html_too_large":     []byte(`<script>var aryPokemon={"id":{}};</script>` + strings.Repeat("x", MaxHTMLBytes)),
		"script_too_large":   []byte(`<script>` + strings.Repeat(" ", MaxExecutableScriptBytes) + `var aryPokemon={"id":{}};</script>`),
		"empty_catalog":      []byte(`<script>var aryPokemon={};</script>`),
		"empty_id":           []byte(`<script>var aryPokemon={"":{}};</script>`),
		"blank_id":           []byte(`<script>var aryPokemon={"   ":{}};</script>`),
		"id_too_long":        []byte(`<script>var aryPokemon={"` + strings.Repeat("i", MaxIDBytes+1) + `":{}};</script>`),
		"name_too_long":      []byte(`<script>var aryPokemon={"id":{"en":"` + strings.Repeat("n", MaxNameBytes+1) + `"}};</script>`),
		"url_too_long":       []byte(`<script>var aryPokemon={"id":{"image":"` + strings.Repeat("u", MaxImageURLBytes+1) + `"}};</script>`),
		"too_many":           []byte(tooMany.String()),
		"entry_null":         []byte(`<script>var aryPokemon={"id":null};</script>`),
		"name_number":        []byte(`<script>var aryPokemon={"id":{"en":7}};</script>`),
		"control_name":       []byte(`<script>var aryPokemon={"id":{"en":"bad\u0007name"}};</script>`),
		"shiny_boolean":      []byte(`<script>var aryPokemon={"id":{"shiny":true}};</script>`),
		"shiny_out_of_range": []byte(`<script>var aryPokemon={"id":{"shiny":2}};</script>`),
		"shiny_null":         []byte(`<script>var aryPokemon={"id":{"shiny":null}};</script>`),
		"cp_zero":            []byte(`<script>var aryPokemon={"id":{"maxcp20":0}};</script>`),
		"cp_negative":        []byte(`<script>var aryPokemon={"id":{"maxcp20":-1}};</script>`),
		"cp_fractional":      []byte(`<script>var aryPokemon={"id":{"maxcp20":1474.5}};</script>`),
		"cp_numeric_string":  []byte(`<script>var aryPokemon={"id":{"maxcp20":"1474"}};</script>`),
		"cp_null":            []byte(`<script>var aryPokemon={"id":{"maxcp20":null}};</script>`),
		"duplicate_id":       []byte(`<script>var aryPokemon={"id":{"en":"bad"},"id":{"en":"good"}};</script>`),
		"duplicate_shiny":    []byte(`<script>var aryPokemon={"id":{"shiny":null,"shiny":1}};</script>`),
		"duplicate_cp":       []byte(`<script>var aryPokemon={"id":{"maxcp20":null,"maxcp20":1474}};</script>`),
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
