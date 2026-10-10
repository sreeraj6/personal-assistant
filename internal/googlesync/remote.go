package googlesync

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"metracker/internal/storage"
)

var headers = []any{"Date", "Time", "Category", "Minutes", "Hours", "Activity", "Entry ID"}

type Remote struct {
	client   *http.Client
	config   Config
	baseURL  string
	sheetID  int
	rowCount int
}

func NewRemote(client *http.Client, c Config) *Remote {
	return &Remote{client: client, config: c, baseURL: "https://sheets.googleapis.com/v4/spreadsheets/"}
}

func QuoteTab(tab string) string { return "'" + strings.ReplaceAll(tab, "'", "''") + "'" }

func (r *Remote) request(ctx context.Context, method, suffix string, body any, result any) error {
	var data []byte
	var err error
	if body != nil {
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, r.baseURL+url.PathEscape(r.config.SpreadsheetID)+suffix, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	response, err := r.client.Do(req)
	if err != nil {
		return fmt.Errorf("Google Sheets request failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		// Do not echo Google's response body: it can contain private sheet contents.
		io.Copy(io.Discard, io.LimitReader(response.Body, 4096))
		return fmt.Errorf("Google Sheets returned HTTP %d; request remains queued", response.StatusCode)
	}
	if result == nil {
		_, err = io.Copy(io.Discard, response.Body)
		return err
	}
	return json.NewDecoder(response.Body).Decode(result)
}

func (r *Remote) Read(ctx context.Context) ([][]any, error) {
	var metadata struct {
		Sheets []struct {
			Properties struct {
				SheetID int    `json:"sheetId"`
				Title   string `json:"title"`
				Grid    struct {
					RowCount    int `json:"rowCount"`
					ColumnCount int `json:"columnCount"`
				} `json:"gridProperties"`
			} `json:"properties"`
		} `json:"sheets"`
	}
	if err := r.request(ctx, http.MethodGet, "?fields=sheets.properties", nil, &metadata); err != nil {
		return nil, err
	}
	found := false
	for _, sheet := range metadata.Sheets {
		if sheet.Properties.Title == r.config.Tab {
			found = true
			r.sheetID = sheet.Properties.SheetID
			r.rowCount = sheet.Properties.Grid.RowCount
			if sheet.Properties.Grid.ColumnCount < 7 {
				return nil, errors.New("sync tab needs at least seven columns")
			}
		}
	}
	if !found {
		body := map[string]any{"requests": []any{map[string]any{"addSheet": map[string]any{"properties": map[string]any{"title": r.config.Tab, "gridProperties": map[string]any{"rowCount": 1000, "columnCount": 7}}}}}}
		var result struct {
			Replies []struct {
				AddSheet struct {
					Properties struct {
						SheetID int `json:"sheetId"`
					} `json:"properties"`
				} `json:"addSheet"`
			} `json:"replies"`
		}
		if err := r.request(ctx, http.MethodPost, ":batchUpdate", body, &result); err != nil {
			return nil, err
		}
		if len(result.Replies) != 1 {
			return nil, errors.New("unexpected Google tab creation response")
		}
		r.sheetID = result.Replies[0].AddSheet.Properties.SheetID
		r.rowCount = 1000
	}
	var result struct {
		Values [][]any `json:"values"`
	}
	rangeName := QuoteTab(r.config.Tab) + "!A:G"
	if err := r.request(ctx, http.MethodGet, "/values/"+url.PathEscape(rangeName), nil, &result); err != nil {
		return nil, err
	}
	if len(result.Values) == 0 {
		if err := r.writeValues(ctx, 1, headers); err != nil {
			return nil, err
		}
		return [][]any{headers}, nil
	}
	if len(result.Values[0]) != len(headers) {
		return nil, errors.New("sync tab has an incompatible header; choose an empty dedicated tab")
	}
	for i, value := range headers {
		if result.Values[0][i] != value {
			return nil, errors.New("sync tab has an incompatible header; choose an empty dedicated tab")
		}
	}
	return result.Values, nil
}

func (r *Remote) Write(ctx context.Context, row int, e storage.Entry) error {
	if row > r.rowCount {
		body := map[string]any{"requests": []any{map[string]any{"appendDimension": map[string]any{"sheetId": r.sheetID, "dimension": "ROWS", "length": row - r.rowCount + 1000}}}}
		if err := r.request(ctx, http.MethodPost, ":batchUpdate", body, nil); err != nil {
			return err
		}
		r.rowCount = row + 1000
	}
	return r.writeValues(ctx, row, []any{e.Time.Format(storage.DateFormat), e.Time.Format(storage.TimeFormat), strings.TrimSpace(e.Category), e.Minutes, float64(e.Minutes) / 60, strings.TrimSpace(e.Activity), e.ID})
}

func (r *Remote) writeValues(ctx context.Context, row int, values []any) error {
	rangeName := fmt.Sprintf("%s!A%d:G%d", QuoteTab(r.config.Tab), row, row)
	body := map[string]any{"range": rangeName, "majorDimension": "ROWS", "values": [][]any{values}}
	// RAW keeps descriptions beginning with '=' as text rather than formulas.
	return r.request(ctx, http.MethodPut, "/values/"+url.PathEscape(rangeName)+"?valueInputOption=RAW", body, nil)
}
