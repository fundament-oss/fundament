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

// maxFieldsPerEntry bounds how many structured fields one log line contributes.
const maxFieldsPerEntry = 64

// klogSeverity maps the leading severity letter of a klog/glog header onto a
// level. This is not an exotic format to support: Gardener's system components
// log in klog, and per-shoot Vali holds exactly those streams — so without it
// every system log line was classified as the default level, the ERROR chip
// read zero, and a severity filter for ERROR matched nothing at all.
var klogSeverity = map[byte]string{
	'E': "ERROR",
	'F': "ERROR", // fatal
	'W': "WARN",
	'I': "INFO",
	'D': "DEBUG",
}

// klogLevel extracts the severity from a klog/glog header:
//
//	E0804 12:33:01.123456       1 reflector.go:1] failed to sync
//
// The shape is matched strictly — severity letter, four digits of MMDD, then a
// space — so ordinary prose beginning with a capital letter is not misread as a
// severity.
func klogLevel(line string) string {
	if len(line) < 6 {
		return ""
	}
	level, ok := klogSeverity[line[0]]
	if !ok {
		return ""
	}
	for i := 1; i <= 4; i++ {
		if line[i] < '0' || line[i] > '9' {
			return ""
		}
	}
	if line[5] != ' ' {
		return ""
	}
	return level
}

// parseLogLine extracts a display message, a raw level string, and structured
// fields from a single log line. If the line is a JSON object, recognised keys
// are promoted and the remainder is returned as fields. Otherwise the whole line
// is the message, with the severity read from a klog/glog header when present.
func parseLogLine(line string) (message, level string, fields map[string]string) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || trimmed[0] != '{' {
		return line, klogLevel(trimmed), nil
	}

	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return line, klogLevel(trimmed), nil
	}

	// Read the recognised keys straight from the object: the field map below is
	// capped, so looking them up there would lose the message on a line with
	// enough keys to hit the cap.
	message = line
	for _, k := range messageKeys {
		if raw, ok := obj[k]; ok {
			message = rawToString(raw)
			break
		}
	}
	for _, k := range levelKeys {
		if raw, ok := obj[k]; ok {
			level = rawToString(raw)
			break
		}
	}

	// Cap the promoted keys. The line is tenant-controlled and every field is
	// copied into the response proto, so an object with thousands of keys — one
	// per entry, across a whole page — is a response-size multiplier.
	fields = make(map[string]string, min(len(obj), maxFieldsPerEntry))
	for k, raw := range obj {
		if len(fields) >= maxFieldsPerEntry {
			break
		}
		fields[k] = rawToString(raw)
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
	if s != "" && s[0] == '"' {
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
