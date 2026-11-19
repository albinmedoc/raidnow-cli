package output

import (
	"encoding/json"
	"io"
)

func RenderJSON(destination io.Writer, value any) error {
	return json.NewEncoder(destination).Encode(value)
}
