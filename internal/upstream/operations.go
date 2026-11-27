package upstream

import (
	"context"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/session"
)

const (
	maxSelectionValues = 64
	maxLobbyFilters    = 256
	maxQueryValueBytes = 256
	maxLobbyPage       = 10000
)

// LobbyQuery describes one bounded read-only par_list request.
type LobbyQuery struct {
	Filters      []string
	Page         int
	SearchRank   string
	HostLevel    int
	HostEvaluate []int
}

// QueueSelection identifies Auto Join ranks, species/forms, and matching filters.
type QueueSelection struct {
	Ranks             []string
	PokemonIDs        []string
	GuestEvalStar     string
	MinimumHostRating int
	HostCountry       int
	WeatherBoost      int
	HostLevel         int
}

// HostLobbyInput contains the invocation-scoped settings for one hosted lobby.
type HostLobbyInput struct {
	StartDate        string
	Rank             string
	PokemonID        string
	Span             int
	Color            int
	Weather          int
	CurrentWeather   int
	MaxUsers         int
	GymColor         int
	PassType         int
	Private          bool
	Filters          []string
	Possible         bool
	GuestLevel       int
	GuestScore       int
	InvitationOnly   bool
	Hot              bool
	MegaPokemonEntry bool
	WeatherBoosts    []string
}

// ListCurrentRaidBosses returns the current read-only Auto Join catalog.
func (client *Client) ListCurrentRaidBosses(ctx context.Context, token string, options CallOptions) ([]domain.RaidBoss, error) {
	if !session.AnonymousTokenIsValid(token) {
		return nil, invalidArguments("Ogiltig anonym token.")
	}
	raw, err := client.PostForm(ctx, LineMatchingGetList, url.Values{"token": {token}}, options)
	if err != nil {
		return nil, err
	}
	return decodeRaidBosses(raw)
}

// ListRaids returns one normalized lobby page.
func (client *Client) ListRaids(ctx context.Context, token string, query LobbyQuery, options CallOptions) ([]domain.Raid, error) {
	if !session.AnonymousTokenIsValid(token) || query.Page < 1 || query.Page > maxLobbyPage || !allowedHostLevel(query.HostLevel) || !safeOptionalValue(query.SearchRank) {
		return nil, invalidArguments("Ogiltiga sökvillkor.")
	}
	filters, ok := normalizeValuesBounded(query.Filters, true, maxLobbyFilters)
	if !ok {
		return nil, invalidArguments("Ogiltiga sökvillkor.")
	}
	hostEvaluate := make([]string, 0, len(query.HostEvaluate))
	seenEvaluate := make(map[int]struct{}, len(query.HostEvaluate))
	if len(query.HostEvaluate) > maxSelectionValues {
		return nil, invalidArguments("Ogiltiga sökvillkor.")
	}
	for _, value := range query.HostEvaluate {
		if value < -1 || value > 5 {
			return nil, invalidArguments("Ogiltiga sökvillkor.")
		}
		if _, exists := seenEvaluate[value]; exists {
			continue
		}
		seenEvaluate[value] = struct{}{}
		hostEvaluate = append(hostEvaluate, strconv.Itoa(value))
	}
	form := url.Values{
		"possible":      {"false"},
		"page":          {strconv.Itoa(query.Page)},
		"firstaccess":   {"0"},
		"searchrank":    {query.SearchRank},
		"hostevalstar":  {""},
		"guestevalstar": {""},
		"lang":          {"1"},
		"token":         {token},
		"hostlevel":     {strconv.Itoa(query.HostLevel)},
	}
	if len(filters) > 0 {
		form["filter[]"] = filters
	}
	if len(hostEvaluate) > 0 {
		form["hostevaluate[]"] = hostEvaluate
	}
	raw, err := client.PostForm(ctx, ParList, form, options)
	if err != nil {
		return nil, err
	}
	return decodeRaids(raw)
}

// PollMatching performs one read-only Auto Join queue poll.
func (client *Client) PollMatching(ctx context.Context, token string, selection QueueSelection, options CallOptions) (domain.QueuePoll, error) {
	ranks, pokemon, selectionOK := normalizeSelectionPairs(selection.Ranks, selection.PokemonIDs)
	if !session.AnonymousTokenIsValid(token) || !selectionOK || len(ranks) == 0 || !safeOptionalValue(selection.GuestEvalStar) {
		return domain.QueuePoll{}, invalidArguments("Ogiltigt köurval.")
	}
	form := url.Values{
		"rank[]":        ranks,
		"pokemon[]":     pokemon,
		"guestevalstar": {selection.GuestEvalStar},
		"token":         {token},
	}
	raw, err := client.PostForm(ctx, LineMatchingList, form, options)
	if err != nil {
		return domain.QueuePoll{}, err
	}
	return decodeQueuePoll(raw)
}

