package status

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/slack-go/slack"
)

const statusEmoji = ":headphones:"

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
	err := s.client.SetUserCustomStatusContextWithUser(ctx, "", text, statusEmoji, exp)
	if err != nil {
		return fmt.Errorf("set slack status: %w", err)
	}
	log.Printf("[slack] status → %s %s", statusEmoji, text)
	return nil
}

// Clear removes the user's custom status.
func (s *SlackStatus) Clear() error {
	if err := s.client.UnsetUserCustomStatus(); err != nil {
		return fmt.Errorf("clear slack status: %w", err)
	}
	log.Printf("[slack] status cleared")
	return nil
}
