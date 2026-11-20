package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/albinmedoc/raidnow-cli/internal/session"
	"github.com/albinmedoc/raidnow-cli/internal/upstream"
	"github.com/spf13/cobra"
)

// RaidOperations is the narrow stateless upstream surface used by primitive commands.
type RaidOperations interface {
	CreateMatching(context.Context, string, domain.Trainer, upstream.QueueSelection, upstream.CallOptions) error
	PollMatching(context.Context, string, upstream.QueueSelection, upstream.CallOptions) (domain.QueuePoll, error)
	CancelMatching(context.Context, string, upstream.CallOptions) error
	CheckMatching(context.Context, string, []string, upstream.CallOptions) (domain.QueueCheck, error)
	ListRaids(context.Context, string, upstream.LobbyQuery, upstream.CallOptions) ([]domain.Raid, error)
	JoinRaid(context.Context, string, string, domain.TrainerIdentity, upstream.CallOptions) error
	FindRaid(context.Context, string, string, upstream.CallOptions) (*domain.Raid, error)
	ListRaidPlayers(context.Context, string, string, upstream.CallOptions) (domain.RaidPlayers, error)
	LeaveRaid(context.Context, string, string, upstream.CallOptions) error
}

func newQueueCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "queue", Short: "Manage the Auto Join queue", Args: usageNoArgs}
	command.RunE = helpRun
	command.AddCommand(newQueueJoinCommand(operations, tokenFile, requestTimeout, jsonOutput))
	command.AddCommand(newQueueStatusCommand(operations, tokenFile, requestTimeout, jsonOutput))
	command.AddCommand(newQueueCancelCommand(operations, tokenFile, requestTimeout, jsonOutput))
	return command
}

func newQueueJoinCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "join", Short: "Create an Auto Join queue", Args: usageNoArgs}
	trainerInput := bindTrainerInputFlags(command)
	selection := bindQueueSelectionFlags(command)
	command.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || (*tokenFile == "-" && trainerInput.codeFile == "-") {
			return newUsageError()
		}
		queueSelection, err := selection.value()
		if err != nil {
			return err
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		trainer, err := trainerInput.read(command)
		if err != nil {
			return err
		}
		if err := operations.CreateMatching(command.Context(), token, trainer, queueSelection, callOptions(requestTimeout)); err != nil {
			return sanitizedPublisherError(err)
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("queue join", "created", struct {
				Created bool `json:"created"`
			}{Created: true}, nil))
		}
		_, err = fmt.Fprintln(command.OutOrStdout(), "Kön skapades.")
		return err
	}
	return command
}

func newQueueStatusCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "status", Short: "Poll the Auto Join queue once", Args: usageNoArgs}
	selection := bindQueueSelectionFlags(command)
	command.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) {
			return newUsageError()
		}
		queueSelection, err := selection.value()
		if err != nil {
			return err
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		poll, err := operations.PollMatching(command.Context(), token, queueSelection, callOptions(requestTimeout))
		if err != nil {
			return sanitizedPublisherError(err)
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("queue status", string(poll.State), poll, nil))
		}
		if poll.State == domain.QueueMatched {
			_, err = fmt.Fprintf(command.OutOrStdout(), "matched %s\n", poll.RaidID)
		} else {
			_, err = fmt.Fprintln(command.OutOrStdout(), "waiting")
		}
		return err
	}
	return command
}

func newQueueCancelCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	var pokemon []string
	command := &cobra.Command{Use: "cancel", Short: "Cancel and verify the Auto Join queue", Args: usageNoArgs}
	command.Flags().StringArrayVar(&pokemon, "pokemon", nil, "Pokemon/form ID to verify after cancellation (repeatable)")
	command.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || len(pokemon) == 0 {
			return newUsageError()
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		options := callOptions(requestTimeout)
		if err := operations.CancelMatching(command.Context(), token, options); err != nil {
			return sanitizedPublisherError(err)
		}
		check, err := operations.CheckMatching(command.Context(), token, pokemon, options)
		if err != nil {
			return sanitizedPublisherError(err)
		}
		if check.Present {
			return output.NewError(output.CodeLifecycleConflict, "Kön finns fortfarande kvar hos upstream.")
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("queue cancel", "cancelled", struct {
				Cancelled bool `json:"cancelled"`
			}{Cancelled: true}, nil))
		}
		_, err = fmt.Fprintln(command.OutOrStdout(), "Kön avbröts.")
		return err
	}
	return command
}

func newLobbiesCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "lobbies", Short: "List raid lobbies", Args: usageNoArgs}
	command.RunE = helpRun
	var page, hostLevel int
	var rank string
	var excluded []string
	list := &cobra.Command{Use: "list", Short: "List one raid lobby page", Args: usageNoArgs}
	list.Flags().IntVar(&page, "page", 1, "Lobby page")
	list.Flags().StringVar(&rank, "rank", "0", "Raid rank filter")
	list.Flags().IntVar(&hostLevel, "host-level", 0, "Host level filter")
	list.Flags().StringArrayVar(&excluded, "exclude-pokemon", nil, "Pokemon/form ID to exclude (repeatable)")
	list.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || page < 1 || !validHostLevel(hostLevel) {
			return newUsageError()
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		raids, err := operations.ListRaids(command.Context(), token, upstream.LobbyQuery{Filters: excluded, Page: page, SearchRank: rank, HostLevel: hostLevel}, callOptions(requestTimeout))
		if err != nil {
			return sanitizedPublisherError(err)
		}
		if raids == nil {
			raids = []domain.Raid{}
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("lobbies list", "listed", raids, nil))
		}
		for _, raid := range raids {
			if _, err := fmt.Fprintf(command.OutOrStdout(), "%s rank=%s pokemon=%s players=%d/%d\n", raid.ID, raid.Rank, raid.PokemonID, raid.Count, raid.MaxUsers); err != nil {
				return err
			}
		}
		return nil
	}
	command.AddCommand(list)
	return command
}

func newRaidCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "raid", Short: "Join, inspect, or leave a raid", Args: usageNoArgs}
	command.RunE = helpRun
	command.AddCommand(newRaidJoinCommand(operations, tokenFile, requestTimeout, jsonOutput))
	command.AddCommand(newRaidStatusCommand(operations, tokenFile, requestTimeout, jsonOutput))
	command.AddCommand(newRaidLeaveCommand(operations, tokenFile, requestTimeout, jsonOutput))
	return command
}

func newRaidJoinCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	var raidID, name, country string
	var level int
	command := &cobra.Command{Use: "join", Short: "Join one raid", Args: usageNoArgs}
	flags := command.Flags()
	flags.StringVar(&raidID, "raid-id", "", "Exact raid ID")
	flags.StringVar(&name, "trainer-name", "", "Trainer name for this invocation")
	flags.IntVar(&level, "trainer-level", 0, "Trainer level for this invocation")
	flags.StringVar(&country, "trainer-country", "", "Trainer country code for this invocation")
	command.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || raidID == "" {
			return newUsageError()
		}
		identity, err := domain.NewTrainerIdentity(name, level, country)
		if err != nil {
			return invalidTrainerInputError()
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		if err := operations.JoinRaid(command.Context(), token, raidID, identity, callOptions(requestTimeout)); err != nil {
			return sanitizedPublisherError(err)
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("raid join", "joined", struct {
				Joined bool   `json:"joined"`
				RaidID string `json:"raidId"`
			}{Joined: true, RaidID: raidID}, nil))
		}
		_, err = fmt.Fprintln(command.OutOrStdout(), "Anslutningen skickades.")
		return err
	}
	return command
}

func newRaidStatusCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	var raidID string
	command := &cobra.Command{Use: "status", Short: "Read one raid and its players", Args: usageNoArgs}
	command.Flags().StringVar(&raidID, "raid-id", "", "Exact raid ID")
	command.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || raidID == "" {
			return newUsageError()
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		options := callOptions(requestTimeout)
		raid, err := operations.FindRaid(command.Context(), token, raidID, options)
		if err != nil {
			return sanitizedPublisherError(err)
		}
		if raid == nil {
			return output.NewError(output.CodeNotFound, "Raiden hittades inte.")
		}
		players, err := operations.ListRaidPlayers(command.Context(), token, raidID, options)
		if err != nil {
			return sanitizedPublisherError(err)
		}
		data := struct {
			Raid    domain.Raid        `json:"raid"`
			Players domain.RaidPlayers `json:"players"`
		}{Raid: *raid, Players: players}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("raid status", "found", data, nil))
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "%s pokemon=%s players=%d ready=%d\n", raid.ID, raid.PokemonID, players.Count, players.ReadyCount)
		return err
	}
	return command
}

func newRaidLeaveCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	var raidID string
	command := &cobra.Command{Use: "leave", Short: "Leave and verify one raid", Args: usageNoArgs}
	command.Flags().StringVar(&raidID, "raid-id", "", "Exact raid ID")
	command.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || raidID == "" {
			return newUsageError()
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		options := callOptions(requestTimeout)
		if err := operations.LeaveRaid(command.Context(), token, raidID, options); err != nil {
			return sanitizedPublisherError(err)
		}
		raid, err := operations.FindRaid(command.Context(), token, raidID, options)
		if err != nil {
			return sanitizedPublisherError(err)
		}
		if raid != nil {
			return output.NewError(output.CodeLifecycleConflict, "Raiden finns fortfarande kvar för sessionen.")
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("raid leave", "left", struct {
				Left   bool   `json:"left"`
				RaidID string `json:"raidId"`
			}{Left: true, RaidID: raidID}, nil))
		}
		_, err = fmt.Fprintln(command.OutOrStdout(), "Raiden lämnades.")
		return err
	}
	return command
}

type queueSelectionFlags struct {
	ranks      []string
	pokemon    []string
	guestScore string
}

func bindQueueSelectionFlags(command *cobra.Command) *queueSelectionFlags {
	selection := &queueSelectionFlags{}
	command.Flags().StringArrayVar(&selection.ranks, "rank", nil, "Raid rank (repeatable, paired with --pokemon)")
	command.Flags().StringArrayVar(&selection.pokemon, "pokemon", nil, "Pokemon/form ID (repeatable, paired with --rank)")
	command.Flags().StringVar(&selection.guestScore, "guest-score", "", "Optional guest score")
	return selection
}

func (selection *queueSelectionFlags) value() (upstream.QueueSelection, error) {
	if selection == nil || len(selection.ranks) == 0 || len(selection.ranks) != len(selection.pokemon) {
		return upstream.QueueSelection{}, newUsageError()
	}
	return upstream.QueueSelection{Ranks: selection.ranks, PokemonIDs: selection.pokemon, GuestEvalStar: selection.guestScore}, nil
}

func readCommandToken(command *cobra.Command, path string) (string, error) {
	token, err := session.ReadTokenInput(path, command.InOrStdin())
	if err != nil {
		return "", output.NewError(output.CodeInvalidInput, "Ogiltig tokeninmatning.")
	}
	return token, nil
}

func invalidPrimitiveSetup(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration) bool {
	return operations == nil || tokenFile == nil || requestTimeout == nil || *requestTimeout < 0
}

func callOptions(requestTimeout *time.Duration) upstream.CallOptions {
	return upstream.CallOptions{Timeout: *requestTimeout}
}

func validHostLevel(level int) bool {
	return level == 0 || level == 40 || level == 50 || level == 60 || level == 70
}

func helpRun(command *cobra.Command, _ []string) error { return command.Help() }
