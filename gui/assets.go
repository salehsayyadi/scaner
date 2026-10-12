package gui

import (
	"embed"
	"encoding/base64"
	"strings"

	"fyne.io/fyne/v2"
)

//go:embed assets/icon.b64 assets/presets/*.txt assets/fonts/*.b64
var assets embed.FS

var Icon fyne.Resource
var Presets map[string]string

// Vazirmatn (SIL OFL 1.1, assets/fonts/OFL.txt) renders Persian text.
var fontFaRegular, fontFaBold fyne.Resource

func init() {
	encoded, _ := assets.ReadFile("assets/icon.b64")
	b, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(encoded)))
	if err != nil {
		panic("invalid embedded application icon")
	}
	Icon = fyne.NewStaticResource("scaner-icon.png", b)
	fontFaRegular = embeddedB64("assets/fonts/vazirmatn-regular.b64", "Vazirmatn-Regular.ttf")
	fontFaBold = embeddedB64("assets/fonts/vazirmatn-bold.b64", "Vazirmatn-Bold.ttf")
	Presets = map[string]string{}
	for name, file := range map[string]string{"Cloudflare": "cloudflare", "CF London": "cloudflare-london", "Fastly": "fastly"} {
		b, _ := assets.ReadFile("assets/presets/" + file + ".txt")
		Presets[name] = string(b)
	}
}

func embeddedB64(path, name string) fyne.Resource {
	encoded, _ := assets.ReadFile(path)
	b, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(string(encoded)), ""))
	if err != nil {
		panic("invalid embedded asset " + path)
	}
	return fyne.NewStaticResource(name, b)
}
