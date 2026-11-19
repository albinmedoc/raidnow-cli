package output

import (
	"errors"
	"testing"
)

func TestExitCodeForDeterministicErrors(t *testing.T) {
	testCases := []struct {
		code ErrorCode
		want int
	}{
		{code: CodeInvalidArguments, want: ExitInvalidArguments},
		{code: CodeInvalidInput, want: ExitInvalidInput},
		{code: CodeAuthenticationUnsupported, want: ExitAuthenticationUnsupported},
		{code: CodeNotFound, want: ExitNotFound},
		{code: CodeLifecycleConflict, want: ExitLifecycleConflict},
		{code: CodeUpstreamRejection, want: ExitUpstreamRejection},
		{code: CodeReadNetwork, want: ExitReadNetwork},
		{code: CodeWriteOutcomeUnknown, want: ExitWriteOutcomeUnknown},
		{code: CodeLocalPersistence, want: ExitLocalPersistence},
		{code: CodeMetadataUnavailable, want: ExitMetadataUnavailable},
		{code: CodePartialSuccess, want: ExitPartialSuccess},
		{code: CodeLocalSecurity, want: ExitLocalSecurity},
	}

	for _, testCase := range testCases {
		t.Run(string(testCase.code), func(t *testing.T) {
			err := NewError(testCase.code, "säkert meddelande")
			if got := ExitCodeFor(err); got != testCase.want {
				t.Fatalf("exit code = %d, want %d", got, testCase.want)
			}
		})
	}
}

func TestUnknownErrorsFailClosedWithoutRawDetails(t *testing.T) {
	const raw = "token=super-secret upstream panic"
	err := errors.New(raw)

	if got := ExitCodeFor(err); got != 1 {
		t.Fatalf("unknown error exit code = %d, want 1", got)
	}

	public := PublicError(err)
	if public.Code != CodeInternal {
		t.Fatalf("public error code = %q, want %q", public.Code, CodeInternal)
	}
	if public.Message == raw {
		t.Fatalf("public error exposed raw details %q", raw)
	}
}

func TestNilErrorMapsToSuccess(t *testing.T) {
	if got := ExitCodeFor(nil); got != ExitSuccess {
		t.Fatalf("nil error exit code = %d, want %d", got, ExitSuccess)
	}
}
