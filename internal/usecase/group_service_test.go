package usecase

import (
	"regexp"
	"testing"
)

func TestLinkRegex(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{"HTTP URL", "Check out https://example.com", true},
		{"HTTPS URL", "Visit https://google.com/search", true},
		{"Telegram link", "Join t.me/mychannel", true},
		{"Telegram.me link", "Join telegram.me/mychannel", true},
		{"Bit.ly short URL", "See bit.ly/abc123", true},
		{"TinyURL", "Check tinyurl.com/test", true},
		{"www link", "Visit www.example.com", true},
		{"No link", "This is just plain text without any links", false},
		{"Partial match", "The website dot com is cool", false},
		{"Email not link", "Email me at user@example.com", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := linkRegex.MatchString(tt.text)
			if matched != tt.expected {
				t.Errorf("linkRegex.MatchString(%q) = %v, want %v", tt.text, matched, tt.expected)
			}
		})
	}
}

func BenchmarkLinkRegex(b *testing.B) {
	text := "Check out https://example.com and t.me/channel for more info! Visit www.google.com"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		linkRegex.MatchString(text)
	}
}

func TestLinkRegexCompile(t *testing.T) {
	// Ensure regex compiles without error
	_, err := regexp.Compile(`(?i)(https?://[^\s]+|t\.me/[^\s]+|telegram\.me/[^\s]+|bit\.ly/[^\s]+|tinyurl\.com/[^\s]+|www\.[^\s]+)`)
	if err != nil {
		t.Errorf("Link regex compilation failed: %v", err)
	}
}

func TestProfanityRegex(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected bool
	}{
		{"English profanity", "What the fuck is this", true},
		{"Persian profanity", "این کس شعر چیه", true},
		{"Clean message", "Hello how are you today", false},
		{"Partial word", "Assessment is important", false},
		{"Mixed case", "DAMN this is bad", true},
		{"Multiple violations", "You are a fucking bastard", true},
		{"No profanity", "This is a perfectly clean message", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			matched := profanityRegex.MatchString(tt.text)
			if matched != tt.expected {
				t.Errorf("profanityRegex.MatchString(%q) = %v, want %v", tt.text, matched, tt.expected)
			}
		})
	}
}

func BenchmarkProfanityRegex(b *testing.B) {
	text := "What the fuck is this damn shit"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		profanityRegex.MatchString(text)
	}
}
