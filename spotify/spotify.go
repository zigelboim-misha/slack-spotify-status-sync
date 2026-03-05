package spotify

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	sp "github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
)

const tokenFile = "spotify_token.json"

// Track holds the essential fields of a currently-playing track.
type Track struct {
	ID       sp.ID
	Name     string
	Artists  string
	Duration int // milliseconds
	Progress int // milliseconds
}

// Client wraps the zmb3/spotify client with token persistence.
type Client struct {
	client *sp.Client
	auth   *spotifyauth.Authenticator
	token  *oauth2.Token
}

// New creates a Spotify client from a persisted refresh token.
// Returns an error if the token file does not exist — run the auth helper first.
func New(ctx context.Context) (*Client, error) {
	auth := spotifyauth.New(
		spotifyauth.WithRedirectURL(redirectURL()),
		spotifyauth.WithScopes(spotifyauth.ScopeUserReadCurrentlyPlaying),
	)

	token, err := loadToken()
	if err != nil {
		return nil, fmt.Errorf("load spotify token: %w (run 'go run ./cmd/auth' first)", err)
	}

	httpClient := auth.Client(ctx, token)
	client := sp.New(httpClient, sp.WithRetry(true))

	return &Client{client: client, auth: auth, token: token}, nil
}

// NowPlaying returns the currently playing track, or nil if nothing is playing.
func (c *Client) NowPlaying(ctx context.Context) (*Track, error) {
	playing, err := c.client.PlayerCurrentlyPlaying(ctx)
	if err != nil {
		return nil, err
	}

	if playing == nil || playing.Item == nil || !playing.Playing {
		return nil, nil
	}

	artists := make([]string, len(playing.Item.Artists))
	for i, a := range playing.Item.Artists {
		artists[i] = a.Name
	}

	return &Track{
		ID:       playing.Item.ID,
		Name:     playing.Item.Name,
		Artists:  strings.Join(artists, ", "),
		Duration: int(playing.Item.Duration),
		Progress: int(playing.Progress),
	}, nil
}

// SaveToken persists the current OAuth token to disk so restarts don't require re-auth.
func (c *Client) SaveToken() {
	ts := c.auth.Client(context.Background(), c.token).Transport.(*oauth2.Transport)
	tok, err := ts.Source.Token()
	if err != nil {
		log.Printf("[spotify] failed to get refreshed token: %v", err)
		return
	}
	if err := persistToken(tok); err != nil {
		log.Printf("[spotify] failed to save token: %v", err)
	}
}

// loadToken tries SPOTIFY_TOKEN_JSON env var first (for platforms with ephemeral
// filesystems like Render), then falls back to the token file on disk.
func loadToken() (*oauth2.Token, error) {
	var data []byte
	if envToken := os.Getenv("SPOTIFY_TOKEN_JSON"); envToken != "" {
		data = []byte(envToken)
	} else {
		var err error
		data, err = os.ReadFile(tokenFile)
		if err != nil {
			return nil, err
		}
	}
	var token oauth2.Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, err
	}
	return &token, nil
}

func persistToken(token *oauth2.Token) error {
	data, err := json.Marshal(token)
	if err != nil {
		return err
	}
	return os.WriteFile(tokenFile, data, 0600)
}

func redirectURL() string {
	if url := os.Getenv("SPOTIFY_REDIRECT_URI"); url != "" {
		return url
	}
	return "http://127.0.0.1:8888/callback"
}

// StatusText formats the track as "Song — Artist(s)", truncated to 100 chars for Slack.
func (t *Track) StatusText() string {
	status := fmt.Sprintf("%s — %s", t.Name, t.Artists)
	if len(status) > 100 {
		status = status[:97] + "..."
	}
	return status
}

// TimeUntilEnd returns the estimated time until the track finishes.
func (t *Track) TimeUntilEnd() time.Duration {
	remaining := t.Duration - t.Progress
	if remaining < 0 {
		remaining = 0
	}
	return time.Duration(remaining) * time.Millisecond
}
