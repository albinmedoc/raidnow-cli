package domain

import (
	"encoding/json"
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

type ValidationField string

const (
	ValidationFieldName    ValidationField = "trainer name"
	ValidationFieldCode    ValidationField = "trainer code"
	ValidationFieldLevel   ValidationField = "trainer level"
	ValidationFieldCountry ValidationField = "trainer country"
)

type ValidationError struct {
	field ValidationField
}

func (err *ValidationError) Error() string {
	return "invalid " + string(err.field)
}

func (err *ValidationError) Field() ValidationField {
	return err.field
}

func invalid(field ValidationField) error {
	return &ValidationError{field: field}
}

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
		return Trainer{}, invalid(ValidationFieldName)
	}
	name = strings.TrimSpace(name)
	if len(name) < 3 || !isASCIIAlphanumeric(name) {
		return Trainer{}, invalid(ValidationFieldName)
	}

	if hasUnsafeTrainerText(code) {
		return Trainer{}, invalid(ValidationFieldCode)
	}
	code = strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, code)
	if len(code) != 12 || !isASCIIDigits(code) {
		return Trainer{}, invalid(ValidationFieldCode)
	}

	if level < 1 || level > 80 {
		return Trainer{}, invalid(ValidationFieldLevel)
	}

	if hasUnsafeTrainerText(country) {
		return Trainer{}, invalid(ValidationFieldCountry)
	}
	country = strings.ToLower(strings.TrimSpace(country))
	if country == "" || len(country) > MaxCountryBytes {
		return Trainer{}, invalid(ValidationFieldCountry)
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

// MarshalJSON deliberately omits the trainer code. Upstream form construction must
// use CodeFormValue explicitly.
func (trainer Trainer) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Name    string `json:"name"`
		Level   int    `json:"level"`
		Country string `json:"country"`
	}{Name: trainer.name, Level: trainer.level, Country: trainer.country})
}
