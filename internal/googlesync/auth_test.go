package googlesync

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"golang.org/x/oauth2"
)

func TestAuthorizeChecksStateAndPKCE(t *testing.T) {
	challenge := ""
	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Error(err)
		}
		verifier := r.Form.Get("code_verifier")
		hash := sha256.Sum256([]byte(verifier))
		if verifier == "" || base64.RawURLEncoding.EncodeToString(hash[:]) != challenge {
			t.Error("PKCE verifier does not match challenge")
		}
		if r.Form.Get("code") != "test-code" {
			t.Error("unexpected code")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"access_token":"access","refresh_token":"refresh","token_type":"Bearer","expires_in":3600}`))
	}))
	defer tokenServer.Close()
	cfg := &oauth2.Config{ClientID: "client", Endpoint: oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: tokenServer.URL}}
	open := func(authURL string) error {
		u, err := url.Parse(authURL)
		if err != nil {
			return err
		}
		challenge = u.Query().Get("code_challenge")
		if u.Query().Get("code_challenge_method") != "S256" {
			t.Error("missing PKCE S256")
		}
		if u.Query().Get("access_type") != "offline" {
			t.Error("missing offline access")
		}
		callback := u.Query().Get("redirect_uri")
		wrong, err := http.Get(callback + "?state=wrong&code=test-code")
		if err != nil {
			return err
		}
		wrong.Body.Close()
		if wrong.StatusCode != http.StatusBadRequest {
			t.Error("accepted wrong state")
		}
		valid, err := http.Get(callback + "?" + url.Values{"state": {u.Query().Get("state")}, "code": {"test-code"}}.Encode())
		if err != nil {
			return err
		}
		valid.Body.Close()
		return nil
	}
	token, err := Authorize(context.Background(), cfg, open)
	if err != nil {
		t.Fatal(err)
	}
	if token.RefreshToken != "refresh" {
		t.Fatal("offline token missing")
	}
}

func TestRefreshedTokenIsPersistedPrivately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "google-token.json")
	token := &oauth2.Token{AccessToken: "new-access", RefreshToken: "refresh", Expiry: time.Now().Add(time.Hour)}
	s := &persistedSource{source: oauth2.StaticTokenSource(token), path: path}
	if _, err := s.Token(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved oauth2.Token
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.AccessToken != token.AccessToken || saved.RefreshToken != token.RefreshToken {
		t.Fatal("token not persisted")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0600 {
		t.Fatalf("token permissions=%o", info.Mode().Perm())
	}
}
