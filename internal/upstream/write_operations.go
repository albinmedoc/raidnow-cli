package upstream

import (
	"context"
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
