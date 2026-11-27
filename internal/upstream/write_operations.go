package upstream

import (
	"bytes"
	"context"
	"encoding/json"
	"net/url"
	"strconv"
	"strings"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/albinmedoc/raidnow-cli/internal/session"
)

// PublishTrainer publishes one invocation-scoped trainer identity without
// retaining token, cookies, or trainer state.
func (client *Client) PublishTrainer(ctx context.Context, token string, trainer domain.Trainer, options CallOptions) error {
	if ctx == nil || !session.AnonymousTokenIsValid(token) ||
		trainer.NameFormValue() == "" || trainer.CodeFormValue() == "" ||
		trainer.LevelFormValue() == "0" || trainer.CountryFormValue() == "" {
		return invalidArguments("Ogiltig tränarinmatning.")
	}
	form := url.Values{
		"input_tid":          {trainer.NameFormValue()},
		"input_level":        {trainer.LevelFormValue()},
		"input_code":         {trainer.CodeFormValue()},
		"input_country_code": {trainer.CountryFormValue()},
		"token":              {token},
	}
	raw, err := client.PostForm(ctx, UserInfoPost, form, options)
	if err != nil {
		return err
	}
	return acknowledgeWrite(UserInfoPost, raw, token, trainer.CodeFormValue())
}

// CreateMatching creates one Auto Join queue selection without retrying writes.
func (client *Client) CreateMatching(ctx context.Context, token string, trainer domain.Trainer, selection QueueSelection, options CallOptions) error {
	ranks, pokemon, selectionOK := normalizeSelectionPairs(selection.Ranks, selection.PokemonIDs)
	if ctx == nil || !session.AnonymousTokenIsValid(token) || !selectionOK || len(ranks) == 0 || !safeOptionalValue(selection.GuestEvalStar) ||
		selection.MinimumHostRating < -1 || selection.MinimumHostRating > 5 || (selection.HostCountry != 0 && selection.HostCountry != 1) ||
		selection.WeatherBoost < 0 || selection.WeatherBoost > 2 || !allowedHostLevel(selection.HostLevel) ||
		trainer.NameFormValue() == "" || trainer.CodeFormValue() == "" || trainer.LevelFormValue() == "0" || trainer.CountryFormValue() == "" {
		return invalidArguments("Ogiltigt köurval.")
	}
	form := url.Values{
		"rank[]": ranks, "pokemon[]": pokemon, "code": {trainer.CodeFormValue()}, "tid": {trainer.NameFormValue()},
		"level": {trainer.LevelFormValue()}, "priority": {"0"}, "lang": {"1"}, "gscore": {selection.GuestEvalStar},
		"country_code": {trainer.CountryFormValue()}, "plugs": {"[]"}, "host_country": {strconv.Itoa(selection.HostCountry)}, "weather_boost": {strconv.Itoa(selection.WeatherBoost)},
		"guest6": {"0"}, "regional_raid": {"0"}, "token": {token}, "hosttl": {strconv.Itoa(selection.HostLevel)},
	}
	for rating := -1; rating < selection.MinimumHostRating; rating++ {
		form["hosteval[]"] = append(form["hosteval[]"], strconv.Itoa(rating))
	}
	raw, err := client.PostForm(ctx, LineMatchingCreate, form, options)
	if err != nil {
		return err
	}
	return acknowledgeWrite(LineMatchingCreate, raw, token, trainer.CodeFormValue())
}

// CancelMatching cancels the current Auto Join queue without retrying writes.
func (client *Client) CancelMatching(ctx context.Context, token string, options CallOptions) error {
	if ctx == nil || !session.AnonymousTokenIsValid(token) {
		return invalidArguments("Ogiltig anonym token.")
	}
	raw, err := client.PostForm(ctx, LineMatchingCancel, url.Values{"lang": {"1"}, "token": {token}}, options)
	if err != nil {
		return err
	}
	return acknowledgeWrite(LineMatchingCancel, raw, token)
}

