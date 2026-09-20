package model

type Preset struct {
	ID          string
	Name        string
	URL         string
	Description string
}

var Catalog = []Preset{
	{ID: "whatsapp", Name: "WhatsApp", URL: "https://web.whatsapp.com/", Description: "WhatsApp Web"},
	{ID: "teams", Name: "Microsoft Teams", URL: "https://teams.microsoft.com/", Description: "Microsoft Teams for the web"},
	{ID: "messenger", Name: "Messenger", URL: "https://www.messenger.com/", Description: "Facebook Messenger"},
	{ID: "instagram", Name: "Instagram", URL: "https://www.instagram.com/direct/inbox/", Description: "Instagram direct messages"},
	{ID: "telegram", Name: "Telegram", URL: "https://web.telegram.org/", Description: "Telegram Web"},
	{ID: "discord", Name: "Discord", URL: "https://discord.com/app", Description: "Discord"},
	{ID: "slack", Name: "Slack", URL: "https://app.slack.com/client", Description: "Slack web client"},
	{ID: "google-chat", Name: "Google Chat", URL: "https://chat.google.com/", Description: "Google Chat"},
	{ID: "element", Name: "Element", URL: "https://app.element.io/", Description: "Matrix messaging with Element"},
	{ID: "gmail", Name: "Gmail", URL: "https://mail.google.com/", Description: "Gmail"},
	{ID: "outlook", Name: "Outlook", URL: "https://outlook.office.com/mail/", Description: "Outlook mail"},
	{ID: "google-messages", Name: "Google Messages", URL: "https://messages.google.com/web/", Description: "Google Messages for web"},
	{ID: "proton-mail", Name: "Proton Mail", URL: "https://mail.proton.me/", Description: "Proton Mail"},
	{ID: "custom", Name: "Custom service", URL: "https://example.com/", Description: "Any web messaging service"},
}

func PresetByID(id string) (Preset, bool) {
	for _, preset := range Catalog {
		if preset.ID == id {
			return preset, true
		}
	}
	return Preset{}, false
}
