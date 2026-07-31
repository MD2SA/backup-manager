package service

import (
	"fmt"

	"github.com/MD2SA/backup-manager/internal/notification"
	"github.com/MD2SA/backup-manager/internal/notification/discord"
)

func createNotificationProvider(pType string, config map[string]interface{}) (notification.NotificationProvider, error) {
	switch pType {
	case "discord":
		webhookURL, ok := config["webhook_url"].(string)
		if !ok || webhookURL == "" {
			return nil, fmt.Errorf("discord provider requires 'webhook_url'")
		}
		return &discord.DiscordProvider{WebhookURL: webhookURL}, nil
	default:
		return nil, fmt.Errorf("unknown notification provider type: %s", pType)
	}
}
