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
	name    string
	code    string
	level   int
	country string
}

func NewTrainer(name, code string, level int, country string) (Trainer, error) {
	if hasUnsafeTrainerText(name) {
		return Trainer{}, ErrInvalidTrainer
	}
	name = strings.TrimSpace(name)
	if len(name) < 3 || !isASCIIAlphanumeric(name) {
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

	if level < 1 || level > 80 {
		return Trainer{}, ErrInvalidTrainer
	}

	if hasUnsafeTrainerText(country) {
		return Trainer{}, ErrInvalidTrainer
	}
	country = strings.ToLower(strings.TrimSpace(country))
	if country == "" || len(country) > MaxCountryBytes {
		return Trainer{}, ErrInvalidTrainer
	}

	return Trainer{name: name, code: code, level: level, country: country}, nil
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
	return trainer.name
}

func (trainer Trainer) CodeFormValue() string {
	return trainer.code
}

func (trainer Trainer) LevelFormValue() string {
	return strconv.Itoa(trainer.level)
}

func (trainer Trainer) CountryFormValue() string {
	return trainer.country
}

// Format prevents generic debug formatting from disclosing the trainer code.
func (trainer Trainer) Format(state fmt.State, _ rune) {
	_, _ = io.WriteString(state, "Trainer{name:")
	_, _ = io.WriteString(state, trainer.name)
	_, _ = io.WriteString(state, ", code:[redacted], level:")
	_, _ = io.WriteString(state, strconv.Itoa(trainer.level))
	_, _ = io.WriteString(state, ", country:")
	_, _ = io.WriteString(state, trainer.country)
	_, _ = io.WriteString(state, "}")
}
