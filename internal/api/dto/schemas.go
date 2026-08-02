package dto

type LocalConfig struct {
	Path string `json:"path" validate:"required"`
}

type S3Config struct {
	Region    string `json:"region" validate:"required"`
	Bucket    string `json:"bucket" validate:"required"`
	AccessKey string `json:"access_key" validate:"required"`
	SecretKey string `json:"secret_key" validate:"required"`
	Endpoint  string `json:"endpoint,omitempty"`
}

type DiscordConfig struct {
	WebhookURL string `json:"webhook_url" validate:"required,url"`
}
