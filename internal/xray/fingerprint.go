package xray

func ValidFingerprint(value string) bool {
	switch value {
	case "", "chrome", "firefox", "safari", "ios", "android", "edge", "360", "qq", "random", "randomized":
		return true
	}
	return false
}

func ClientFingerprint(value string) string {
	if value == "" {
		return "chrome"
	}
	return value
}
