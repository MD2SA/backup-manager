package discord

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/MD2SA/backup-manager/internal/notification"
)

type DiscordProvider struct {
	WebhookURL string
}

func (d *DiscordProvider) Name() string { return "discord" }

func (d *DiscordProvider) Send(ctx context.Context, event notification.Event, message string, metadata map[string]interface{}) error {
	payload := map[string]interface{}{
		"content": fmt.Sprintf("**Backup Event: %s**\n%s\n```json\n%v\n```", event, message, metadata),
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", d.WebhookURL, bytes.NewBuffer(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("discord API returned status %d", resp.StatusCode)
	}

	return nil
}
