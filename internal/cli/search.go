package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/albinmedoc/raidnow-cli/internal/domain"
	"github.com/albinmedoc/raidnow-cli/internal/output"
	"github.com/albinmedoc/raidnow-cli/internal/upstream"
	"github.com/spf13/cobra"
)

const (
	defaultPollInterval = 3 * time.Second
	minimumPollInterval = 250 * time.Millisecond
	defaultMaxWait      = 20 * time.Minute
	defaultCleanupWait  = 10 * time.Second
)

type waitFlags struct {
	pollInterval time.Duration
	maxWait      time.Duration
}

type waitEnd uint8

const (
	waitMatched waitEnd = iota
	waitTimedOut
	waitCancelled
)

func newQueueWaitCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "wait", Short: "Wait for the Auto Join queue to match", Args: usageNoArgs}
	selection := bindQueueSelectionFlags(command)
	wait := bindWaitFlags(command)
	command.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || !wait.valid() {
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
		poll, end, err := waitForQueue(command.Context(), operations, token, queueSelection, callOptions(requestTimeout), wait)
		if err != nil {
			return err
		}
		switch end {
		case waitMatched:
			return renderMatched(command, jsonOutput, "queue wait", poll)
		case waitTimedOut:
			return queueWaitTimeoutError()
		default:
			return queueWaitCancelledError()
		}
	}
	return command
}

func newSearchCommand(operations RaidOperations, tokenFile *string, requestTimeout *time.Duration, jsonOutput *bool) *cobra.Command {
	command := &cobra.Command{Use: "search", Short: "Create an Auto Join queue and wait for a match", Args: usageNoArgs}
	trainerInput := bindTrainerInputFlags(command)
	selection := bindQueueSelectionFlags(command)
	filters := bindMatchingFilterFlags(command)
	wait := bindWaitFlags(command)
	command.RunE = func(command *cobra.Command, _ []string) error {
		if invalidPrimitiveSetup(operations, tokenFile, requestTimeout) || !wait.valid() || (*tokenFile == "-" && trainerInput.codeFile == "-") {
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
		options := callOptions(requestTimeout)
		if err := operations.CreateMatching(command.Context(), token, trainer, queueSelection, options); err != nil {
			return sanitizedPublisherError(err)
		}
		poll, end, err := waitForQueue(command.Context(), operations, token, queueSelection, options, wait)
		if err != nil {
			return err
		}
		if end == waitMatched {
			return renderMatched(command, jsonOutput, "search", poll)
		}
		return cleanupSearch(operations, token, queueSelection.PokemonIDs, options, end)
	}
	return command
}

func bindWaitFlags(command *cobra.Command) *waitFlags {
	wait := &waitFlags{}
	command.Flags().DurationVar(&wait.pollInterval, "poll-interval", defaultPollInterval, "Time between queue polls")
	command.Flags().DurationVar(&wait.maxWait, "max-wait", defaultMaxWait, "Maximum time to wait for a match")
	return wait
}

func (wait *waitFlags) valid() bool {
	return wait != nil && wait.pollInterval >= minimumPollInterval && wait.maxWait > 0
}

func waitForQueue(parent context.Context, operations RaidOperations, token string, selection upstream.QueueSelection, options upstream.CallOptions, wait *waitFlags) (domain.QueuePoll, waitEnd, error) {
	waitContext, cancel := context.WithTimeout(parent, wait.maxWait)
	defer cancel()

	poll := func() (domain.QueuePoll, error) {
		result, err := operations.PollMatching(waitContext, token, selection, options)
		if err != nil {
			if waitContext.Err() != nil {
				return domain.QueuePoll{}, nil
			}
			return domain.QueuePoll{}, sanitizedPublisherError(err)
		}
		return result, nil
	}

	if waitContext.Err() != nil {
		return domain.QueuePoll{}, waitReason(parent), nil
	}
	result, err := poll()
	if err != nil {
		return domain.QueuePoll{}, waitMatched, err
	}
	if result.State == domain.QueueMatched {
		return result, waitMatched, nil
	}
	if waitContext.Err() != nil {
		return domain.QueuePoll{}, waitReason(parent), nil
	}

	ticker := time.NewTicker(wait.pollInterval)
	defer ticker.Stop()
	for {
		select {
		case <-waitContext.Done():
			return domain.QueuePoll{}, waitReason(parent), nil
		case <-ticker.C:
			result, err = poll()
			if err != nil {
				return domain.QueuePoll{}, waitMatched, err
			}
			if result.State == domain.QueueMatched {
				return result, waitMatched, nil
			}
			if waitContext.Err() != nil {
				return domain.QueuePoll{}, waitReason(parent), nil
			}
		}
	}
}

func waitReason(parent context.Context) waitEnd {
	if parent.Err() != nil {
		return waitCancelled
	}
	return waitTimedOut
}

func cleanupSearch(operations RaidOperations, token string, pokemon []string, options upstream.CallOptions, end waitEnd) error {
	timeout := defaultCleanupWait
	if options.Timeout > 0 {
		timeout = options.Timeout
	}
	cleanupContext, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cancelErr := operations.CancelMatching(cleanupContext, token, options)
	check, checkErr := operations.CheckMatching(cleanupContext, token, pokemon, options)
	if output.ExitCodeFor(cancelErr) == output.ExitWriteOutcomeUnknown {
		return sanitizedPublisherError(cancelErr)
	}
	if checkErr != nil {
		return output.NewError(output.CodePartialSuccess, "Köns rensningsförsök kunde inte verifieras.")
	}
	if check.Present {
		return output.NewError(output.CodeLifecycleConflict, "Kön finns fortfarande kvar efter rensningsförsöket.")
	}
	if end == waitTimedOut {
		return queueWaitTimeoutError()
	}
	return queueWaitCancelledError()
}

func renderMatched(command *cobra.Command, jsonOutput *bool, commandName string, poll domain.QueuePoll) error {
	if *jsonOutput {
		return output.RenderJSON(command.OutOrStdout(), output.NewSuccess(commandName, "matched", poll, nil))
	}
	_, err := fmt.Fprintf(command.OutOrStdout(), "matched %s\n", poll.RaidID)
	return err
}

func queueWaitTimeoutError() error {
	return output.NewError(output.CodeNotFound, "Ingen matchning hittades innan väntetiden löpte ut.")
}

func queueWaitCancelledError() error {
	return output.NewError(output.CodeReadNetwork, "Väntan avbröts.")
}
