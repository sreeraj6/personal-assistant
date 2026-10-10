package googlesync

import "testing"

func TestParseSheetID(t *testing.T) {
	for _, value := range []string{"sheet_ID-123", "https://docs.google.com/spreadsheets/d/sheet_ID-123/edit#gid=0"} {
		id, err := ParseSheetID(value)
		if err != nil || id != "sheet_ID-123" {
			t.Fatalf("%s => %s %v", value, id, err)
		}
	}
	for _, value := range []string{"", "https://example.com/spreadsheets/d/id/edit", "https://docs.google.com/document/d/id", "id?query"} {
		if _, err := ParseSheetID(value); err == nil {
			t.Fatalf("accepted %q", value)
		}
	}
}
