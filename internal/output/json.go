package output

import (
	"encoding/json"
	"errors"
	"io"
)

func RenderJSON(destination io.Writer, envelope Envelope) error {
	normalized, err := normalizeEnvelope(envelope)
	if err != nil {
		return err
	}
	return json.NewEncoder(destination).Encode(normalized)
}

func normalizeEnvelope(envelope Envelope) (Envelope, error) {
	switch value := envelope.(type) {
	case SuccessEnvelope:
		if value.Warnings == nil {
			value.Warnings = []string{}
		}
		return value, nil
	case *SuccessEnvelope:
		if value == nil {
			return nil, errors.New("output envelope must not be nil")
		}
		normalized := *value
		if normalized.Warnings == nil {
			normalized.Warnings = []string{}
		}
		return normalized, nil
	case FailureEnvelope:
		return value, nil
	case *FailureEnvelope:
		if value == nil {
			return nil, errors.New("output envelope must not be nil")
		}
		return value, nil
	default:
		return nil, errors.New("output envelope must not be nil")
	}
}
