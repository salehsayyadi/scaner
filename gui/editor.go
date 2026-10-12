package gui

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"piyazche/config"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

// The editor is generated from Config, not a hand-picked subset. Adding a
// configuration field later automatically makes it editable in both apps.
type Editor struct {
	cfg      *config.Config
	errors   map[string]error
	controls map[string]fyne.CanvasObject
	onChange func()
	Sections map[string]fyne.CanvasObject
}

func NewEditor(cfg *config.Config, onChange func()) *Editor {
	e := &Editor{cfg: cfg, errors: map[string]error{}, controls: map[string]fyne.CanvasObject{}, onChange: onChange, Sections: map[string]fyne.CanvasObject{}}
	v := reflect.ValueOf(cfg).Elem()
	for i := 0; i < v.NumField(); i++ {
		name := strings.Split(v.Type().Field(i).Tag.Get("json"), ",")[0]
		e.Sections[name] = e.build(v.Field(i), name)
	}
	return e
}

func (e *Editor) Snapshot() (*config.Config, error) {
	var keys []string
	for k := range e.errors {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 0 {
		return nil, fmt.Errorf("%s: %v", keys[0], e.errors[keys[0]])
	}
	return e.cfg.Clone(), nil
}

func (e *Editor) build(v reflect.Value, path string) fyne.CanvasObject {
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			v.Set(reflect.New(v.Type().Elem()))
		}
		v = v.Elem()
	}
	var objects []fyne.CanvasObject
	var groups []*widget.AccordionItem
	for i := 0; i < v.NumField(); i++ {
		field := v.Type().Field(i)
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		p := path + "." + name
		value := v.Field(i)
		if value.Kind() == reflect.Struct || value.Kind() == reflect.Pointer {
			groups = append(groups, widget.NewAccordionItem(T(friendly(name)), e.build(value, p)))
			continue
		}
		control := e.control(value, p)
		e.controls[p] = control
		objects = append(objects, labeled(friendly(name), control))
	}
	if len(groups) > 0 {
		a := widget.NewAccordion(groups...)
		a.MultiOpen = true
		objects = append(objects, a)
	}
	return container.NewVBox(objects...)
}

func (e *Editor) changed() {
	if e.onChange != nil {
		e.onChange()
	}
}

var choices = map[string][]string{
	"proxy.method":             {"tls", "reality"},
	"proxy.type":               {"ws", "xhttp", "grpc", "tcp", "httpupgrade"},
	"fragment.mode":            {"manual", "auto", "off"},
	"output.format":            {"csv", "json"},
	"xray.logLevel":            {"none", "error", "warning", "info", "debug"},
	"xray.mux.xudpProxyUDP443": {"reject", "allow", "skip"},
}

func (e *Editor) control(v reflect.Value, path string) fyne.CanvasObject {
	if v.Kind() == reflect.Bool {
		c := widget.NewCheck(T("Enabled"), nil)
		c.SetChecked(v.Bool())
		c.OnChanged = func(b bool) { v.SetBool(b); e.changed() }
		return c
	}
	if opts, ok := choices[path]; ok {
		// Do not drop an existing/custom value merely because it is not a preset.
		value := v.String()
		found := false
		for _, o := range opts {
			if o == value {
				found = true
			}
		}
		if !found && value != "" {
			opts = append(append([]string{}, opts...), value)
		}
		c := widget.NewSelect(opts, nil)
		c.SetSelected(value)
		c.OnChanged = func(s string) { v.SetString(s); e.changed() }
		return c
	}
	c := widget.NewEntry()
	c.Wrapping = fyne.TextWrapOff
	switch v.Kind() {
	case reflect.String:
		c.SetText(v.String())
	case reflect.Int:
		c.SetText(strconv.FormatInt(v.Int(), 10))
	case reflect.Float64:
		c.SetText(strconv.FormatFloat(v.Float(), 'f', -1, 64))
	case reflect.Slice:
		var parts []string
		for i := 0; i < v.Len(); i++ {
			parts = append(parts, v.Index(i).String())
		}
		c.SetText(strings.Join(parts, ", "))
		c.SetPlaceHolder(T("Comma-separated values"))
	}
	c.OnChanged = func(s string) {
		delete(e.errors, path)
		var err error
		switch v.Kind() {
		case reflect.String:
			v.SetString(s)
		case reflect.Int:
			var n int64
			n, err = strconv.ParseInt(strings.TrimSpace(s), 10, 64)
			if err == nil {
				v.SetInt(n)
			}
		case reflect.Float64:
			var n float64
			n, err = strconv.ParseFloat(strings.TrimSpace(s), 64)
			if err == nil {
				v.SetFloat(n)
			}
		case reflect.Slice:
			var values []string
			for _, p := range strings.Split(s, ",") {
				if p = strings.TrimSpace(p); p != "" {
					values = append(values, p)
				}
			}
			v.Set(reflect.ValueOf(values))
		}
		if err != nil {
			e.errors[path] = errors.New(T("enter a valid number"))
		}
		c.SetValidationError(e.errors[path])
		e.changed()
	}
	return c
}

func friendly(s string) string {
	known := map[string]string{
		"uuid": "VLESS UUID", "sni": "SNI", "alpn": "ALPN (comma-separated)", "ws": "WebSocket / HTTP upgrade", "grpc": "gRPC", "xhttp": "XHTTP", "tls": "TLS", "reality": "Reality",
		"timeout": "Timeout (seconds)", "maxLatency": "Maximum latency (ms)", "maxLoss": "Maximum loss (%) · 0 = no filter", "minUpload": "Minimum upload (Mbps)", "minDownload": "Minimum download (Mbps)",
		"uploadBytes": "Upload payload (bytes)", "downloadBytes": "Download payload (bytes)", "sampleSize": "IPs per subnet · 0 = all", "tcpPrecheck": "Fast TCP pre-check (skip dead IPs quickly)", "maxIPs": "Maximum IPs · 0 = all", "successThreshold": "Success threshold (0–1)", "directory": "Output directory",
		"length": "Length (bytes, e.g. 10-20)", "interval": "Interval (ms, e.g. 10-20)", "testIP": "Optimization test IP", "realisticPing": "Realistic warm ping", "handshakeTest": "TLS handshake test", "lossTest": "Packet-loss test",
	}
	if s, ok := known[s]; ok {
		return s
	}
	var b strings.Builder
	for i, r := range s {
		if i == 0 {
			b.WriteRune(unicode.ToUpper(r))
			continue
		}
		if unicode.IsUpper(r) {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
