package logs

import (
	"encoding/json"
	"strconv"
	"strings"
)

// messageKeys and levelKeys are the structured-log field names we recognise when
// extracting a human message and severity from a JSON log line.
var (
	messageKeys = []string{"message", "msg"}
	levelKeys   = []string{"level", "severity", "lvl", "loglevel", "log_level"}
)

// parseLogLine extracts a display message, a raw level string, and structured
// fields from a single log line. If the line is a JSON object, recognised keys
// are promoted and the remainder is returned as fields. Otherwise the whole
// line is the message with no fields.
func parseLogLine(line string) (message, level string, fields map[string]string) {
	trimmed := strings.TrimSpace(line)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return line, "", nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return line, "", nil
	}

	fields = make(map[string]string, len(obj))
	for k, raw := range obj {
		fields[k] = rawToString(raw)
	}

	message = line
	for _, k := range messageKeys {
		if v, ok := fields[k]; ok {
			message = v
			break
		}
	}
	for _, k := range levelKeys {
		if v, ok := fields[k]; ok {
			level = v
			break
		}
	}
	if len(fields) == 0 {
		fields = nil
	}
	return message, level, fields
}

// rawToString renders a JSON value as a plain string: strings unquoted, other
// scalars/containers kept as their JSON text.
func rawToString(raw json.RawMessage) string {
	s := string(raw)
	if len(s) > 0 && s[0] == '"' {
		if unq, err := strconv.Unquote(s); err == nil {
			return unq
		}
	}
	return s
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
