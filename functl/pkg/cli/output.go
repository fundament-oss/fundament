package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"reflect"
	"text/tabwriter"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
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
	return writeJSON(os.Stdout, v)
}

// protoJSON encodes API messages with the field names of the proto files,
// the snake_case the rest of the JSON output uses, and with every field
// present so scripts can rely on the keys.
var protoJSON = protojson.MarshalOptions{UseProtoNames: true, EmitUnpopulated: true}

var protoMessageType = reflect.TypeFor[proto.Message]()

// writeJSON writes v as indented JSON. API responses are protobuf messages
// whose fields encoding/json cannot see (it would print {} for each), so a
// message, or a slice of them, goes through protojson instead.
func writeJSON(w io.Writer, v any) error {
	data, err := marshalJSON(v)
	if err != nil {
		return err
	}

	var buf bytes.Buffer
	if err := json.Indent(&buf, data, "", "  "); err != nil {
		return fmt.Errorf("formatting JSON: %w", err)
	}
	buf.WriteByte('\n')

	if _, err := w.Write(buf.Bytes()); err != nil {
		return fmt.Errorf("writing JSON: %w", err)
	}
	return nil
}

func marshalJSON(v any) ([]byte, error) {
	if msg, ok := v.(proto.Message); ok {
		data, err := protoJSON.Marshal(msg)
		if err != nil {
			return nil, fmt.Errorf("encoding JSON: %w", err)
		}
		return data, nil
	}

	rv := reflect.ValueOf(v)
	if rv.Kind() == reflect.Slice && rv.Type().Elem().Implements(protoMessageType) {
		items := make([]json.RawMessage, rv.Len())
		for i := range rv.Len() {
			item, err := protoJSON.Marshal(rv.Index(i).Interface().(proto.Message))
			if err != nil {
				return nil, fmt.Errorf("encoding JSON: %w", err)
			}
			items[i] = item
		}
		v = items
	}

	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("encoding JSON: %w", err)
	}
	return data, nil
}

// NewTableWriter creates a new tabwriter for formatted table output.
func NewTableWriter() *tabwriter.Writer {
	return tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
}

// PrintKeyValue prints a key-value pair to the given writer.
func PrintKeyValue(w io.Writer, key string, value any) {
	fmt.Fprintf(w, "%s:\t%v\n", key, value)
}
