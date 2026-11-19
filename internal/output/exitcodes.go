package output

import "errors"

const (
	ExitSuccess                   = 0
	ExitInvalidArguments          = 2
	ExitInvalidInput              = 3
	ExitAuthenticationUnsupported = 4
	ExitNotFound                  = 5
	ExitLifecycleConflict         = 6
	ExitUpstreamRejection         = 7
	ExitReadNetwork               = 8
	ExitWriteOutcomeUnknown       = 9
	ExitLocalPersistence          = 10
	ExitMetadataUnavailable       = 11
	ExitPartialSuccess            = 12
	ExitLocalSecurity             = 13
)

const (
	CodeInternal                  ErrorCode = "INTERNAL_ERROR"
	CodeInvalidArguments          ErrorCode = "INVALID_ARGUMENTS"
	CodeInvalidInput              ErrorCode = "INVALID_INPUT"
	CodeAuthenticationUnsupported ErrorCode = "AUTHENTICATION_UNSUPPORTED"
	CodeNotFound                  ErrorCode = "NOT_FOUND"
	CodeLifecycleConflict         ErrorCode = "LIFECYCLE_CONFLICT"
	CodeUpstreamRejection         ErrorCode = "UPSTREAM_REJECTION"
	CodeReadNetwork               ErrorCode = "READ_NETWORK_ERROR"
	CodeWriteOutcomeUnknown       ErrorCode = "WRITE_OUTCOME_UNKNOWN"
	CodeLocalPersistence          ErrorCode = "LOCAL_PERSISTENCE_ERROR"
	CodeMetadataUnavailable       ErrorCode = "METADATA_UNAVAILABLE"
	CodePartialSuccess            ErrorCode = "PARTIAL_SUCCESS"
	CodeLocalSecurity             ErrorCode = "LOCAL_SECURITY_ERROR"
)

var exitCodes = map[ErrorCode]int{
	CodeInvalidArguments:          ExitInvalidArguments,
	CodeInvalidInput:              ExitInvalidInput,
	CodeAuthenticationUnsupported: ExitAuthenticationUnsupported,
	CodeNotFound:                  ExitNotFound,
	CodeLifecycleConflict:         ExitLifecycleConflict,
	CodeUpstreamRejection:         ExitUpstreamRejection,
	CodeReadNetwork:               ExitReadNetwork,
	CodeWriteOutcomeUnknown:       ExitWriteOutcomeUnknown,
	CodeLocalPersistence:          ExitLocalPersistence,
	CodeMetadataUnavailable:       ExitMetadataUnavailable,
	CodePartialSuccess:            ExitPartialSuccess,
	CodeLocalSecurity:             ExitLocalSecurity,
}

func ExitCodeFor(err error) int {
	if err == nil {
		return ExitSuccess
	}
	var cliError *CLIError
	if errors.As(err, &cliError) && cliError != nil {
		if exitCode, ok := exitCodes[cliError.Code]; ok {
			return exitCode
		}
	}
	return 1
}

func PublicError(err error) *CLIError {
	var cliError *CLIError
	if errors.As(err, &cliError) && cliError != nil {
		if _, ok := exitCodes[cliError.Code]; ok {
			return cliError
		}
	}
	return NewError(CodeInternal, "Ett oväntat internt fel inträffade.")
}
