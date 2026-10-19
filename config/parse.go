package config

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// ParseConfigInput auto-detects the format of a pasted config blob and returns a
// populated *Config. It accepts:
//   - a vless:// share link (the kind v2rayN / nekoray export)
//   - a full xray-core client JSON config (with an "outbounds" array)
//   - this app's own config.json (with a "proxy" object)
//   - a base64-encoded single line wrapping any of the above (subscription style)
//
// This is what powers the TUI "paste config" tab and the CLI --link flag, so the
// user never has to hand-assemble a config file.
func ParseConfigInput(text string) (*Config, error) {
	s := strings.TrimSpace(text)
	if s == "" {
		return nil, fmt.Errorf("empty config input")
	}

	lower := strings.ToLower(s)
	switch {
	case strings.HasPrefix(lower, "vless://"):
		return ParseVlessLink(s)
	case strings.HasPrefix(lower, "vmess://"):
		return nil, fmt.Errorf("vmess links are not supported — paste a vless:// link or an xray JSON config")
	case strings.HasPrefix(s, "{"):
		return ParseJSONConfig([]byte(s))
	default:
		// Maybe it is a base64-wrapped link/config (subscription line).
		if decoded, ok := tryBase64(s); ok && decoded != s {
			return ParseConfigInput(decoded)
		}
		return nil, fmt.Errorf("unrecognized config format — expected a vless:// link or JSON")
	}
}

// tryBase64 attempts to base64-decode s and returns the decoded text if it looks
// like a config (a vless link or JSON).
func tryBase64(s string) (string, bool) {
	s = strings.TrimSpace(s)
	// Try standard and URL-safe, with and without padding.
	decoders := []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	}
	for _, dec := range decoders {
		if b, err := dec.DecodeString(s); err == nil {
			t := strings.TrimSpace(string(b))
			tl := strings.ToLower(t)
			if strings.HasPrefix(tl, "vless://") || strings.HasPrefix(t, "{") {
				return t, true
			}
		}
	}
	return "", false
}

// ParseVlessLink parses a vless://UUID@HOST:PORT?params#remark share link.
func ParseVlessLink(link string) (*Config, error) {
	link = strings.TrimSpace(link)
	// Drop the #remark fragment — it is unused and frequently contains raw
	// spaces / emoji (e.g. "#🇳🇱 Cloudflare") that break url.Parse.
	if i := strings.IndexByte(link, '#'); i >= 0 {
		link = link[:i]
	}
	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("invalid vless link: %w", err)
	}
	if u.User == nil || u.User.Username() == "" {
		return nil, fmt.Errorf("vless link is missing the UUID")
	}

	cfg := DefaultConfig()
	cfg.Proxy.UUID = u.User.Username()
	cfg.Proxy.Address = u.Hostname()
	if p := u.Port(); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			cfg.Proxy.Port = n
		}
	}

	// Normalize query params to lowercase keys for robust lookup.
	q := map[string]string{}
	for k, v := range u.Query() {
		if len(v) > 0 {
			q[strings.ToLower(k)] = v[0]
		}
	}
	get := func(keys ...string) string {
		for _, k := range keys {
			if val, ok := q[strings.ToLower(k)]; ok && val != "" {
				return val
			}
		}
		return ""
	}

	cfg.Proxy.Flow = get("flow")
	if encryption := get("encryption"); encryption != "" && encryption != "none" {
		return nil, fmt.Errorf("unsupported VLESS encryption: %s", encryption)
	}

	if header := get("headerType"); header != "" && header != "none" {
		return nil, fmt.Errorf("unsupported transport headerType: %s", header)
	}

	// Transport type / network.
	network := strings.ToLower(get("type", "network"))
	cfg.Proxy.Type = normalizeNetwork(network)

	// Security method.
	security := strings.ToLower(get("security"))
	switch security {
	case "reality":
		cfg.Proxy.Method = "reality"
	case "tls":
		cfg.Proxy.Method = "tls"
	case "none", "":
		cfg.Proxy.Method = "none"
	default:
		return nil, fmt.Errorf("unsupported security: %s", security)
	}

	sni := get("sni", "peer")
	fp := get("fp", "fingerprint")
	alpn := splitCSV(get("alpn"))

	if cfg.Proxy.Method == "reality" {
		cfg.Proxy.TLS = nil
		cfg.Proxy.Reality = &RealityConfig{
			PublicKey:   get("pbk", "publickey"),
			ShortId:     get("sid", "shortid"),
			SpiderX:     get("spx", "spiderx"),
			Fingerprint: fp,
			ServerName:  sni,
		}
		if cfg.Proxy.Reality.SpiderX == "" {
			cfg.Proxy.Reality.SpiderX = "/"
		}
		if cfg.Proxy.Reality.Fingerprint == "" {
			cfg.Proxy.Reality.Fingerprint = "chrome"
		}
	} else if cfg.Proxy.Method == "tls" {
		cfg.Proxy.Reality = nil
		cfg.Proxy.TLS = &TlsConfig{
			SNI:           sni,
			ALPN:          alpn,
			Fingerprint:   fp,
			AllowInsecure: isTruthy(get("allowinsecure", "insecure")),
		}
		if cfg.Proxy.TLS.Fingerprint == "" {
			cfg.Proxy.TLS.Fingerprint = "chrome"
		}
		if cfg.Proxy.TLS.SNI == "" {
			cfg.Proxy.TLS.SNI = cfg.Proxy.Address
		}
	} else {
		cfg.Proxy.TLS, cfg.Proxy.Reality = nil, nil
	}

	host := get("host")
	path := get("path")
	if path == "" {
		path = "/"
	}

	switch cfg.Proxy.Type {
	case "ws", "httpupgrade":
		cfg.Proxy.WS = &WsConfig{Host: host, Path: path}
		cfg.Proxy.Xhttp = nil
		cfg.Proxy.Grpc = nil
	case "xhttp":
		cfg.Proxy.Xhttp = &XhttpConfig{Host: host, Path: path, Mode: get("mode")}
		if cfg.Proxy.Xhttp.Mode == "" {
			cfg.Proxy.Xhttp.Mode = "auto"
		}
		cfg.Proxy.WS = nil
		cfg.Proxy.Grpc = nil
	case "grpc":
		cfg.Proxy.Grpc = &GrpcConfig{
			ServiceName: get("servicename", "serviceName"),
			Authority:   get("authority"),
			MultiMode:   strings.EqualFold(get("mode"), "multi"),
		}
		cfg.Proxy.WS = nil
		cfg.Proxy.Xhttp = nil
	case "tcp":
		cfg.Proxy.WS = nil
		cfg.Proxy.Xhttp = nil
		cfg.Proxy.Grpc = nil
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("parsed link is incomplete: %w", err)
	}
	return cfg, nil
}