// CheckMatching checks whether the token's selected queue is visible upstream.
func (client *Client) CheckMatching(ctx context.Context, token string, pokemonIDs []string, options CallOptions) (domain.QueueCheck, error) {
	pokemon, ok := normalizeValues(pokemonIDs, true)
	if !session.AnonymousTokenIsValid(token) || !ok || len(pokemon) == 0 {
		return domain.QueueCheck{}, invalidArguments("Ogiltigt köurval.")
	}
	raw, err := client.PostForm(ctx, LineMatchingCheck, url.Values{"pokemon[]": pokemon, "token": {token}}, options)
	if err != nil {
		return domain.QueueCheck{}, err
	}
	return decodeQueueCheck(raw)
}

// FindRaid returns the exact requested raid, or nil for a valid absence.
func (client *Client) FindRaid(ctx context.Context, token, id string, options CallOptions) (*domain.Raid, error) {
	if !session.AnonymousTokenIsValid(token) || !safeValue(id, true) {
		return nil, invalidArguments("Ogiltigt raid-id.")
	}
	form := url.Values{
		"possible":    {"false"},
		"firstaccess": {id},
		"lang":        {"1"},
		"token":       {token},
		"hostlevel":   {"0"},
	}
	raw, err := client.PostForm(ctx, ParFindFirst, form, options)
	if err != nil {
		return nil, err
	}
	raids, err := decodeRaids(raw)
	if err != nil {
		return nil, err
	}
	for index := range raids {
		if raids[index].ID == id {
			result := raids[index]
			return &result, nil
		}
	}
	return nil, nil
}

// ListRaidPlayers returns normalized read-only membership details.
func (client *Client) ListRaidPlayers(ctx context.Context, token, raidID string, options CallOptions) (domain.RaidPlayers, error) {
	if !session.AnonymousTokenIsValid(token) || !safeValue(raidID, true) {
		return domain.RaidPlayers{}, invalidArguments("Ogiltigt raid-id.")
	}
	form := url.Values{"parid": {raidID}, "lang": {"1"}, "token": {token}}
	raw, err := client.PostForm(ctx, ChildList, form, options)
	if err != nil {
		return domain.RaidPlayers{}, err
	}
	return decodeRaidPlayers(raw)
}

func allowedHostLevel(value int) bool {
	return value == 0 || value == 40 || value == 50 || value == 60 || value == 70
}

func normalizeSelectionPairs(ranks, pokemonIDs []string) ([]string, []string, bool) {
	if len(ranks) != len(pokemonIDs) || len(ranks) > maxSelectionValues {
		return nil, nil, false
	}
	type pair struct{ rank, pokemon string }
	seen := make(map[pair]struct{}, len(ranks))
	normalizedRanks := make([]string, 0, len(ranks))
	normalizedPokemon := make([]string, 0, len(pokemonIDs))
	for index := range ranks {
		if !safeValue(ranks[index], true) || !safeValue(pokemonIDs[index], true) {
			return nil, nil, false
		}
		selection := pair{rank: ranks[index], pokemon: pokemonIDs[index]}
		if _, exists := seen[selection]; exists {
			continue
		}
		seen[selection] = struct{}{}
		normalizedRanks = append(normalizedRanks, selection.rank)
		normalizedPokemon = append(normalizedPokemon, selection.pokemon)
	}
	return normalizedRanks, normalizedPokemon, true
}

func normalizeValues(values []string, requireNonempty bool) ([]string, bool) {
	return normalizeValuesBounded(values, requireNonempty, maxSelectionValues)
}

func normalizeValuesBounded(values []string, requireNonempty bool, maximum int) ([]string, bool) {
	if len(values) > maximum {
		return nil, false
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if !safeValue(value, requireNonempty) {
			return nil, false
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, true
}

func safeOptionalValue(value string) bool {
	return value == "" || safeValue(value, true)
}

func safeValue(value string, requireNonempty bool) bool {
	if !utf8.ValidString(value) || len(value) > maxQueryValueBytes || (requireNonempty && strings.TrimSpace(value) == "") {
		return false
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) || r == '\u2028' || r == '\u2029' {
			return false
		}
	}
	return true
}
