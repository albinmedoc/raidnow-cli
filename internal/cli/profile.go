package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/albinmedoc/raidnow-cli/internal/upstream"
	"github.com/spf13/cobra"
)

// ProfileOperations is the read-only upstream surface for profile history and blocks.
type ProfileOperations interface {
	ListHostedHistory(context.Context, string, upstream.CallOptions) ([]domain.ProfileHistoryRecord, error)
	ListJoinedHistory(context.Context, string, upstream.CallOptions) ([]domain.ProfileHistoryRecord, error)
	ListKickedHistory(context.Context, string, upstream.CallOptions) ([]domain.ProfileHistoryRecord, error)
	ListRemovedHistory(context.Context, string, upstream.CallOptions) ([]domain.ProfileHistoryRecord, error)
	ListBlocks(context.Context, string, upstream.CallOptions) ([]domain.BlockRecord, error)
}

type historyView struct {
	domain.ProfileHistoryRecord
	PokemonName string `json:"pokemonName"`
	ImageURL    string `json:"imageUrl,omitempty"`
}

func newHistoryCommand(operations ProfileOperations, metadataFetcher MetadataFetcher, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "history", Short: "Read profile raid history", Args: usageNoArgs, RunE: helpRun}
	command.AddCommand(newHistoryListCommand("hosted", operations, metadataFetcher, tokenFile, requestTimeout, jsonOutput, operations.ListHostedHistory))
	command.AddCommand(newHistoryListCommand("joined", operations, metadataFetcher, tokenFile, requestTimeout, jsonOutput, operations.ListJoinedHistory))
	command.AddCommand(newHistoryListCommand("kicked", operations, metadataFetcher, tokenFile, requestTimeout, jsonOutput, operations.ListKickedHistory))
	command.AddCommand(newHistoryListCommand("removed", operations, metadataFetcher, tokenFile, requestTimeout, jsonOutput, operations.ListRemovedHistory))
	return command
}

func newHistoryListCommand(name string, operations ProfileOperations, metadataFetcher MetadataFetcher, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool, list func(context.Context, string, upstream.CallOptions) ([]domain.ProfileHistoryRecord, error)) *cobra.Command {
	commandName := "history " + name
	command := &cobra.Command{Use: name, Short: "List " + name + " raid history", Args: usageNoArgs}
	command.RunE = func(command *cobra.Command, _ []string) error {
		if operations == nil || metadataFetcher == nil || tokenFile == nil || requestTimeout == nil || *requestTimeout < 0 {
			return newUsageError()
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		records, err := list(command.Context(), token, callOptions(requestTimeout))
		if err != nil {
			return sanitizedPublisherError(err)
		}
		views, err := profileHistoryViews(command.Context(), metadataFetcher, records)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess(commandName, "listed", views, nil))
		}
		for _, view := range views {
			rating := "none"
			if view.Rating != nil {
				rating = fmt.Sprint(*view.Rating)
			}
			if _, err := fmt.Fprintf(command.OutOrStdout(), "%s %s name=%s pokemon=%s trainer=%s level=%d country=%s rating=%s\n", view.RaidID, view.DisplayTime, view.PokemonName, view.PokemonID, view.TrainerName, view.Level, view.Country, rating); err != nil {
				return err
			}
			if view.Guest != nil {
				guestRating := "none"
				if view.Guest.Rating != nil {
					guestRating = fmt.Sprint(*view.Guest.Rating)
				}
				if _, err := fmt.Fprintf(command.OutOrStdout(), "  guest=%s level=%d country=%s rating=%s\n", view.Guest.TrainerName, view.Guest.Level, view.Guest.Country, guestRating); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return command
}

func profileHistoryViews(ctx context.Context, fetcher MetadataFetcher, records []domain.ProfileHistoryRecord) ([]historyView, error) {
	if len(records) == 0 {
		return []historyView{}, nil
	}
	catalog, err := fetchCatalog(ctx, fetcher)
	if err != nil {
		return nil, err
	}
	views := make([]historyView, 0, len(records))
	for _, record := range records {
		entry, ok := catalog.Lookup(record.PokemonID)
		if !ok || strings.TrimSpace(entry.NameEN) == "" {
			return nil, metadataUnavailableError()
		}
		views = append(views, historyView{ProfileHistoryRecord: record, PokemonName: entry.NameEN, ImageURL: entry.ImageURL})
	}
	return views, nil
}

func newBlocksCommand(operations ProfileOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "blocks", Short: "Read blocked trainers", Args: usageNoArgs, RunE: helpRun}
	list := &cobra.Command{Use: "list", Short: "List blocked trainers", Args: usageNoArgs}
	list.RunE = func(command *cobra.Command, _ []string) error {
		if operations == nil || tokenFile == nil || requestTimeout == nil || *requestTimeout < 0 {
			return newUsageError()
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		blocks, err := operations.ListBlocks(command.Context(), token, callOptions(requestTimeout))
		if err != nil {
			return sanitizedPublisherError(err)
		}
		if blocks == nil {
			blocks = []domain.BlockRecord{}
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("blocks list", "listed", blocks, nil))
		}
		for _, block := range blocks {
			if _, err := fmt.Fprintf(command.OutOrStdout(), "%s trainer=%s\n", block.BlockID, block.TrainerName); err != nil {
				return err
			}
		}
		return nil
	}
	command.AddCommand(list)
	return command
}
