package upstream

import (
	"context"
	"net/url"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/session"
)

func (client *Client) ListHostedHistory(ctx context.Context, token string, options CallOptions) ([]domain.ProfileHistoryRecord, error) {
	return client.listProfileHistory(ctx, token, MyListGet, false, options)
}

func (client *Client) ListJoinedHistory(ctx context.Context, token string, options CallOptions) ([]domain.ProfileHistoryRecord, error) {
	return client.listProfileHistory(ctx, token, JoinListGet, true, options)
}

func (client *Client) ListKickedHistory(ctx context.Context, token string, options CallOptions) ([]domain.ProfileHistoryRecord, error) {
	return client.listProfileHistory(ctx, token, KickListGet, false, options)
}

func (client *Client) ListRemovedHistory(ctx context.Context, token string, options CallOptions) ([]domain.ProfileHistoryRecord, error) {
	return client.listProfileHistory(ctx, token, DeleteListGet, false, options)
}

func (client *Client) ListBlocks(ctx context.Context, token string, options CallOptions) ([]domain.BlockRecord, error) {
	if !session.AnonymousTokenIsValid(token) {
		return nil, invalidArguments("Ogiltig anonym token.")
	}
	if err := client.validateProfileToken(ctx, token, options); err != nil {
		return nil, err
	}
	raw, err := client.PostForm(ctx, BlockGet, url.Values{"token": {token}}, options)
	if err != nil {
		return nil, err
	}
	return decodeBlocks(raw)
}

func (client *Client) listProfileHistory(ctx context.Context, token string, operation Operation, includeGuest bool, options CallOptions) ([]domain.ProfileHistoryRecord, error) {
	if !session.AnonymousTokenIsValid(token) {
		return nil, invalidArguments("Ogiltig anonym token.")
	}
	if err := client.validateProfileToken(ctx, token, options); err != nil {
		return nil, err
	}
	raw, err := client.PostForm(ctx, operation, url.Values{"lang": {"1"}, "token": {token}}, options)
	if err != nil {
		return nil, err
	}
	return decodeProfileHistory(raw, includeGuest)
}

func (client *Client) validateProfileToken(ctx context.Context, token string, options CallOptions) error {
	raw, err := client.PostForm(ctx, CookieGet, url.Values{"token": {token}}, options)
	if err != nil {
		return err
	}
	return decodeProfileCookie(raw)
}
