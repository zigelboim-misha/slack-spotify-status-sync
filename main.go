package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
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
	heartbeatInterval   = 5 * time.Minute
	maxRecentTracks     = 20
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

// recentTracks keeps a rolling log of recently played tracks for heartbeat logging.
type recentTracks struct {
	mu     sync.Mutex
	tracks []trackEntry
}

type trackEntry struct {
	text string
	at   time.Time
}

func (r *recentTracks) add(text string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tracks = append(r.tracks, trackEntry{text: text, at: time.Now()})
	if len(r.tracks) > maxRecentTracks {
		r.tracks = r.tracks[len(r.tracks)-maxRecentTracks:]
	}
}

func (r *recentTracks) since(d time.Duration) []trackEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := time.Now().Add(-d)
	var result []trackEntry
	for _, t := range r.tracks {
		if t.at.After(cutoff) {
			result = append(result, t)
		}
	}
	return result
}

func pollLoop(ctx context.Context, sp *spotify.Client, slack *status.SlackStatus) {
	var lastTrackID string
	recent := &recentTracks{}
	tokenSaveTick := time.NewTicker(30 * time.Minute)
	defer tokenSaveTick.Stop()
	heartbeat := time.NewTicker(heartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			clearStatus(slack)
			return
		case <-tokenSaveTick.C:
			sp.SaveToken()
			continue
		case <-heartbeat.C:
			logHeartbeat(recent)
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
			recent.add(text)
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

func logHeartbeat(recent *recentTracks) {
	tracks := recent.since(heartbeatInterval)
	if len(tracks) == 0 {
		log.Printf("[heartbeat] alive — no tracks in the last %v", heartbeatInterval)
		return
	}
	log.Printf("[heartbeat] alive — %d track(s) in the last %v:", len(tracks), heartbeatInterval)
	for i, t := range tracks {
		log.Printf("[heartbeat]   %d. %s", i+1, t.text)
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
