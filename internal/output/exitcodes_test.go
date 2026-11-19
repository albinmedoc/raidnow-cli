package output

import "testing"

func TestStableExitCodes(t *testing.T) {
	testCases := []struct {
		name string
		got  int
		want int
	}{
		{name: "success", got: ExitSuccess, want: 0},
		{name: "invalid arguments", got: ExitInvalidArguments, want: 2},
		{name: "invalid invocation input", got: ExitInvalidInput, want: 3},
		{name: "authentication unsupported", got: ExitAuthenticationUnsupported, want: 4},
		{name: "not found", got: ExitNotFound, want: 5},
		{name: "lifecycle conflict", got: ExitLifecycleConflict, want: 6},
		{name: "upstream rejection", got: ExitUpstreamRejection, want: 7},
		{name: "read network timeout", got: ExitReadNetwork, want: 8},
		{name: "write outcome unknown", got: ExitWriteOutcomeUnknown, want: 9},
		{name: "cache persistence error", got: ExitLocalPersistence, want: 10},
		{name: "metadata unavailable", got: ExitMetadataUnavailable, want: 11},
		{name: "partial success", got: ExitPartialSuccess, want: 12},
		{name: "local security failure", got: ExitLocalSecurity, want: 13},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if testCase.got != testCase.want {
				t.Fatalf("exit code = %d, want %d", testCase.got, testCase.want)
			}
		})
	}
}

func TestStableErrorCodes(t *testing.T) {
	testCases := []struct {
		got  ErrorCode
		want ErrorCode
	}{
		{got: CodeInternal, want: "INTERNAL_ERROR"},
		{got: CodeInvalidArguments, want: "INVALID_ARGUMENTS"},
		{got: CodeInvalidInput, want: "INVALID_INPUT"},
		{got: CodeAuthenticationUnsupported, want: "AUTHENTICATION_UNSUPPORTED"},
		{got: CodeNotFound, want: "NOT_FOUND"},
		{got: CodeLifecycleConflict, want: "LIFECYCLE_CONFLICT"},
		{got: CodeUpstreamRejection, want: "UPSTREAM_REJECTION"},
		{got: CodeReadNetwork, want: "READ_NETWORK_ERROR"},
		{got: CodeWriteOutcomeUnknown, want: "WRITE_OUTCOME_UNKNOWN"},
		{got: CodeLocalPersistence, want: "LOCAL_PERSISTENCE_ERROR"},
		{got: CodeMetadataUnavailable, want: "METADATA_UNAVAILABLE"},
		{got: CodePartialSuccess, want: "PARTIAL_SUCCESS"},
		{got: CodeLocalSecurity, want: "LOCAL_SECURITY_ERROR"},
	}

	for _, testCase := range testCases {
		if testCase.got != testCase.want {
			t.Errorf("error code = %q, want %q", testCase.got, testCase.want)
		}
	}
}
