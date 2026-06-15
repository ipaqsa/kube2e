package engine

import (
	"encoding/json"
	"fmt"
	"time"
)

// Duration is a time.Duration that serializes as a human-readable string
// (for example "1m30s") in report output instead of raw nanoseconds.
type Duration time.Duration

// String returns the human-readable form, for example "1.5s".
func (d Duration) String() string {
	return time.Duration(d).String()
}

// MarshalJSON encodes the duration as a quoted human-readable string.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// UnmarshalJSON decodes a human-readable duration string produced by MarshalJSON.
func (d *Duration) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("unmarshal duration: %w", err)
	}

	parsed, err := time.ParseDuration(s)
	if err != nil {
		return fmt.Errorf("parse duration %q: %w", s, err)
	}

	*d = Duration(parsed)

	return nil
}
