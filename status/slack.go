package status

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"time"

	"github.com/slack-go/slack"
)

var musicEmojis = []string{
	":headphones:",
	":musical_note:",
	":notes:",
	":microphone:",
	":guitar:",
	":saxophone:",
	":trumpet:",
	":violin:",
	":drum_with_drumsticks:",
	":musical_keyboard:",
	":studio_microphone:",
	":level_slider:",
	":control_knobs:",
	":radio:",
	":musical_score:",
	":speaker:",
	":loud_sound:",
	":sound:",
	":cd:",
	":minidisc:",
	":accordion:",
	":banjo:",
	":long_drum:",
	":maracas:",
}

// SlackStatus manages a Slack user's custom status.
type SlackStatus struct {
	client *slack.Client
}

// NewSlack creates a new SlackStatus. The token must be a user token (xoxp-...)
// with the users.profile:write scope.
func NewSlack(token string) *SlackStatus {
	return &SlackStatus{client: slack.New(token)}
}

// Set updates the user's Slack status to the given text with a headphones emoji.
// The expiry parameter controls when Slack auto-clears the status (safety net).
func (s *SlackStatus) Set(ctx context.Context, text string, expiry time.Duration) error {
	exp := int64(0)
	if expiry > 0 {
		exp = time.Now().Add(expiry).Unix()
	}
	emoji := randomEmoji()
	err := s.client.SetUserCustomStatusContextWithUser(ctx, "", text, emoji, exp)
	if err != nil {
		return fmt.Errorf("set slack status: %w", err)
	}
	log.Printf("[slack] status → %s %s", emoji, text)
	return nil
}

func randomEmoji() string {
	return musicEmojis[rand.Intn(len(musicEmojis))]
}

// Clear removes the user's custom status.
func (s *SlackStatus) Clear() error {
	if err := s.client.UnsetUserCustomStatus(); err != nil {
		return fmt.Errorf("clear slack status: %w", err)
	}
	log.Printf("[slack] status cleared")
	return nil
}
