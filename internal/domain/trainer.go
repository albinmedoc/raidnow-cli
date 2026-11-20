package domain

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxCountryBytes is a conservative bound for normalized upstream country values.
// It permits known values such as "se", "jp", and "world" without a fixed catalog.
const MaxCountryBytes = 16

var ErrInvalidTrainer = errors.New("invalid trainer")

// Trainer is the normalized, per-invocation trainer identity used in upstream forms.
// Its fields are intentionally private so callers must choose an explicit form accessor.
type Trainer struct {
	identity TrainerIdentity
	code     string
}

// TrainerIdentity is the validated trainer identity required to join a raid.
type TrainerIdentity struct {
	name    string
	level   int
	country string
}

func NewTrainerIdentity(name string, level int, country string) (TrainerIdentity, error) {
	if hasUnsafeTrainerText(name) {
		return TrainerIdentity{}, ErrInvalidTrainer
	}
	name = strings.TrimSpace(name)
	if len(name) < 3 || !isASCIIAlphanumeric(name) {
		return TrainerIdentity{}, ErrInvalidTrainer
	}
	if level < 1 || level > 80 {
		return TrainerIdentity{}, ErrInvalidTrainer
	}
	if hasUnsafeTrainerText(country) {
		return TrainerIdentity{}, ErrInvalidTrainer
	}
	country = strings.ToLower(strings.TrimSpace(country))
	if country == "" || len(country) > MaxCountryBytes {
		return TrainerIdentity{}, ErrInvalidTrainer
	}
	return TrainerIdentity{name: name, level: level, country: country}, nil
}

func NewTrainer(name, code string, level int, country string) (Trainer, error) {
	identity, err := NewTrainerIdentity(name, level, country)
	if err != nil {
		return Trainer{}, ErrInvalidTrainer
	}

	if hasUnsafeTrainerText(code) {
		return Trainer{}, ErrInvalidTrainer
	}
	code = strings.Map(func(r rune) rune {
		if r == ' ' || r == '\u3000' {
			return -1
		}
		return r
	}, code)
	if len(code) != 12 || !isASCIIDigits(code) {
		return Trainer{}, ErrInvalidTrainer
	}

	return Trainer{identity: identity, code: code}, nil
}

func hasUnsafeTrainerText(value string) bool {
	if !utf8.ValidString(value) {
		return true
	}
	for _, r := range value {
		if unicode.IsControl(r) || unicode.In(r, unicode.Cf) || r == '\u2028' || r == '\u2029' {
			return true
		}
	}
	return false
}

func isASCIIAlphanumeric(value string) bool {
	for i := 0; i < len(value); i++ {
		character := value[i]
		if (character < 'a' || character > 'z') && (character < 'A' || character > 'Z') && (character < '0' || character > '9') {
			return false
		}
	}
	return true
}

func isASCIIDigits(value string) bool {
	for i := 0; i < len(value); i++ {
		if value[i] < '0' || value[i] > '9' {
			return false
		}
	}
	return true
}

func (trainer Trainer) NameFormValue() string {
	return trainer.identity.NameFormValue()
}

func (trainer Trainer) CodeFormValue() string {
	return trainer.code
}

func (trainer Trainer) LevelFormValue() string {
	return trainer.identity.LevelFormValue()
}

func (trainer Trainer) CountryFormValue() string {
	return trainer.identity.CountryFormValue()
}

func (identity TrainerIdentity) NameFormValue() string { return identity.name }

func (identity TrainerIdentity) LevelFormValue() string { return strconv.Itoa(identity.level) }

func (identity TrainerIdentity) CountryFormValue() string { return identity.country }

// Format prevents generic debug formatting from disclosing the trainer code.
func (trainer Trainer) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "Trainer{name:")
	_, _ = io.WriteString(state, trainer.identity.name)
	_, _ = io.WriteString(state, ", code:[redacted], level:")
	_, _ = io.WriteString(state, strconv.Itoa(trainer.identity.level))
	_, _ = io.WriteString(state, ", country:")
	_, _ = io.WriteString(state, trainer.identity.country)
	_, _ = io.WriteString(state, "}")
}
