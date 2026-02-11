package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
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