// ParseJSONConfig decides whether the JSON is an xray config or this app's config.
func ParseJSONConfig(data []byte) (*Config, error) {
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(data, &probe); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	if _, ok := probe["outbounds"]; ok {
		return ParseXrayJSON(data)
	}
	if _, ok := probe["proxy"]; ok {
		return ParseAppConfigJSON(data)
	}
	if c, err := ParseXrayJSON(data); err == nil {
		return c, nil
	}
	return ParseAppConfigJSON(data)
}

// ParseAppConfigJSON parses this app's own config.json layout from raw bytes.
func ParseAppConfigJSON(data []byte) (*Config, error) {
	cfg := DefaultConfig()
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config JSON: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

// ParseXrayJSON extracts a *Config from a full xray-core client JSON config by
// reading its vless outbound and (optionally) a fragment outbound.
func ParseXrayJSON(data []byte) (*Config, error) {
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid xray JSON: %w", err)
	}

	outbounds := asSlice(raw["outbounds"])
	if len(outbounds) == 0 {
		return nil, fmt.Errorf("no outbounds found in xray config")
	}

	cfg := DefaultConfig()
	var proxyOut map[string]interface{}
	for _, o := range outbounds {
		ob := asMap(o)
		if strings.EqualFold(asString(ob["protocol"]), "vless") {
			proxyOut = ob
			break
		}
	}
	if proxyOut == nil {
		return nil, fmt.Errorf("no vless outbound found (only vless is supported)")
	}

	settings := asMap(proxyOut["settings"])
	vnextList := asSlice(settings["vnext"])
	if len(vnextList) == 0 {
		return nil, fmt.Errorf("vless outbound has no vnext")
	}
	vnext := asMap(vnextList[0])
	cfg.Proxy.Address = asString(vnext["address"])
	if port := asInt(vnext["port"]); port > 0 {
		cfg.Proxy.Port = port
	}
	users := asSlice(vnext["users"])
	if len(users) > 0 {
		user := asMap(users[0])
		cfg.Proxy.UUID = asString(user["id"])
		cfg.Proxy.Flow = asString(user["flow"])
		if enc := asString(user["encryption"]); enc != "" && enc != "none" {
			return nil, fmt.Errorf("unsupported VLESS encryption: %s", enc)
		}
	}

	ss := asMap(proxyOut["streamSettings"])
	cfg.Proxy.Type = normalizeNetwork(strings.ToLower(asString(ss["network"])))
	security := strings.ToLower(asString(ss["security"]))

	switch security {
	case "reality":
		cfg.Proxy.Method = "reality"
		rs := asMap(ss["realitySettings"])
		cfg.Proxy.TLS = nil
		cfg.Proxy.Reality = &RealityConfig{
			PublicKey:   asString(rs["publicKey"]),
			ShortId:     asString(rs["shortId"]),
			SpiderX:     firstNonEmpty(asString(rs["spiderX"]), "/"),
			Fingerprint: firstNonEmpty(asString(rs["fingerprint"]), "chrome"),
			ServerName:  asString(rs["serverName"]),
		}
	case "", "none":
		cfg.Proxy.Method = "none"
		cfg.Proxy.TLS, cfg.Proxy.Reality = nil, nil
	case "tls":
		cfg.Proxy.Method = "tls"
		ts := asMap(ss["tlsSettings"])
		alpn := []string{}
		for _, a := range asSlice(ts["alpn"]) {
			alpn = append(alpn, asString(a))
		}
		cfg.Proxy.Reality = nil
		cfg.Proxy.TLS = &TlsConfig{
			SNI:           firstNonEmpty(asString(ts["serverName"]), cfg.Proxy.Address),
			ALPN:          alpn,
			Fingerprint:   firstNonEmpty(asString(ts["fingerprint"]), "chrome"),
			AllowInsecure: asBool(ts["allowInsecure"]),
		}
	default:
		return nil, fmt.Errorf("unsupported security: %s", security)
	}
	if mux := asMap(proxyOut["mux"]); len(mux) > 0 {
		if b, err := json.Marshal(mux); err == nil {
			if err := json.Unmarshal(b, &cfg.Xray.Mux); err != nil {
				return nil, err
			}
		}
	}

	switch cfg.Proxy.Type {
	case "ws":
		ws := asMap(ss["wsSettings"])
		host := asString(ws["host"])
		if host == "" {
			host = asString(asMap(ws["headers"])["Host"])
		}
		cfg.Proxy.WS = &WsConfig{Host: host, Path: firstNonEmpty(asString(ws["path"]), "/")}
	case "httpupgrade":
		hu := asMap(ss["httpupgradeSettings"])
		host := asString(hu["host"])
		if host == "" {
			host = asString(asMap(hu["headers"])["Host"])
		}
		cfg.Proxy.WS = &WsConfig{Host: host, Path: firstNonEmpty(asString(hu["path"]), "/")}
	case "xhttp":
		xs := asMap(ss["xhttpSettings"])
		if len(xs) == 0 {
			xs = asMap(ss["splithttpSettings"])
		}
		cfg.Proxy.Xhttp = &XhttpConfig{
			Host: asString(xs["host"]),
			Path: firstNonEmpty(asString(xs["path"]), "/"),
			Mode: firstNonEmpty(asString(xs["mode"]), "auto"),
		}
	case "grpc":
		gs := asMap(ss["grpcSettings"])
		cfg.Proxy.Grpc = &GrpcConfig{
			ServiceName: asString(gs["serviceName"]),
			Authority:   asString(gs["authority"]),
			MultiMode:   asBool(gs["multiMode"]),
		}
	}

	// Pick up a fragment outbound if present.
	for _, o := range outbounds {
		ob := asMap(o)
		if !strings.EqualFold(asString(ob["protocol"]), "freedom") {
			continue
		}
		fset := asMap(asMap(ob["settings"])["fragment"])
		if len(fset) == 0 {
			continue
		}
		cfg.Fragment.Enabled = true
		cfg.Fragment.Mode = "manual"
		if p := asString(fset["packets"]); p != "" {
			cfg.Fragment.Packets = p
		}
		if l := asString(fset["length"]); l != "" {
			cfg.Fragment.Manual.Length = l
		}
		if iv := asString(fset["interval"]); iv != "" {
			cfg.Fragment.Manual.Interval = iv
		}
		break
	}

	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("parsed xray config is incomplete: %w", err)
	}
	return cfg, nil
}

// --- small helpers -------------------------------------------------------

func normalizeNetwork(n string) string {
	switch n {
	case "splithttp", "xhttp":
		return "xhttp"
	case "h2", "http":
		return n // rejected explicitly rather than silently rewritten to TCP
	case "":
		return "tcp"
	default:
		return n
	}
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var out []string
	for _, p := range strings.Split(s, ",") {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

func isTruthy(s string) bool {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func asMap(v interface{}) map[string]interface{} {
	if m, ok := v.(map[string]interface{}); ok {
		return m
	}
	return map[string]interface{}{}
}

func asSlice(v interface{}) []interface{} {
	if s, ok := v.([]interface{}); ok {
		return s
	}
	return nil
}

func asString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(t)
	}
	return ""
}

func asInt(v interface{}) int {
	switch t := v.(type) {
	case float64:
		return int(t)
	case int:
		return t
	case string:
		if n, err := strconv.Atoi(t); err == nil {
			return n
		}
	}
	return 0
}

func asBool(v interface{}) bool {
	switch t := v.(type) {
	case bool:
		return t
	case string:
		return isTruthy(t)
	}
	return false
}
