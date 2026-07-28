package terminal

import (
	"os"
	"strconv"
	"strings"
)

const (
	MarkdownStyleDark  = "dark"
	MarkdownStyleLight = "light"
)

var (
	darkBackground = detectDarkBackgroundFromEnv()
	markdownStyle  = MarkdownStyleDark
)

func init() {
	if !darkBackground {
		markdownStyle = MarkdownStyleLight
	}
}

func MarkdownStyle() string {
	return markdownStyle
}

func HasDarkBackground() bool {
	return darkBackground
}

func detectDarkBackgroundFromEnv() bool {
	colorFGBG := os.Getenv("COLORFGBG")
	if colorFGBG == "" {
		return true
	}
	parts := strings.Split(colorFGBG, ";")
	bg, err := strconv.Atoi(parts[len(parts)-1])
	if err != nil {
		return true
	}
	return !isLightANSIBackground(bg)
}

func isLightANSIBackground(bg int) bool {
	switch bg {
	case 7, 15:
		return true
	default:
		return bg >= 230
	}
}
