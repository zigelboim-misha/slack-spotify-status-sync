package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/mishazigelboim/slack-spotify-status-sync/spotify"
	"github.com/mishazigelboim/slack-spotify-status-sync/status"
)

const (
	defaultPollInterval = 5 * time.Second
	defaultIdleInterval = 30 * time.Second
	minPollInterval     = 2 * time.Second
	defaultPort         = "8080"
	defaultStatusExpiry = 3 * time.Minute
)

func main() {
	slackToken := os.Getenv("SLACK_TOKEN")
	if slackToken == "" {
		log.Fatal("SLACK_TOKEN is required")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Spotify client (requires spotify_token.json from OAuth flow)
	sp, err := spotify.New(ctx)
	if err != nil {
		log.Fatalf("spotify: %v", err)
	}

	slack := status.NewSlack(slackToken)

	// Health server for hosting platforms (e.g. Render)
	go startHealthServer(port())

	// Graceful shutdown
	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sig
		log.Println("shutting down...")
		cancel()
	}()

	log.Println("started — polling Spotify for track changes")
	pollLoop(ctx, sp, slack)
}

func pollLoop(ctx context.Context, sp *spotify.Client, slack *status.SlackStatus) {
	var lastTrackID string
	tokenSaveTick := time.NewTicker(30 * time.Minute)
	defer tokenSaveTick.Stop()

	for {
		select {
		case <-ctx.Done():
			clearStatus(slack)
			return
		case <-tokenSaveTick.C:
			sp.SaveToken()
			continue
		default:
		}

		track, err := sp.NowPlaying(ctx)
		if err != nil {
			log.Printf("[spotify] error: %v", err)
			sleep(ctx, defaultIdleInterval)
			continue
		}

		// Nothing playing or paused
		if track == nil {
			if lastTrackID != "" {
				clearStatus(slack)
				lastTrackID = ""
			}
			sleep(ctx, defaultIdleInterval)
			continue
		}

		// Track changed
		if string(track.ID) != lastTrackID {
			lastTrackID = string(track.ID)
			text := track.StatusText()
			expiry := track.TimeUntilEnd() + defaultStatusExpiry
			if err := slack.Set(ctx, text, expiry); err != nil {
				log.Printf("[slack] error: %v", err)
			}
		}

		// Smart sleep: wait until track ends or poll interval, whichever is shorter
		wait := track.TimeUntilEnd() + 500*time.Millisecond
		if wait > defaultPollInterval {
			wait = defaultPollInterval
		}
		if wait < minPollInterval {
			wait = minPollInterval
		}
		sleep(ctx, wait)
	}
}

func clearStatus(slack *status.SlackStatus) {
	if err := slack.Clear(); err != nil {
		log.Printf("[slack] error clearing status: %v", err)
	}
}

func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func startHealthServer(port string) {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "ok")
	})
	log.Printf("[health] listening on :%s", port)
	if err := http.ListenAndServe(":"+port, mux); err != nil {
		log.Printf("[health] server error: %v", err)
	}
}

func port() string {
	if p := os.Getenv("PORT"); p != "" {
		if _, err := strconv.Atoi(p); err == nil {
			return p
		}
	}
	return defaultPort
}
