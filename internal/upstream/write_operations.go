package upstream

import (
	"context"
	"net/url"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
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
	if len(raw) == 0 {
		return nil
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
	if err := classifyStatus(map[string]any{"status": text, "text": object["text"]}); err != nil {
		return err
	}
	return nil
}
