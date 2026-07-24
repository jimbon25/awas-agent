package cron

import (
	"fmt"
	"testing"
	"time"
)

func TestMatchCron(t *testing.T) {
	refTime := time.Date(2026, 7, 22, 9, 30, 0, 0, time.UTC)

	tests := []struct {
		spec  string
		want  bool
	}{
		{"* * * * *", true},
		{"30 * * * *", true},
		{"0 * * * *", false},
		{"30 9 * * *", true},
		{"30 10 * * *", false},
		{"* 9 * * *", true},
		{"*/15 * * * *", true},
		{"*/20 * * * *", false},
		{"20-40 * * * *", true},
		{"40-50 * * * *", false},
		{"15,30,45 * * * *", true},
		{"10,20,40 * * * *", false},
		{"30 9 * * 3", true}, 
		{"30 9 * * 4", false}, 
	}

	for _, tt := range tests {
		got := MatchCron(tt.spec, refTime)
		if got != tt.want {
			t.Errorf("MatchCron(%q) = %v; want %v", tt.spec, got, tt.want)
		}
	}
}

func TestNormalizeSchedule(t *testing.T) {
	tests := []struct {
		input   string
		want    string
		wantErr bool
	}{
		{"* * * * *", "* * * * *", false},
		{"0 9 * * *", "0 9 * * *", false},
		{"every 30m", "*/30 * * * *", false},
		{"every 15 minutes", "*/15 * * * *", false},
		{"every 2h", "0 */2 * * *", false},
		{"every 4 hours", "0 */4 * * *", false},
		{"daily at 9am", "0 9 * * *", false},
		{"daily at 2:30pm", "30 14 * * *", false},
		{"daily at 18:45", "45 18 * * *", false},
		{"invalid format", "", true},
	}

	for _, tt := range tests {
		got, err := NormalizeSchedule(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("NormalizeSchedule(%q) error = %v; wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("NormalizeSchedule(%q) = %q; want %q", tt.input, got, tt.want)
		}
	}
}

func TestRelativeSchedules(t *testing.T) {
	now := time.Now()
	today := now
	tomorrow := now.AddDate(0, 0, 1)

	wantToday := fmt.Sprintf("20 14 %d %d *", today.Day(), int(today.Month()))
	gotToday, err := NormalizeSchedule("today at 14:20")
	if err != nil {
		t.Fatalf("NormalizeSchedule(\"today at 14:20\") error = %v", err)
	}
	if gotToday != wantToday {
		t.Errorf("got %q, want %q", gotToday, wantToday)
	}

	wantTomorrow := fmt.Sprintf("0 9 %d %d *", tomorrow.Day(), int(tomorrow.Month()))
	gotTomorrow, err := NormalizeSchedule("tomorrow at 9am")
	if err != nil {
		t.Fatalf("NormalizeSchedule(\"tomorrow at 9am\") error = %v", err)
	}
	if gotTomorrow != wantTomorrow {
		t.Errorf("got %q, want %q", gotTomorrow, wantTomorrow)
	}
}
