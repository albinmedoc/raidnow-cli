package domain

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestNewTrainerNormalizesInvocationValues(t *testing.T) {
	trainer, err := NewTrainer("\u3000Ash123\u3000", "1234 5678\u30009012", 80, "\u3000WORLD\u3000")
	if err != nil {
		t.Fatalf("NewTrainer: %v", err)
	}
	if trainer.NameFormValue() != "Ash123" || trainer.CodeFormValue() != "123456789012" || trainer.LevelFormValue() != "80" || trainer.CountryFormValue() != "world" {
		t.Fatalf("unexpected normalized trainer: %+v", trainer)
	}
}

func TestNewTrainerAcceptsPermittedCodeSeparatorsAndBoundaries(t *testing.T) {
	for _, testCase := range []struct {
		name    string
		code    string
		level   int
		country string
	}{
		{name: "compact", code: "123456789012", level: 1, country: "se"},
		{name: "ascii space", code: "1234 5678 9012", level: 80, country: "jp"},
		{name: "ideographic space", code: "1234\u30005678\u30009012", level: 50, country: "world"},
		{name: "mixed", code: "1234 5678\u30009012", level: 1, country: "se"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			trainer, err := NewTrainer("Ash123", testCase.code, testCase.level, testCase.country)
			if err != nil {
				t.Fatalf("NewTrainer: %v", err)
			}
			if trainer.CodeFormValue() != "123456789012" {
				t.Fatalf("code = %q", trainer.CodeFormValue())
			}
		})
	}
}

func TestNewTrainerRejectsInvalidValuesWithOneSentinel(t *testing.T) {
	invalidUTF8 := string([]byte{0xff})
	testCases := []struct {
		name        string
		trainerName string
		code        string
		level       int
		country     string
	}{
		{name: "name too short", trainerName: "ab", code: "123456789012", level: 1, country: "se"},
		{name: "name non ascii", trainerName: "Ash-1", code: "123456789012", level: 1, country: "se"},
		{name: "name invalid utf8", trainerName: invalidUTF8, code: "123456789012", level: 1, country: "se"},
		{name: "code too short", trainerName: "Ash", code: "12345678901", level: 1, country: "se"},
		{name: "code non digit", trainerName: "Ash", code: "12345678901x", level: 1, country: "se"},
		{name: "code invalid utf8", trainerName: "Ash", code: invalidUTF8 + "123456789012", level: 1, country: "se"},
		{name: "code nonbreaking space", trainerName: "Ash", code: "1234\u00a056789012", level: 1, country: "se"},
		{name: "level zero", trainerName: "Ash", code: "123456789012", level: 0, country: "se"},
		{name: "level eighty one", trainerName: "Ash", code: "123456789012", level: 81, country: "se"},
		{name: "country empty", trainerName: "Ash", code: "123456789012", level: 1, country: "  "},
		{name: "country invalid utf8", trainerName: "Ash", code: "123456789012", level: 1, country: invalidUTF8},
		{name: "country control", trainerName: "Ash", code: "123456789012", level: 1, country: "s\u0085e"},
		{name: "country too long", trainerName: "Ash", code: "123456789012", level: 1, country: strings.Repeat("x", MaxCountryBytes+1)},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			_, err := NewTrainer(testCase.trainerName, testCase.code, testCase.level, testCase.country)
			if !errors.Is(err, ErrInvalidTrainer) {
				t.Fatalf("error = %v, want ErrInvalidTrainer", err)
			}
			if strings.Contains(fmt.Sprintf("%v %+v %#v", err, err, err), "123456789012") {
				t.Fatalf("error formatting leaked trainer code: %v", err)
			}
		})
	}
}

func TestNewTrainerRejectsUnsafeRunesBeforeNormalization(t *testing.T) {
	for _, unsafe := range []string{"\n", "\r", "\t", "\u0085", "\u2028", "\u2029", "\u202e"} {
		for _, build := range []func(string) (Trainer, error){
			func(value string) (Trainer, error) { return NewTrainer(value+"Ash123", "123456789012", 1, "se") },
			func(value string) (Trainer, error) { return NewTrainer("Ash123", "1234"+value+"56789012", 1, "se") },
			func(value string) (Trainer, error) { return NewTrainer("Ash123", "123456789012", 1, value+"se") },
		} {
			if _, err := build(unsafe); !errors.Is(err, ErrInvalidTrainer) {
				t.Fatalf("unsafe %q error = %v", unsafe, err)
			}
		}
	}
}

func TestTrainerFormattingNeverExposesCode(t *testing.T) {
	const code = "123456789012"
	trainer, err := NewTrainer("Ash123", code, 50, "se")
	if err != nil {
		t.Fatal(err)
	}
	for _, rendered := range []string{fmt.Sprintf("%v", trainer), fmt.Sprintf("%+v", trainer), fmt.Sprintf("%#v", trainer)} {
		if strings.Contains(rendered, code) {
			t.Fatalf("format exposed trainer code: %q", rendered)
		}
	}
}

func TestNewTrainerIdentityNormalizesValidatedValues(t *testing.T) {
	identity, err := NewTrainerIdentity(" Ash123 ", 80, " WORLD ")
	if err != nil {
		t.Fatalf("new trainer identity: %v", err)
	}
	if identity.NameFormValue() != "Ash123" || identity.LevelFormValue() != "80" || identity.CountryFormValue() != "world" {
		t.Fatalf("identity=%+v", identity)
	}
}

func TestNewTrainerIdentityRejectsInvalidValues(t *testing.T) {
	for _, values := range []struct {
		name    string
		level   int
		country string
	}{
		{name: "A!", level: 50, country: "se"},
		{name: "Ash123", level: 0, country: "se"},
		{name: "Ash123", level: 81, country: "se"},
		{name: "Ash123", level: 50, country: ""},
		{name: "Ash123", level: 50, country: "SE\n"},
	} {
		if _, err := NewTrainerIdentity(values.name, values.level, values.country); err == nil {
			t.Fatalf("NewTrainerIdentity(%q, %d, %q) succeeded", values.name, values.level, values.country)
		}
	}
}
