package gui

import (
	"embed"
	"encoding/base64"
	"strings"

	"fyne.io/fyne/v2"
)

//go:embed assets/icon.b64 assets/presets/*.txt
var assets embed.FS

var Icon fyne.Resource
var Presets map[string]string

func init() {
	encoded, _ := assets.ReadFile("assets/icon.b64")
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		panic("invalid embedded application icon")
	}
	Icon = fyne.NewStaticResource("scaner-icon.png", b)
	Presets = map[string]string{}
	for name, file := range map[string]string{"Cloudflare": "cloudflare", "CF London": "cloudflare-london", "Fastly": "fastly"} {
		b, _ := assets.ReadFile("assets/presets/" + file + ".txt")
		Presets[name] = string(b)
	}
}
