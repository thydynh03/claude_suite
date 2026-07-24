package database

import (
	"fmt"
	"time"
)

func parseTimeValue(v interface{}) time.Time {
	if v == nil {
		return time.Time{}
	}
	switch val := v.(type) {
	case time.Time:
		return val
	case string:
		formats := []string{
			"2006-01-02 15:04:05",
			"2006-01-02T15:04:05Z",
			"2006-01-02T15:04:05.999999999Z07:00",
			"2006-01-02 15:04:05.999999999-07:00",
			time.RFC3339,
		}
		for _, f := range formats {
			if t, err := time.Parse(f, val); err == nil {
				return t
			}
		}
	}
	return time.Time{}
}

func parseString(v interface{}) string {
	if v == nil {
		return ""
	}
	switch val := v.(type) {
	case string:
		return val
	case []byte:
		return string(val)
	default:
		return fmt.Sprintf("%v", val)
	}
}
