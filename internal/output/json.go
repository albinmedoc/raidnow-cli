package output

import (
	"encoding/json"
	"io"
)

func RenderJSON(destination io.Writer, envelope Envelope) error {
	return json.NewEncoder(destination).Encode(envelope)
}