// JoinRaid submits one trainer identity to a raid without retrying writes.
func (client *Client) JoinRaid(ctx context.Context, token, raidID string, identity domain.TrainerIdentity, options CallOptions) error {
	if ctx == nil || !session.AnonymousTokenIsValid(token) || !safeValue(raidID, true) || identity.NameFormValue() == "" || identity.LevelFormValue() == "0" || identity.CountryFormValue() == "" {
		return invalidArguments("Ogiltig raidanslutning.")
	}
	form := url.Values{
		"parid": {raidID}, "tid": {identity.NameFormValue()}, "show": {"0"}, "level": {identity.LevelFormValue()},
		"color": {"0"}, "memo": {""}, "lang": {"1"}, "country_code": {identity.CountryFormValue()},
		"regional_raid": {"0"}, "token": {token},
	}
	raw, err := client.PostForm(ctx, ChildPostFast, form, options)
	if err != nil {
		return err
	}
	return acknowledgeWrite(ChildPostFast, raw, token)
}

// LeaveRaid removes the anonymous trainer from one raid without retrying writes.
func (client *Client) LeaveRaid(ctx context.Context, token, raidID string, options CallOptions) error {
	if ctx == nil || !session.AnonymousTokenIsValid(token) || !safeValue(raidID, true) {
		return invalidArguments("Ogiltigt raid-id.")
	}
	raw, err := client.PostForm(ctx, GuestRaidDelPost, url.Values{"edit": {raidID}, "lang": {"1"}, "token": {token}}, options)
	if err != nil {
		return err
	}
	return acknowledgeWrite(GuestRaidDelPost, raw, token)
}

// ReadyRaid marks the anonymous trainer ready in one raid without retrying writes.
func (client *Client) ReadyRaid(ctx context.Context, token, raidID string, options CallOptions) error {
	if ctx == nil || !session.AnonymousTokenIsValid(token) || !safeValue(raidID, true) {
		return invalidArguments("Ogiltigt raid-id.")
	}
	raw, err := client.PostForm(ctx, GuestReadyPost, url.Values{"edit": {raidID}, "ready": {"1"}, "lang": {"1"}, "token": {token}}, options)
	if err != nil {
		return err
	}
	return acknowledgeWrite(GuestReadyPost, raw, token)
}

// CreateHostedRaid creates one host-owned raid lobby without retrying writes.
func (client *Client) CreateHostedRaid(ctx context.Context, token string, trainer domain.Trainer, input HostLobbyInput, options CallOptions) (domain.Raid, error) {
	filters, filtersOK := normalizeValuesBounded(input.Filters, true, maxLobbyFilters)
	weatherBoosts, boostsOK := normalizeValues(input.WeatherBoosts, true)
	if ctx == nil || !session.AnonymousTokenIsValid(token) || trainer.NameFormValue() == "" || trainer.CodeFormValue() == "" ||
		trainer.LevelFormValue() == "0" || trainer.CountryFormValue() == "" || !safeValue(input.StartDate, true) ||
		!safeValue(input.Rank, true) || !safeValue(input.PokemonID, true) || input.Span <= 0 || input.MaxUsers < 1 || input.MaxUsers > 10 ||
		input.Color < 0 || input.Color > 3 || input.Weather < 0 || input.Weather > 1 || input.CurrentWeather < 0 || input.CurrentWeather > 9 ||
		input.GymColor < 0 || input.GymColor > 3 || input.PassType < 0 || input.PassType > 1 || (input.PassType == 0 && input.MaxUsers == 10) ||
		!validHostedGuestLevel(input.GuestLevel) || input.GuestScore < -1 || input.GuestScore > 5 || !filtersOK || !boostsOK {
		return domain.Raid{}, invalidArguments("Ogiltig värdlobby.")
	}
	form := url.Values{
		"start_date": {input.StartDate}, "rank": {input.Rank}, "pokemon": {input.PokemonID}, "span": {strconv.Itoa(input.Span)},
		"code": {trainer.CodeFormValue()}, "level": {trainer.LevelFormValue()}, "color": {strconv.Itoa(input.Color)}, "tid": {trainer.NameFormValue()},
		"weather": {strconv.Itoa(input.Weather)}, "current_weather": {strconv.Itoa(input.CurrentWeather)}, "maxuser": {strconv.Itoa(input.MaxUsers)},
		"gpcolor": {strconv.Itoa(input.GymColor)}, "passtype": {strconv.Itoa(input.PassType)}, "private": {zeroOne(input.Private)},
		"possible": {strconv.FormatBool(input.Possible)}, "lang": {"1"}, "country_code": {trainer.CountryFormValue()},
		"glevel": {strconv.Itoa(input.GuestLevel)}, "gescore": {strconv.Itoa(input.GuestScore)}, "invitation_only": {zeroOne(input.InvitationOnly)},
		"hot": {zeroOne(input.Hot)}, "mega_pokemon_entry": {zeroOne(input.MegaPokemonEntry)}, "regional_raid": {"0"},
		"token": {token},
	}
	if len(filters) > 0 {
		form["filter[]"] = filters
	}
	if len(weatherBoosts) > 0 {
		form["weather_boost[]"] = weatherBoosts
	}
	raw, err := client.PostForm(ctx, ParPost, form, options)
	if err != nil {
		return domain.Raid{}, err
	}
	if err := acknowledgeWrite(ParPost, raw, token, trainer.CodeFormValue()); err != nil {
		return domain.Raid{}, err
	}
	object, err := decodeObject(raw)
	if err != nil {
		return domain.Raid{}, err
	}
	raids, err := decodeRaids(raw)
	if err != nil {
		return domain.Raid{}, err
	}
	index, ok := optionalStrictInt(object, "now")
	if !ok || index == nil || *index < 0 || *index >= len(raids) {
		return domain.Raid{}, upstreamRejection()
	}
	return raids[*index], nil
}

