package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"

	"github.com/google/uuid"
)

// OutputFormat represents the output format type.
type OutputFormat string

const (
	OutputTable OutputFormat = "table"
	OutputJSON  OutputFormat = "json"
)

// TimeFormat is the standard time format for CLI output.
const TimeFormat = "2006-01-02T15:04:05Z07:00"

// PrintJSON outputs data as formatted JSON to stdout.
func PrintJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// NewTableWriter creates a new tabwriter for formatted table output.
func NewTableWriter() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
}

// PrintKeyValue prints a key-value pair to the given writer.
func PrintKeyValue(w io.Writer, key string, value any) {
	fmt.Fprintf(w, "%s:\t%v\n", key, value)
}

// createdOutput is the JSON output structure of every create command: the id
// of what was made, and nothing else.
type createdOutput struct {
	ID string `json:"id"`
}

// outputCreatedID prints the id of a created row: bare on a line for the
// table format, so it can be captured by a script, and as an object in JSON.
func outputCreatedID(format OutputFormat, id uuid.UUID) error {
	switch format {
	case OutputJSON:
		return PrintJSON(createdOutput{ID: id.String()})
	case OutputTable:
		fmt.Println(id.String())
		return nil
	default:
		panic(fmt.Sprintf("unknown output format: %s", format))
	}
}
