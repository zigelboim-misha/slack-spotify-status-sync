// Command auth performs the one-time Spotify OAuth flow and saves the refresh token.
//
// Usage:
//
//	source .env && go run ./cmd/auth
//
// Then open the printed URL in your browser and authorize the app.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"

	spotifyauth "github.com/zmb3/spotify/v2/auth"
)

func main() {
	redirectURI := os.Getenv("SPOTIFY_REDIRECT_URI")
	if redirectURI == "" {
		redirectURI = "http://127.0.0.1:8888/callback"
	}

	auth := spotifyauth.New(
		spotifyauth.WithRedirectURL(redirectURI),
		spotifyauth.WithScopes(spotifyauth.ScopeUserReadCurrentlyPlaying),
	)

	state := "spotify-slack-sync"
	ch := make(chan struct{})

	http.HandleFunc("/callback", func(w http.ResponseWriter, r *http.Request) {
		token, err := auth.Token(r.Context(), state, r)
		if err != nil {
			http.Error(w, "token exchange failed: "+err.Error(), http.StatusInternalServerError)
			log.Fatalf("token exchange: %v", err)
		}

		data, err := json.Marshal(token)
		if err != nil {
			log.Fatalf("marshal token: %v", err)
		}
		if err := os.WriteFile("spotify_token.json", data, 0600); err != nil {
			log.Fatalf("write token: %v", err)
		}

		fmt.Fprintln(w, "Authorization complete! You can close this tab.")
		log.Println("token saved to spotify_token.json")
		log.Println("")
		log.Println("For Render deployment, set this as SPOTIFY_TOKEN_JSON env var:")
		log.Printf("  %s", string(data))
		log.Println("")
		close(ch)
	})

	url := auth.AuthURL(state)
	fmt.Printf("Open this URL in your browser:\n\n  %s\n\n", url)

	server := &http.Server{Addr: ":8888"}
	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	<-ch
	_ = server.Shutdown(context.Background())
}
