package nativeui

import (
	"metracker/internal/storage"
	"testing"
	"time"
)

func TestNativeEntryValidation(t *testing.T) {
	valid := storage.Entry{Time: time.Now(), Category: "Development", Minutes: 30, Activity: "Implemented the GTK form"}
	if err := validate(valid, []string{"Development"}); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*storage.Entry){func(e *storage.Entry) { e.Category = "Unknown" }, func(e *storage.Entry) { e.Minutes = 0 }, func(e *storage.Entry) { e.Activity = " " }} {
		entry := valid
		change(&entry)
		if err := validate(entry, []string{"Development"}); err == nil {
			t.Fatal("invalid native entry accepted")
		}
	}
}