// CloseHostedRaid closes one host-owned lobby without retrying writes.
func (client *Client) CloseHostedRaid(ctx context.Context, token, raidID string, options CallOptions) error {
	if ctx == nil || !session.AnonymousTokenIsValid(token) || !safeValue(raidID, true) {
		return invalidArguments("Ogiltigt raid-id.")
	}
	raw, err := client.PostForm(ctx, ParDelete, url.Values{"parid": {raidID}, "token": {token}}, options)
	if err != nil {
		return err
	}
	return acknowledgeOpaqueWrite(ParDelete, raw, token)
}

// StartHostedRaid announces that one hosted raid is starting.
func (client *Client) StartHostedRaid(ctx context.Context, token, raidID string, options CallOptions) error {
	if ctx == nil || !session.AnonymousTokenIsValid(token) || !safeValue(raidID, true) {
		return invalidArguments("Ogiltigt raid-id.")
	}
	raw, err := client.PostForm(ctx, HostReadyPost, url.Values{"edit": {raidID}, "lang": {"1"}, "token": {token}}, options)
	if err != nil {
		return err
	}
	return acknowledgeWrite(HostReadyPost, raw, token)
}

// KickHostedGuest removes one exact guest from a hosted lobby.
func (client *Client) KickHostedGuest(ctx context.Context, token, raidID, guestID string, options CallOptions) error {
	if ctx == nil || !session.AnonymousTokenIsValid(token) || !safeValue(raidID, true) || !safeValue(guestID, true) {
		return invalidArguments("Ogiltigt raid- eller gäst-id.")
	}
	raw, err := client.PostForm(ctx, HostKicksGuestPost, url.Values{"edit": {raidID}, "kick": {guestID}, "lang": {"1"}, "token": {token}}, options)
	if err != nil {
		return err
	}
	return acknowledgeWrite(HostKicksGuestPost, raw, token)
}

func acknowledgeOpaqueWrite(operation Operation, raw []byte, secrets ...string) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || !json.Valid(trimmed) {
		return nil
	}
	object, err := decodeObject(trimmed)
	if err != nil {
		return nil
	}
	if _, present := object["status"]; !present {
		return nil
	}
	return acknowledgeWrite(operation, trimmed, secrets...)
}

func validHostedGuestLevel(value int) bool {
	switch value {
	case 0, 40, 50, 60, 70:
		return true
	default:
		return false
	}
}

func zeroOne(value bool) string {
	if value {
		return "1"
	}
	return "0"
}

func acknowledgeWrite(operation Operation, raw []byte, secrets ...string) error {
	if len(raw) == 0 {
		descriptor, err := operation.Descriptor()
		if err != nil {
			return err
		}
		if descriptor.AllowsEmptyResponse() {
			return nil
		}
		return upstreamRejection()
	}
	object, err := decodeObject(raw)
	if err != nil {
		return err
	}
	status, present := object["status"]
	if !present {
		return upstreamRejection()
	}
	text, ok := exactScalar(status)
	if !ok {
		return upstreamRejection()
	}
	if !strings.EqualFold(strings.TrimSpace(text), "success") {
		if err := structuredRejection(object); output.PublicError(err).Code == output.CodeAuthenticationUnsupported {
			return err
		}
		detail, _ := exactScalar(object["text"])
		return upstreamRejectionDetail(detail, secrets...)
	}
	return nil
}
