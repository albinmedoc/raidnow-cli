package cli

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/metadata"
	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/albinmedoc/raidnow-cli/internal/session"
	"github.com/albinmedoc/raidnow-cli/internal/upstream"
	"github.com/spf13/cobra"
)

// RaidOperations is the narrow stateless upstream surface used by primitive commands.
type RaidOperations interface {
	ListCurrentRaidBosses(context.Context, string, upstream.CallOptions) ([]domain.RaidBoss, error)
	CreateMatching(context.Context, string, domain.Trainer, upstream.QueueSelection, upstream.CallOptions) error
	PollMatching(context.Context, string, upstream.QueueSelection, upstream.CallOptions) (domain.QueuePoll, error)
	CancelMatching(context.Context, string, upstream.CallOptions) error
	CheckMatching(context.Context, string, []string, upstream.CallOptions) (domain.QueueCheck, error)
	ListRaids(context.Context, string, upstream.LobbyQuery, upstream.CallOptions) ([]domain.Raid, error)
	JoinRaid(context.Context, string, string, domain.TrainerIdentity, upstream.CallOptions) error
	FindRaid(context.Context, string, string, upstream.CallOptions) (*domain.Raid, error)
	ListRaidPlayers(context.Context, string, string, upstream.CallOptions) (domain.RaidPlayers, error)
	LeaveRaid(context.Context, string, string, upstream.CallOptions) error
	ReadyRaid(context.Context, string, string, upstream.CallOptions) error
	CreateHostedRaid(context.Context, string, domain.Trainer, upstream.HostLobbyInput, upstream.CallOptions) (domain.Raid, error)
	CloseHostedRaid(context.Context, string, string, upstream.CallOptions) error
	StartHostedRaid(context.Context, string, string, upstream.CallOptions) error
	KickHostedGuest(context.Context, string, string, string, upstream.CallOptions) error
}

type MetadataFetcher interface {
	Catalog(context.Context) (metadata.Catalog, error)
}

func newQueueCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "queue", Short: "Manage the Auto Join queue", Args: usageNoArgs}
	command.RunE = helpRun
	command.AddCommand(newQueueJoinCommand(operations, tokenFile, requestTimeout, jsonOutput))
	command.AddCommand(newQueueStatusCommand(operations, tokenFile, requestTimeout, jsonOutput))
	command.AddCommand(newQueueCheckCommand(operations, tokenFile, requestTimeout, jsonOutput))
	command.AddCommand(newQueueWaitCommand(operations, tokenFile, requestTimeout, jsonOutput))
	command.AddCommand(newQueueCancelCommand(operations, tokenFile, requestTimeout, jsonOutput))
	return command
}

func newQueueJoinCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "join", Short: "Create an Auto Join queue", Args: usageNoArgs}
	trainerInput := bindTrainerInputFlags(command)
	selection := bindQueueSelectionFlags(command)
	filters := bindMatchingFilterFlags(command)
	command.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || (*tokenFile == "-" && trainerInput.codeFile == "-") {
			return newUsageError()
		}
		queueSelection, err := selection.value()
		if err != nil {
			return err
		}
		queueSelection, err = filters.apply(queueSelection)
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

func newQueueCheckCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	var pokemon []string
	command := &cobra.Command{Use: "check", Short: "Check whether the Auto Join queue is present", Args: usageNoArgs}
	command.Flags().StringArrayVar(&pokemon, "pokemon", nil, "Pokemon/form ID to check (repeatable)")
	command.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || len(pokemon) == 0 {
			return newUsageError()
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		check, err := operations.CheckMatching(command.Context(), token, pokemon, callOptions(requestTimeout))
		if err != nil {
			return sanitizedPublisherError(err)
		}
		status := "absent"
		if check.Present {
			status = "present"
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("queue check", status, check, nil))
		}
		_, err = fmt.Fprintln(command.OutOrStdout(), status)
		return err
	}
	return command
}

func newQueueCancelCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	var pokemon []string
	verify := true
	command := &cobra.Command{Use: "cancel", Short: "Cancel and verify the Auto Join queue", Args: usageNoArgs}
	command.Flags().StringArrayVar(&pokemon, "pokemon", nil, "Pokemon/form ID to verify after cancellation (repeatable)")
	command.Flags().BoolVar(&verify, "verify", true, "Verify that the queue is absent after cancellation")
	command.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || (verify && len(pokemon) == 0) {
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
		if !verify {
			if *jsonOutput {
				return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("queue cancel", "requested", struct {
					Requested bool `json:"requested"`
					Verified  bool `json:"verified"`
				}{Requested: true, Verified: false}, nil))
			}
			_, err = fmt.Fprintln(command.OutOrStdout(), "Avbrytningsbegäran skickades.")
			return err
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

func newLobbiesCommand(operations RaidOperations, metadataFetcher MetadataFetcher, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "lobbies", Short: "List raid lobbies", Args: usageNoArgs}
	command.RunE = helpRun
	var page, hostLevel, minimumHostRating int
	var rank string
	var excluded []string
	var includeMetadata bool
	list := &cobra.Command{Use: "list", Short: "List one raid lobby page", Args: usageNoArgs}
	list.Flags().IntVar(&page, "page", 1, "Lobby page")
	list.Flags().StringVar(&rank, "rank", "0", "Raid rank filter")
	list.Flags().IntVar(&hostLevel, "host-level", 0, "Host level filter")
	list.Flags().IntVar(&minimumHostRating, "min-host-rating", -1, "Minimum host rating from -1 through 5 (-1 disables the filter)")
	list.Flags().StringArrayVar(&excluded, "exclude-pokemon", nil, "Pokemon/form ID to exclude (repeatable)")
	list.Flags().BoolVar(&includeMetadata, "include-metadata", true, "Include English Pokemon names from homepage metadata")
	list.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || page < 1 || !validHostLevel(hostLevel) || minimumHostRating < -1 || minimumHostRating > 5 {
			return newUsageError()
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		raids, err := operations.ListRaids(command.Context(), token, upstream.LobbyQuery{Filters: excluded, Page: page, SearchRank: rank, HostLevel: hostLevel, HostEvaluate: lobbyHostEvaluate(minimumHostRating)}, callOptions(requestTimeout))
		if err != nil {
			return sanitizedPublisherError(err)
		}
		if raids == nil {
			raids = []domain.Raid{}
		}
		views := make([]raidView, 0, len(raids))
		if includeMetadata {
			views, err = raidViews(command.Context(), metadataFetcher, raids)
			if err != nil {
				return err
			}
		} else {
			for _, raid := range raids {
				views = append(views, raidView{Raid: raid})
			}
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("lobbies list", "listed", views, nil))
		}
		for _, raid := range views {
			if includeMetadata {
				if _, err := fmt.Fprintf(command.OutOrStdout(), "%s rank=%s name=%s pokemon=%s players=%d/%d\n", raid.ID, raid.Rank, raid.PokemonName, raid.PokemonID, raid.Count, raid.MaxUsers); err != nil {
					return err
				}
				continue
			}
			if _, err := fmt.Fprintf(command.OutOrStdout(), "%s rank=%s pokemon=%s players=%d/%d\n", raid.ID, raid.Rank, raid.PokemonID, raid.Count, raid.MaxUsers); err != nil {
				return err
			}
		}
		return nil
	}
	command.AddCommand(list)
	return command
}

func newHostCommand(operations RaidOperations, metadataFetcher MetadataFetcher, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "host", Short: "Create and manage a hosted raid lobby", Args: usageNoArgs}
	command.RunE = helpRun
	command.AddCommand(newHostCreateCommand(operations, metadataFetcher, tokenFile, requestTimeout, jsonOutput))
	command.AddCommand(newHostStatusCommand(operations, tokenFile, requestTimeout, jsonOutput))
	command.AddCommand(newHostCloseCommand(operations, tokenFile, requestTimeout, jsonOutput))
	command.AddCommand(newHostStartCommand(operations, tokenFile, requestTimeout, jsonOutput))
	command.AddCommand(newHostKickCommand(operations, tokenFile, requestTimeout, jsonOutput))
	return command
}

