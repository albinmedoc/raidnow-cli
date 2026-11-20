package cli

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/spf13/cobra"
)

type bossView struct {
	Rank         string `json:"rank"`
	PokemonID    string `json:"pokemonId"`
	PokemonName  string `json:"pokemonName"`
	ImageURL     string `json:"imageUrl"`
	WaitingCount int    `json:"waitingCount"`
}

func newBossesCommand(operations RaidOperations, metadataFetcher MetadataFetcher, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "bosses", Short: "List current raid bosses", Args: usageNoArgs}
	command.RunE = helpRun
	var rank int
	list := &cobra.Command{Use: "list", Short: "List current Raid NOW bosses", Args: usageNoArgs}
	list.Flags().IntVar(&rank, "rank", 0, "Exact numeric raid rank (0 lists all)")
	list.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || metadataFetcher == nil || rank < 0 {
			return newUsageError()
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		bosses, err := operations.ListCurrentRaidBosses(command.Context(), token, callOptions(requestTimeout))
		if err != nil {
			return sanitizedPublisherError(err)
		}
		filtered := make([]domain.RaidBoss, 0, len(bosses))
		for _, boss := range bosses {
			if rank == 0 || boss.Rank == strconv.Itoa(rank) {
				filtered = append(filtered, boss)
			}
		}
		views, err := bossViews(command, metadataFetcher, filtered)
		if err != nil {
			return err
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("bosses list", "listed", views, nil))
		}
		for _, boss := range views {
			if _, err := fmt.Fprintf(command.OutOrStdout(), "%s rank=%s pokemon=%s waiting=%d\n", boss.PokemonName, boss.Rank, boss.PokemonID, boss.WaitingCount); err != nil {
				return err
			}
		}
		return nil
	}
	command.AddCommand(list)
	return command
}

func bossViews(command *cobra.Command, fetcher MetadataFetcher, bosses []domain.RaidBoss) ([]bossView, error) {
	if len(bosses) == 0 {
		return []bossView{}, nil
	}
	catalog, err := fetchCatalog(command.Context(), fetcher)
	if err != nil {
		return nil, err
	}
	views := make([]bossView, 0, len(bosses))
	for _, boss := range bosses {
		entry, ok := catalog.Lookup(boss.PokemonID)
		if !ok || strings.TrimSpace(entry.NameEN) == "" {
			return nil, metadataUnavailableError()
		}
		views = append(views, bossView{Rank: boss.Rank, PokemonID: boss.PokemonID, PokemonName: entry.NameEN, ImageURL: entry.ImageURL, WaitingCount: boss.WaitingCount})
	}
	sort.SliceStable(views, func(i, j int) bool {
		left, _ := strconv.Atoi(views[i].Rank)
		right, _ := strconv.Atoi(views[j].Rank)
		if left != right {
			return left > right
		}
		return views[i].PokemonName < views[j].PokemonName
	})
	return views, nil
}
