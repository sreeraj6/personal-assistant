package googlesync

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"metracker/internal/storage"
)

func TestRemoteUsesRawValuesAndQuotedTab(t *testing.T) {
	wrote := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && !strings.Contains(r.URL.Path, "/values/"):
			json.NewEncoder(w).Encode(map[string]any{"sheets": []any{map[string]any{"properties": map[string]any{"sheetId": 1, "title": "Tony's work", "gridProperties": map[string]any{"rowCount": 1000, "columnCount": 7}}}}})
		case r.Method == http.MethodGet:
			if !strings.Contains(r.URL.Path, "'Tony''s work'!A:G") {
				t.Errorf("range=%s", r.URL.Path)
			}
			json.NewEncoder(w).Encode(map[string]any{"values": [][]any{headers}})
		case r.Method == http.MethodPut:
			wrote = true
			if r.URL.Query().Get("valueInputOption") != "RAW" {
				t.Error("descriptions must be treated as plain text")
			}
			var body struct {
				Values [][]any `json:"values"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if body.Values[0][5] != "=SUM(1,2)" || body.Values[0][6] != "entry-id" {
				t.Errorf("body=%v", body)
			}
			w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.Error(w, "unexpected", 500)
		}
	}))
	defer server.Close()
	remote := NewRemote(server.Client(), Config{SpreadsheetID: "sheet-id", Tab: "Tony's work"})
	remote.baseURL = server.URL + "/"
	if _, err := remote.Read(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := remote.Write(context.Background(), 2, storage.Entry{ID: "entry-id", Activity: "=SUM(1,2)"}); err != nil {
		t.Fatal(err)
	}
	if !wrote {
		t.Fatal("entry was not uploaded")
	}
}

func TestIncompatibleSheetIsNotOverwritten(t *testing.T) {
	writes := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writes++
		}
		if strings.Contains(r.URL.Path, "/values/") {
			json.NewEncoder(w).Encode(map[string]any{"values": [][]any{{"Personal budget"}}})
			return
		}
		w.Write([]byte(`{"sheets":[{"properties":{"sheetId":1,"title":"Activities","gridProperties":{"rowCount":1000,"columnCount":7}}}]}`))
	}))
	defer server.Close()
	remote := NewRemote(server.Client(), Config{SpreadsheetID: "sheet-id", Tab: "Activities"})
	remote.baseURL = server.URL + "/"
	if _, err := remote.Read(context.Background()); err == nil {
		t.Fatal("expected incompatible header error")
	}
	if writes != 0 {
		t.Fatal("existing sheet was modified")
	}
}

func TestMissingTabCreatesHeadersAndExpandsRows(t *testing.T) {
	created, headerWritten, expanded, entryWritten := false, false, false, false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && !strings.Contains(r.URL.Path, "/values/"):
			w.Write([]byte(`{"sheets":[]}`))
		case r.Method == http.MethodPost:
			var body map[string][]map[string]json.RawMessage
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if _, ok := body["requests"][0]["addSheet"]; ok {
				created = true
				w.Write([]byte(`{"replies":[{"addSheet":{"properties":{"sheetId":7}}}]}`))
			} else if _, ok := body["requests"][0]["appendDimension"]; ok {
				expanded = true
				w.Write([]byte(`{}`))
			} else {
				t.Error("unexpected batch request")
				http.Error(w, "unexpected", 500)
			}
		case r.Method == http.MethodGet:
			w.Write([]byte(`{"values":[]}`))
		case r.Method == http.MethodPut:
			var body struct {
				Values [][]any `json:"values"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if strings.HasSuffix(r.URL.Path, "!A1:G1") {
				headerWritten = true
				if len(body.Values) != 1 || len(body.Values[0]) != 7 || body.Values[0][6] != "Entry ID" {
					t.Error("wrong headers")
				}
			} else {
				entryWritten = true
			}
			w.Write([]byte(`{}`))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
	}))
	defer server.Close()
	remote := NewRemote(server.Client(), Config{SpreadsheetID: "sheet-id", Tab: "Activities"})
	remote.baseURL = server.URL + "/"
	rows, err := remote.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !created || !headerWritten {
		t.Fatalf("rows=%v created=%v headers=%v", rows, created, headerWritten)
	}
	if err := remote.Write(context.Background(), 1001, storage.Entry{ID: "entry-id", Activity: "Activity"}); err != nil {
		t.Fatal(err)
	}
	if !expanded || !entryWritten {
		t.Fatal("row capacity was not expanded before entry upload")
	}
}
