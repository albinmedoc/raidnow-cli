package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNewTrainerNormalizesInvocationValues(t *testing.T) {
	trainer, err := NewTrainer("  Ash123  ", "1234\u20035678\u00a09012", 80, "  WORLD  ")
	if err != nil {
		t.Fatalf("NewTrainer: %v", err)
	}
	if got, want := trainer.NameFormValue(), "Ash123"; got != want {
		t.Fatalf("name = %q, want %q", got, want)
	}
	if got, want := trainer.CodeFormValue(), "123456789012"; got != want {
		t.Fatalf("code = %q, want %q", got, want)
	}
	if got, want := trainer.LevelFormValue(), "80"; got != want {
		t.Fatalf("level = %q, want %q", got, want)
	}
	if got, want := trainer.CountryFormValue(), "world"; got != want {
		t.Fatalf("country = %q, want %q", got, want)
	}
}

func TestNewTrainerValidatesEveryFieldWithTypedSanitizedErrors(t *testing.T) {
	invalidUTF8 := string([]byte{0xff})
	testCases := []struct {
		name        string
		trainerName string
		code        string
		level       int
		country     string
		field       ValidationField
	}{
		{name: "name_too_short", trainerName: "ab", code: "123456789012", level: 1, country: "se", field: ValidationFieldName},
		{name: "name_non_ascii", trainerName: "Ash-1", code: "123456789012", level: 1, country: "se", field: ValidationFieldName},
		{name: "name_invalid_utf8", trainerName: invalidUTF8, code: "123456789012", level: 1, country: "se", field: ValidationFieldName},
		{name: "code_too_short", trainerName: "Ash", code: "12345678901", level: 1, country: "se", field: ValidationFieldCode},
		{name: "code_non_digit", trainerName: "Ash", code: "12345678901x", level: 1, country: "se", field: ValidationFieldCode},
		{name: "code_invalid_utf8", trainerName: "Ash", code: invalidUTF8 + "123456789012", level: 1, country: "se", field: ValidationFieldCode},
		{name: "level_zero", trainerName: "Ash", code: "123456789012", level: 0, country: "se", field: ValidationFieldLevel},
		{name: "level_eighty_one", trainerName: "Ash", code: "123456789012", level: 81, country: "se", field: ValidationFieldLevel},
		{name: "country_empty", trainerName: "Ash", code: "123456789012", level: 1, country: "  ", field: ValidationFieldCountry},
		{name: "country_invalid_utf8", trainerName: "Ash", code: "123456789012", level: 1, country: invalidUTF8, field: ValidationFieldCountry},
		{name: "country_control", trainerName: "Ash", code: "123456789012", level: 1, country: "s\u0085e", field: ValidationFieldCountry},
		{name: "country_too_long", trainerName: "Ash", code: "123456789012", level: 1, country: strings.Repeat("x", MaxCountryBytes+1), field: ValidationFieldCountry},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := NewTrainer(testCase.trainerName, testCase.code, testCase.level, testCase.country)
			if err == nil {
				t.Fatal("NewTrainer: got nil error, want rejection")
			}
			var validationError *ValidationError
			if !errors.As(err, &validationError) {
				t.Fatalf("error type = %T, want *ValidationError", err)
			}
			if validationError.Field() != testCase.field {
				t.Fatalf("field = %q, want %q", validationError.Field(), testCase.field)
			}
			encodedError, marshalErr := json.Marshal(err)
			if marshalErr != nil {
				t.Fatalf("marshal error: %v", marshalErr)
			}
			for label, rendered := range map[string]string{
				"percent-v":       fmt.Sprintf("%v", err),
				"percent-plus-v":  fmt.Sprintf("%+v", err),
				"percent-sharp-v": fmt.Sprintf("%#v", err),
				"json":            string(encodedError),
			} {
				if strings.Contains(rendered, "123456789012") {
					t.Fatalf("%s error formatting leaked trainer code: %q", label, rendered)
				}
			}
		})
	}
}

func TestNewTrainerAcceptsLevelBoundariesAndOpenCountryValues(t *testing.T) {
	for _, testCase := range []struct {
		level   int
		country string
	}{
		{level: 1, country: "se"},
		{level: 80, country: "jp"},
		{level: 50, country: "world"},
	} {
		if _, err := NewTrainer("Ash123", "123456789012", testCase.level, testCase.country); err != nil {
			t.Fatalf("NewTrainer(level=%d, country=%q): %v", testCase.level, testCase.country, err)
		}
	}
}

func TestTrainerFormattingAndJSONNeverExposeCode(t *testing.T) {
	const code = "123456789012"
	trainer, err := NewTrainer("Ash123", code, 50, "se")
	if err != nil {
		t.Fatalf("NewTrainer: %v", err)
	}
	encoded, err := json.Marshal(trainer)
	if err != nil {
		t.Fatalf("marshal trainer: %v", err)
	}
	for label, rendered := range map[string]string{
		"percent-v":       fmt.Sprintf("%v", trainer),
		"percent-plus-v":  fmt.Sprintf("%+v", trainer),
		"percent-sharp-v": fmt.Sprintf("%#v", trainer),
		"json":            string(encoded),
	} {
		if strings.Contains(rendered, code) {
			t.Fatalf("%s exposed trainer code: %q", label, rendered)
		}
	}
}
