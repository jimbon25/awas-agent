package cron

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

func MatchCron(spec string, t time.Time) bool {
	fields := strings.Fields(spec)
	if len(fields) != 5 {
		return false
	}

	return matchField(fields[0], t.Minute(), 0, 59) &&
		matchField(fields[1], t.Hour(), 0, 23) &&
		matchField(fields[2], t.Day(), 1, 31) &&
		matchField(fields[3], int(t.Month()), 1, 12) &&
		matchField(fields[4], int(t.Weekday()), 0, 6)
}

func matchField(field string, val int, min, max int) bool {
	if field == "*" {
		return true
	}

	if strings.Contains(field, ",") {
		parts := strings.Split(field, ",")
		for _, part := range parts {
			if matchField(part, val, min, max) {
				return true
			}
		}
		return false
	}

	if strings.Contains(field, "/") {
		parts := strings.Split(field, "/")
		if len(parts) != 2 {
			return false
		}
		step, err := strconv.Atoi(parts[1])
		if err != nil || step <= 0 {
			return false
		}
		rangePart := parts[0]
		if rangePart == "*" {
			return (val-min)%step == 0
		}
		if strings.Contains(rangePart, "-") {
			rParts := strings.Split(rangePart, "-")
			if len(rParts) != 2 {
				return false
			}
			start, err1 := strconv.Atoi(rParts[0])
			end, err2 := strconv.Atoi(rParts[1])
			if err1 != nil || err2 != nil {
				return false
			}
			if val < start || val > end {
				return false
			}
			return (val-start)%step == 0
		}
		start, err := strconv.Atoi(rangePart)
		if err != nil {
			return false
		}
		if val < start {
			return false
		}
		return (val-start)%step == 0
	}

	if strings.Contains(field, "-") {
		parts := strings.Split(field, "-")
		if len(parts) != 2 {
			return false
		}
		start, err1 := strconv.Atoi(parts[0])
		end, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil {
			return false
		}
		return val >= start && val <= end
	}

	num, err := strconv.Atoi(field)
	if err != nil {
		return false
	}
	return val == num
}

func NormalizeSchedule(spec string) (string, error) {
	spec = strings.TrimSpace(strings.ToLower(spec))
	if spec == "" {
		return "", fmt.Errorf("empty schedule")
	}

	if len(strings.Fields(spec)) == 5 {
		fields := strings.Fields(spec)
		for i, f := range fields {
			var min, max int
			switch i {
			case 0:
				min, max = 0, 59
			case 1:
				min, max = 0, 23
			case 2:
				min, max = 1, 31
			case 3:
				min, max = 1, 12
			case 4:
				min, max = 0, 6
			}
			if !isValidFieldSpec(f, min, max) {
				return "", fmt.Errorf("invalid cron field at position %d: %q", i+1, f)
			}
		}
		return spec, nil
	}

	if strings.HasPrefix(spec, "every ") {
		val := strings.TrimPrefix(spec, "every ")
		val = strings.TrimSpace(val)

		if strings.HasSuffix(val, "m") || strings.HasSuffix(val, "minute") || strings.HasSuffix(val, "minutes") || strings.HasSuffix(val, "menit") {
			numStr := extractDigits(val)
			num, err := strconv.Atoi(numStr)
			if err != nil || num <= 0 || num > 59 {
				return "", fmt.Errorf("invalid minute interval: %q", val)
			}
			return fmt.Sprintf("*/%d * * * *", num), nil
		}

		if strings.HasSuffix(val, "h") || strings.HasSuffix(val, "hour") || strings.HasSuffix(val, "hours") || strings.HasSuffix(val, "jam") {
			numStr := extractDigits(val)
			num, err := strconv.Atoi(numStr)
			if err != nil || num <= 0 || num > 23 {
				return "", fmt.Errorf("invalid hour interval: %q", val)
			}
			return fmt.Sprintf("0 */%d * * *", num), nil
		}
	}

	if strings.HasPrefix(spec, "daily at ") {
		timeStr := strings.TrimPrefix(spec, "daily at ")
		timeStr = strings.TrimSpace(timeStr)
		return parseDailyTime(timeStr)
	}

	if strings.HasPrefix(spec, "today at ") {
		timeStr := strings.TrimPrefix(spec, "today at ")
		timeStr = strings.TrimSpace(timeStr)
		return parseOnceOffTime(timeStr, 0)
	}

	if strings.HasPrefix(spec, "tomorrow at ") {
		timeStr := strings.TrimPrefix(spec, "tomorrow at ")
		timeStr = strings.TrimSpace(timeStr)
		return parseOnceOffTime(timeStr, 1)
	}

	return "", fmt.Errorf("unsupported schedule format: %q", spec)
}

func isValidFieldSpec(f string, min, max int) bool {
	for _, r := range f {
		if !((r >= '0' && r <= '9') || r == '*' || r == '/' || r == '-' || r == ',') {
			return false
		}
	}
	return true
}

func extractDigits(s string) string {
	var res []rune
	for _, r := range s {
		if r >= '0' && r <= '9' {
			res = append(res, r)
		}
	}
	return string(res)
}

func parseDailyTime(timeStr string) (string, error) {
	timeStr = strings.ReplaceAll(timeStr, " ", "")

	isPM := false
	isAM := false
	if strings.HasSuffix(timeStr, "pm") {
		isPM = true
		timeStr = strings.TrimSuffix(timeStr, "pm")
	} else if strings.HasSuffix(timeStr, "am") {
		isAM = true
		timeStr = strings.TrimSuffix(timeStr, "am")
	}

	var hour, min int
	var err error
	if strings.Contains(timeStr, ":") {
		parts := strings.Split(timeStr, ":")
		if len(parts) != 2 {
			return "", fmt.Errorf("invalid time format: %q", timeStr)
		}
		hour, err = strconv.Atoi(parts[0])
		if err != nil {
			return "", err
		}
		min, err = strconv.Atoi(parts[1])
		if err != nil {
			return "", err
		}
	} else {
		hour, err = strconv.Atoi(timeStr)
		if err != nil {
			return "", err
		}
		min = 0
	}

	if isPM && hour < 12 {
		hour += 12
	} else if isAM && hour == 12 {
		hour = 0
	}

	if hour < 0 || hour > 23 || min < 0 || min > 59 {
		return "", fmt.Errorf("time out of range: %d:%d", hour, min)
	}

	return fmt.Sprintf("%d %d * * *", min, hour), nil
}

func parseOnceOffTime(timeStr string, relativeDays int) (string, error) {
	dailyCron, err := parseDailyTime(timeStr)
	if err != nil {
		return "", err
	}
	fields := strings.Fields(dailyCron)
	if len(fields) != 5 {
		return "", fmt.Errorf("unexpected daily cron format: %q", dailyCron)
	}
	min := fields[0]
	hour := fields[1]

	target := time.Now().AddDate(0, 0, relativeDays)
	day := target.Day()
	month := int(target.Month())

	return fmt.Sprintf("%s %s %d %d *", min, hour, day, month), nil
}
