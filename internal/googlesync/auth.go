package googlesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/oauth2"
)

const sheetsScope = "https://www.googleapis.com/auth/spreadsheets"

type credentials struct {
	Installed struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
	} `json:"installed"`
}

func loadCredentials(path string) (credentials, *oauth2.Config, error) {
	var creds credentials
	data, err := os.ReadFile(path)
	if err != nil {
		return creds, nil, err
	}
	if err := json.Unmarshal(data, &creds); err != nil {
		return creds, nil, errors.New("invalid OAuth credentials JSON")
	}
	if creds.Installed.ClientID == "" {
		return creds, nil, errors.New("download an OAuth client of type Desktop app")
	}
	cfg := &oauth2.Config{ClientID: creds.Installed.ClientID, ClientSecret: creds.Installed.ClientSecret, Scopes: []string{sheetsScope}, Endpoint: oauth2.Endpoint{AuthURL: "https://accounts.google.com/o/oauth2/v2/auth", TokenURL: "https://oauth2.googleapis.com/token"}}
	return creds, cfg, nil
}

// Authorize uses the system browser and a loopback listener with state and PKCE.
// openBrowser must return promptly after initiating browser navigation.
func Authorize(ctx context.Context, cfg *oauth2.Config, openBrowser func(string) error) (*oauth2.Token, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	defer listener.Close()
	local := *cfg
	local.RedirectURL = "http://" + listener.Addr().String() + "/"
	state, verifier := oauth2.GenerateVerifier(), oauth2.GenerateVerifier()
	type result struct {
		code string
		err  error
	}
	completed := make(chan result, 1)
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" || r.Method != http.MethodGet {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("state") != state {
			http.Error(w, "Invalid authorization state", http.StatusBadRequest)
			return
		}
		outcome := result{code: r.URL.Query().Get("code")}
		if r.URL.Query().Get("error") != "" {
			outcome.err = errors.New("Google authorization was denied")
		} else if outcome.code == "" {
			http.Error(w, "Missing authorization code", http.StatusBadRequest)
			return
		}
		select {
		case completed <- outcome:
		default:
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		fmt.Fprintln(w, "Authorization received. Return to Me Tracker to finish connecting.")
	})
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	defer server.Close()
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case completed <- result{err: err}:
			default:
			}
		}
	}()
	authURL := local.AuthCodeURL(state, oauth2.AccessTypeOffline, oauth2.SetAuthURLParam("prompt", "consent"), oauth2.S256ChallengeOption(verifier))
	if err := openBrowser(authURL); err != nil {
		return nil, fmt.Errorf("open Google sign-in: %w", err)
	}
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case result := <-completed:
		if result.err != nil {
			return nil, result.err
		}
		token, err := local.Exchange(ctx, result.code, oauth2.VerifierOption(verifier))
		if err != nil {
			return nil, errors.New("Google token exchange failed; try authorizing again")
		}
		if token.RefreshToken == "" {
			return nil, errors.New("Google did not supply offline access; authorize again")
		}
		return token, nil
	}
}

type persistedSource struct {
	mu     sync.Mutex
	source oauth2.TokenSource
	path   string
	last   oauth2.Token
}

func (s *persistedSource) Token() (*oauth2.Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	token, err := s.source.Token()
	if err != nil {
		return nil, errors.New("Google token refresh failed; check your connection or run Google setup again")
	}
	if token.AccessToken != s.last.AccessToken || !token.Expiry.Equal(s.last.Expiry) || token.RefreshToken != s.last.RefreshToken {
		if err := writeJSON(s.path, token); err != nil {
			return nil, fmt.Errorf("persist Google token: %w", err)
		}
		s.last = *token
	}
	return token, nil
}

func HTTPClient(ctx context.Context, dir string) (*http.Client, error) {
	_, cfg, err := loadCredentials(filepath.Join(dir, "google-credentials.json"))
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "google-token.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("Google authorization missing; run --setup-google: %w", err)
	}
	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, errors.New("invalid Google authorization token; run setup again")
	}
	if token.RefreshToken == "" {
		return nil, errors.New("Google offline authorization is missing; run setup again")
	}
	ctx = context.WithValue(ctx, oauth2.HTTPClient, &http.Client{Timeout: 20 * time.Second})
	source := &persistedSource{source: cfg.TokenSource(ctx, &token), path: path, last: token}
	client := oauth2.NewClient(ctx, source)
	client.Timeout = 20 * time.Second
	return client, nil
}

// Setup writes settings last so a failed sign-in never enables partial sync.
func Setup(ctx context.Context, dir, sheet, tab, credentialsPath string, openBrowser func(string) error) error {
	id, err := ParseSheetID(sheet)
	if err != nil {
		return err
	}
	c := Config{SpreadsheetID: id, Tab: strings.TrimSpace(tab)}
	if err := c.Validate(); err != nil {
		return err
	}
	creds, cfg, err := loadCredentials(credentialsPath)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	token, err := Authorize(ctx, cfg, openBrowser)
	if err != nil {
		return err
	}
	client := cfg.Client(ctx, token)
	client.Timeout = 20 * time.Second
	remote := NewRemote(client, c)
	if _, err := remote.Read(ctx); err != nil {
		return fmt.Errorf("check destination sheet: %w", err)
	}
	if err := writeJSON(filepath.Join(dir, "google-credentials.json"), creds); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, "google-token.json"), token); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, "google-sync.json"), c)
}