func newHostCreateCommand(operations RaidOperations, metadataFetcher MetadataFetcher, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	var startDate, pokemonID, currentWeatherName, gymColorName, passTypeName, guestRatingName string
	var span, maxUsers, guestLevel int
	var private, weatherBoosted, invitationOnly, hot, megaPokemonEntry bool
	command := &cobra.Command{Use: "create", Short: "Create one hosted raid lobby", Args: usageNoArgs}
	trainerInput := bindTrainerInputFlags(command)
	flags := command.Flags()
	flags.StringVar(&startDate, "start-date", "", "Raid start date as YYYY/MM/DD HH:MM:SS")
	flags.StringVar(&pokemonID, "pokemon", "", "Exact Pokemon/form ID from Raid NOW metadata")
	flags.IntVar(&span, "span", 45, "Raid availability span in minutes")
	flags.StringVar(&currentWeatherName, "current-weather", "unknown", "In-game weather: unknown|sunny|clear-night|partly-cloudy|partly-cloudy-night|cloudy|rain|snow|windy|fog")
	flags.IntVar(&maxUsers, "max-users", 5, "Maximum lobby users (1-10)")
	flags.StringVar(&gymColorName, "gym-color", "neutral", "Gym team: neutral|valor|mystic|instinct")
	flags.StringVar(&passTypeName, "pass-type", "remote", "Host pass type: remote|local")
	flags.BoolVar(&private, "private", false, "Create a private lobby")
	flags.BoolVar(&weatherBoosted, "weather-boosted", false, "Mark the raid boss as weather boosted")
	flags.IntVar(&guestLevel, "guest-level", 5, "Minimum guest level: 5|40|50|60|70")
	flags.StringVar(&guestRatingName, "guest-rating", "all", "Minimum guest rating: all|0|1|2|3|4|5")
	flags.BoolVar(&invitationOnly, "invitation-only", false, "Show invitation-only raids")
	flags.BoolVar(&hot, "hot", false, "Enable Raid NOW HOT/group-code flow")
	flags.BoolVar(&megaPokemonEntry, "mega-pokemon-entry", false, "Allow Mega Pokemon in the raid")
	command.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || strings.TrimSpace(startDate) == "" || strings.TrimSpace(pokemonID) == "" ||
			span <= 0 || maxUsers < 1 || maxUsers > 10 || (*tokenFile == "-" && trainerInput.codeFile == "-") {
			return newUsageError()
		}
		currentWeather, weatherOK := hostCurrentWeather(currentWeatherName)
		gymColor, gymOK := hostGymColor(gymColorName)
		passType, passOK := hostPassType(passTypeName)
		guestLevelValue, levelOK := hostGuestLevel(guestLevel)
		guestRating, ratingOK := hostGuestRating(guestRatingName)
		if !weatherOK || !gymOK || !passOK || !levelOK || !ratingOK || (passType == 0 && maxUsers == 10) {
			return newUsageError()
		}
		catalog, err := fetchCatalog(command.Context(), metadataFetcher)
		if err != nil {
			return err
		}
		entry, ok := catalog.Lookup(strings.TrimSpace(pokemonID))
		if !ok || strings.TrimSpace(entry.CurrentRaidRank) == "" || !entry.HasWeatherBoosts {
			return metadataUnavailableError()
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		trainer, err := trainerInput.read(command)
		if err != nil {
			return err
		}
		weather := 0
		if weatherBoosted {
			weather = 1
		}
		input := upstream.HostLobbyInput{
			StartDate: startDate, Rank: entry.CurrentRaidRank, PokemonID: entry.ID, Span: span,
			Color: 0, Weather: weather, CurrentWeather: currentWeather, MaxUsers: maxUsers,
			GymColor: gymColor, PassType: passType, Private: private, Possible: false,
			GuestLevel: guestLevelValue, GuestScore: guestRating, InvitationOnly: invitationOnly, Hot: hot,
			MegaPokemonEntry: megaPokemonEntry, WeatherBoosts: append([]string(nil), entry.WeatherBoosts...),
		}
		raid, err := operations.CreateHostedRaid(command.Context(), token, trainer, input, callOptions(requestTimeout))
		if err != nil {
			return sanitizedPublisherError(err)
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("host create", "created", struct {
				Created bool   `json:"created"`
				RaidID  string `json:"raidId"`
			}{Created: true, RaidID: raid.ID}, nil))
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "Lobbyn skapades: %s\n", raid.ID)
		return err
	}
	return command
}

func newHostStatusCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	var raidID string
	command := &cobra.Command{Use: "status", Short: "Read one hosted lobby's guests", Args: usageNoArgs}
	command.Flags().StringVar(&raidID, "raid-id", "", "Exact raid ID")
	command.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || raidID == "" {
			return newUsageError()
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		players, err := operations.ListRaidPlayers(command.Context(), token, raidID, callOptions(requestTimeout))
		if err != nil {
			return sanitizedPublisherError(err)
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("host status", "listed", players, nil))
		}
		if _, err = fmt.Fprintf(command.OutOrStdout(), "players=%d ready=%d deleted=%t\n", players.Count, players.ReadyCount, players.RaidDeleted); err != nil {
			return err
		}
		for _, player := range players.Players {
			rating := "none"
			if player.Rating != nil {
				rating = fmt.Sprint(*player.Rating)
			}
			if _, err = fmt.Fprintf(command.OutOrStdout(), "guest-id=%s %s level=%d country=%s rating=%s ready=%t\n", player.ID, player.TrainerName, player.Level, player.Country, rating, player.Ready); err != nil {
				return err
			}
		}
		return nil
	}
	return command
}

func newHostCloseCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	return newHostRaidActionCommand("close", "Close one hosted lobby", "closed", "Lobbyn stängdes.", operations.CloseHostedRaid, operations, tokenFile, requestTimeout, jsonOutput)
}

func newHostStartCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	return newHostRaidActionCommand("start", "Announce that a hosted raid is starting", "started", "Raidstart markerad.", operations.StartHostedRaid, operations, tokenFile, requestTimeout, jsonOutput)
}

func newHostRaidActionCommand(name, short, status, human string, action func(context.Context, string, string, upstream.CallOptions) error, operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	var raidID string
	command := &cobra.Command{Use: name, Short: short, Args: usageNoArgs}
	command.Flags().StringVar(&raidID, "raid-id", "", "Exact raid ID")
	command.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || action == nil || raidID == "" {
			return newUsageError()
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		if err := action(command.Context(), token, raidID, callOptions(requestTimeout)); err != nil {
			return sanitizedPublisherError(err)
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("host "+name, status, struct {
				RaidID string `json:"raidId"`
			}{RaidID: raidID}, nil))
		}
		_, err = fmt.Fprintln(command.OutOrStdout(), human)
		return err
	}
	return command
}

func newHostKickCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	var raidID, guestID string
	command := &cobra.Command{Use: "kick", Short: "Kick one guest from a hosted lobby", Args: usageNoArgs}
	command.Flags().StringVar(&raidID, "raid-id", "", "Exact raid ID")
	command.Flags().StringVar(&guestID, "guest-id", "", "Exact guest ID from host status")
	command.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || raidID == "" || guestID == "" {
			return newUsageError()
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		if err := operations.KickHostedGuest(command.Context(), token, raidID, guestID, callOptions(requestTimeout)); err != nil {
			return sanitizedPublisherError(err)
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("host kick", "kicked", struct {
				RaidID  string `json:"raidId"`
				GuestID string `json:"guestId"`
			}{RaidID: raidID, GuestID: guestID}, nil))
		}
		_, err = fmt.Fprintln(command.OutOrStdout(), "Gästen kickades.")
		return err
	}
	return command
}

func newRaidCommand(operations RaidOperations, metadataFetcher MetadataFetcher, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "raid", Short: "Join, inspect, or leave a raid", Args: usageNoArgs}
	command.RunE = helpRun
	command.AddCommand(newRaidJoinCommand(operations, tokenFile, requestTimeout, jsonOutput))
	command.AddCommand(newRaidGetCommand(operations, tokenFile, requestTimeout, jsonOutput))
	command.AddCommand(newRaidPlayersCommand(operations, tokenFile, requestTimeout, jsonOutput))
	command.AddCommand(newRaidStatusCommand(operations, metadataFetcher, tokenFile, requestTimeout, jsonOutput))
	command.AddCommand(newRaidReadyCommand(operations, tokenFile, requestTimeout, jsonOutput))
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

func newRaidGetCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	var raidID string
	command := &cobra.Command{Use: "get", Short: "Read one exact raid", Args: usageNoArgs}
	command.Flags().StringVar(&raidID, "raid-id", "", "Exact raid ID")
	command.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || raidID == "" {
			return newUsageError()
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		raid, err := operations.FindRaid(command.Context(), token, raidID, callOptions(requestTimeout))
		if err != nil {
			return sanitizedPublisherError(err)
		}
		if raid == nil {
			return output.NewError(output.CodeNotFound, "Raiden hittades inte.")
		}
		if *jsonOutput {
			view, warnings := raidDetailViewFor(raidView{Raid: *raid})
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("raid get", "found", view, warnings))
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "%s pokemon=%s players=%d\n", raid.ID, raid.PokemonID, raid.Count)
		return err
	}
	return command
}

func newRaidPlayersCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	var raidID string
	command := &cobra.Command{Use: "players", Short: "List one raid's players", Args: usageNoArgs}
	command.Flags().StringVar(&raidID, "raid-id", "", "Exact raid ID")
	command.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || raidID == "" {
			return newUsageError()
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		players, err := operations.ListRaidPlayers(command.Context(), token, raidID, callOptions(requestTimeout))
		if err != nil {
			return sanitizedPublisherError(err)
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("raid players", "listed", players, nil))
		}
		if _, err := fmt.Fprintf(command.OutOrStdout(), "players=%d ready=%d deleted=%t\n", players.Count, players.ReadyCount, players.RaidDeleted); err != nil {
			return err
		}
		for _, player := range players.Players {
			rating := "none"
			if player.Rating != nil {
				rating = fmt.Sprint(*player.Rating)
			}
			if _, err := fmt.Fprintf(command.OutOrStdout(), "%s level=%d country=%s rating=%s ready=%t me=%t\n", player.TrainerName, player.Level, player.Country, rating, player.Ready, player.IsMe); err != nil {
				return err
			}
		}
		return nil
	}
	return command
}

func newRaidStatusCommand(operations RaidOperations, metadataFetcher MetadataFetcher, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
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
		view, err := raidViewFor(command.Context(), metadataFetcher, *raid)
		if err != nil {
			return err
		}
		players, err := operations.ListRaidPlayers(command.Context(), token, raidID, options)
		if err != nil {
			return sanitizedPublisherError(err)
		}
		detail, warnings := raidDetailViewFor(view)
		data := struct {
			Raid    raidDetailView     `json:"raid"`
			Players domain.RaidPlayers `json:"players"`
		}{Raid: detail, Players: players}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("raid status", "found", data, warnings))
		}
		_, err = fmt.Fprintf(command.OutOrStdout(), "%s name=%s pokemon=%s players=%d ready=%d\n", raid.ID, view.PokemonName, raid.PokemonID, players.Count, players.ReadyCount)
		return err
	}
	return command
}

func newRaidReadyCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	var raidID string
	command := &cobra.Command{Use: "ready", Short: "Mark the anonymous trainer ready in one raid", Args: usageNoArgs}
	command.Flags().StringVar(&raidID, "raid-id", "", "Exact raid ID")
	command.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || raidID == "" {
			return newUsageError()
		}
		token, err := readCommandToken(command, *tokenFile)
		if err != nil {
			return err
		}
		if err := operations.ReadyRaid(command.Context(), token, raidID, callOptions(requestTimeout)); err != nil {
			return sanitizedPublisherError(err)
		}
		if *jsonOutput {
			return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("raid ready", "ready", struct {
				Ready  bool   `json:"ready"`
				RaidID string `json:"raidId"`
			}{Ready: true, RaidID: raidID}, nil))
		}
		_, err = fmt.Fprintln(command.OutOrStdout(), "Redo markerad.")
		return err
	}
	return command
}

func newRaidLeaveCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	var raidID string
	verify := true
	command := &cobra.Command{Use: "leave", Short: "Leave and verify one raid", Args: usageNoArgs}
	command.Flags().StringVar(&raidID, "raid-id", "", "Exact raid ID")
	command.Flags().BoolVar(&verify, "verify", true, "Verify that the raid is absent after leaving")
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
		if !verify {
			if *jsonOutput {
				return output.RenderJSON(command.OutOrStdout(), output.NewSuccess("raid leave", "requested", struct {
					Requested bool   `json:"requested"`
					Verified  bool   `json:"verified"`
					RaidID    string `json:"raidId"`
				}{Requested: true, Verified: false, RaidID: raidID}, nil))
			}
			_, err = fmt.Fprintln(command.OutOrStdout(), "Begäran att lämna raiden skickades.")
			return err
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

type matchingFilterFlags struct {
	minimumHostRating int
	hostCountry       int
	weatherBoost      int
	hostLevel         int
}

func bindMatchingFilterFlags(command *cobra.Command) *matchingFilterFlags {
	filters := &matchingFilterFlags{}
	command.Flags().IntVar(&filters.minimumHostRating, "min-host-rating", 3, "Minimum host rating from -1 through 5 (-1 disables the filter)")
	command.Flags().IntVar(&filters.hostCountry, "host-country", 0, "Host country filter (0 or 1)")
	command.Flags().IntVar(&filters.weatherBoost, "weather-boost", 0, "Weather boost filter (0, 1, or 2)")
	command.Flags().IntVar(&filters.hostLevel, "host-level", 0, "Host level filter (0, 40, 50, 60, or 70)")
	return filters
}

func (filters *matchingFilterFlags) apply(selection upstream.QueueSelection) (upstream.QueueSelection, error) {
	if filters == nil || filters.minimumHostRating < -1 || filters.minimumHostRating > 5 ||
		(filters.hostCountry != 0 && filters.hostCountry != 1) || filters.weatherBoost < 0 || filters.weatherBoost > 2 || !validHostLevel(filters.hostLevel) {
		return upstream.QueueSelection{}, newUsageError()
	}
	selection.MinimumHostRating = filters.minimumHostRating
	selection.HostCountry = filters.hostCountry
	selection.WeatherBoost = filters.weatherBoost
	selection.HostLevel = filters.hostLevel
	return selection, nil
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

func lobbyHostEvaluate(minimum int) []int {
	values := make([]int, 0, minimum+1)
	for rating := -1; rating < minimum; rating++ {
		values = append(values, rating)
	}
	return values
}

func helpRun(command *cobra.Command, _ []string) error { return command.Help() }

type raidView struct {
	domain.Raid
	PokemonName string `json:"pokemonName,omitempty"`
}

type raidDetailView struct {
	raidView
	HostTrainerCode string `json:"hostTrainerCode,omitempty"`
	ContainsSecret  bool   `json:"containsSecret"`
}

func raidDetailViewFor(view raidView) (raidDetailView, []string) {
	detail := raidDetailView{raidView: view, HostTrainerCode: view.HostTrainerCode, ContainsSecret: view.HostTrainerCode != ""}
	if !detail.ContainsSecret {
		return detail, nil
	}
	return detail, []string{"Värdens tränarkod är hemlig; hantera den säkert."}
}

func raidViews(ctx context.Context, fetcher MetadataFetcher, raids []domain.Raid) ([]raidView, error) {
	if len(raids) == 0 {
		return []raidView{}, nil
	}
	catalog, err := fetchCatalog(ctx, fetcher)
	if err != nil {
		return nil, err
	}
	views := make([]raidView, 0, len(raids))
	for _, raid := range raids {
		name, ok := catalogName(catalog, raid.PokemonID)
		if !ok {
			return nil, metadataUnavailableError()
		}
		views = append(views, raidView{Raid: raid, PokemonName: name})
	}
	return views, nil
}

func raidViewFor(ctx context.Context, fetcher MetadataFetcher, raid domain.Raid) (raidView, error) {
	views, err := raidViews(ctx, fetcher, []domain.Raid{raid})
	if err != nil {
		return raidView{}, err
	}
	return views[0], nil
}

func fetchCatalog(ctx context.Context, fetcher MetadataFetcher) (metadata.Catalog, error) {
	if fetcher == nil {
		return metadata.Catalog{}, metadataUnavailableError()
	}
	catalog, err := fetcher.Catalog(ctx)
	if err != nil {
		return metadata.Catalog{}, metadataUnavailableError()
	}
	return catalog, nil
}

func catalogName(catalog metadata.Catalog, id string) (string, bool) {
	entry, ok := catalog.Lookup(id)
	if !ok || strings.TrimSpace(entry.NameEN) == "" {
		return "", false
	}
	return entry.NameEN, true
}

func hostCurrentWeather(value string) (int, bool) {
	values := map[string]int{
		"unknown": 0, "sunny": 1, "clear-night": 2, "partly-cloudy": 3,
		"partly-cloudy-night": 4, "cloudy": 5, "rain": 6, "snow": 7, "windy": 8, "fog": 9,
	}
	result, ok := values[strings.ToLower(strings.TrimSpace(value))]
	return result, ok
}

func hostGymColor(value string) (int, bool) {
	values := map[string]int{"neutral": 0, "valor": 1, "mystic": 2, "instinct": 3}
	result, ok := values[strings.ToLower(strings.TrimSpace(value))]
	return result, ok
}

func hostPassType(value string) (int, bool) {
	values := map[string]int{"remote": 0, "local": 1}
	result, ok := values[strings.ToLower(strings.TrimSpace(value))]
	return result, ok
}

func hostGuestLevel(value int) (int, bool) {
	switch value {
	case 5:
		return 0, true
	case 40, 50, 60, 70:
		return value, true
	default:
		return 0, false
	}
}

func hostGuestRating(value string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "all":
		return 0, true
	case "0":
		return -1, true
	case "1":
		return 1, true
	case "2":
		return 2, true
	case "3":
		return 3, true
	case "4":
		return 4, true
	case "5":
		return 5, true
	default:
		return 0, false
	}
}

func metadataUnavailableError() error {
	return output.NewError(output.CodeMetadataUnavailable, "Pokémonmetadata är inte tillgänglig.")
}
