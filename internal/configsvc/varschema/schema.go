// internal/configsvc/varschema/schema.go
package varschema

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
)

type VarType string

const (
	TString VarType = "string"
	TBool   VarType = "bool"
	TInt    VarType = "int"
	TList   VarType = "list" // comma-separated or multi-line
	TIPv4   VarType = "ipv4"
	TIPv6   VarType = "ipv6"
)

type VarDef struct {
	Key      string
	Type     VarType
	Example  string
	Validate func(string) (string, error)               // нормализация/проверка одного значения
	Required bool                                       // безусловно обязателен (редко)
	Requires func(get func(string) (string, bool)) bool // условно обязателен (зависит от других vars)
}

/* ——— validators ——— */

var (
	reHostname = regexp.MustCompile(`^(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)(?:\.(?i:[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?))*$`)
	reTZ       = regexp.MustCompile(`^[A-Za-z]+(?:/[A-Za-z0-9_\-+]+)+$`)
)

func normHostname(v string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(v))
	if s == "" || len(s) > 253 || !reHostname.MatchString(s) {
		return "", fmt.Errorf("invalid hostname")
	}
	return s, nil
}
func normTZ(v string) (string, error) {
	s := strings.TrimSpace(v)
	if s == "" || len(s) > 128 || !reTZ.MatchString(s) {
		return "", fmt.Errorf("invalid timezone (Area/City)")
	}
	return s, nil
}
func normBool(v string) (string, error) {
	s := strings.ToLower(strings.TrimSpace(v))
	switch s {
	case "1", "true", "yes", "on":
		return "1", nil
	case "0", "false", "no", "off":
		return "0", nil
	}
	return "", errors.New("invalid bool")
}
func normInt(min, max int) func(string) (string, error) {
	return func(v string) (string, error) {
		s := strings.TrimSpace(v)
		n, err := strconv.Atoi(s)
		if err != nil {
			return "", err
		}
		if n < min || n > max {
			return "", fmt.Errorf("int out of range [%d..%d]", min, max)
		}
		return strconv.Itoa(n), nil
	}
}
func normIPv4(v string) (string, error) {
	ip := net.ParseIP(strings.TrimSpace(v))
	if ip == nil || ip.To4() == nil {
		return "", errors.New("invalid ipv4")
	}
	return ip.To4().String(), nil
}
func normIPv6(v string) (string, error) {
	ip := net.ParseIP(strings.TrimSpace(v))
	if ip == nil || ip.To16() == nil || ip.To4() != nil {
		return "", errors.New("invalid ipv6")
	}
	return ip.String(), nil
}
func normNetmask(v string) (string, error) {
	s := strings.TrimSpace(v)
	if n, err := strconv.Atoi(s); err == nil && n >= 0 && n <= 32 {
		mask := net.CIDRMask(n, 32)
		return net.IP(mask).String(), nil
	}
	ip := net.ParseIP(s)
	if ip == nil || ip.To4() == nil {
		return "", errors.New("invalid netmask")
	}
	return ip.To4().String(), nil
}
func normList(v string) (string, error) {
	s := strings.TrimSpace(v)
	if s == "" {
		return "", errors.New("empty list")
	}
	// корректно: функция-предикат должна быть func(rune) bool
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == ',' || r == '\n' || r == '\r'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return strings.Join(out, ","), nil
}
func normSSHKey(v string) (string, error) {
	s := strings.TrimSpace(v)
	if s == "" {
		return "", errors.New("empty ssh key")
	}
	if !(strings.HasPrefix(s, "ssh-rsa ") || strings.HasPrefix(s, "ssh-ed25519 ") || strings.HasPrefix(s, "ecdsa-")) {
		return "", errors.New("unsupported ssh key type")
	}
	return s, nil
}
func normWiFiPSK(v string) (string, error) {
	s := strings.TrimSpace(v)
	if l := len(s); l < 8 || l > 63 {
		return "", errors.New("wifi psk must be 8..63 chars")
	}
	return s, nil
}
func pass(v string) (string, error) { return strings.TrimSpace(v), nil }

/* ——— catalog ——— */
// helper для Requires:
func has(get func(string) (string, bool), key string) bool { _, ok := get(key); return ok }

// NB: это “разумный минимум” для OpenWrt. Шаблоны могут требовать больше.
var Catalog = []VarDef{
	// System
	{Key: "hostname", Type: TString, Example: "branch-ap-01", Validate: normHostname, Required: true},
	{Key: "timezone", Type: TString, Example: "Europe/Rome", Validate: normTZ},

	// Uplink (WAN)
	{Key: "wan_proto", Type: TString, Example: "dhcp|static|pppoe", Validate: func(s string) (string, error) {
		v := strings.ToLower(strings.TrimSpace(s))
		switch v {
		case "dhcp", "static", "pppoe":
			return v, nil
		}
		return "", errors.New("wan_proto must be dhcp|static|pppoe")
	}, Required: true},
	{Key: "wan_iface", Type: TString, Example: "eth0", Validate: pass},

	// IPv4 static (требуются только при static)
	{Key: "ipv4_address", Type: TIPv4, Example: "10.100.0.2", Validate: normIPv4, Requires: func(get func(string) (string, bool)) bool {
		v, _ := get("wan_proto")
		return v == "static"
	}},
	{Key: "ipv4_netmask", Type: TIPv4, Example: "255.255.255.0", Validate: normNetmask, Requires: func(get func(string) (string, bool)) bool {
		v, _ := get("wan_proto")
		return v == "static"
	}},
	{Key: "ipv4_gateway", Type: TIPv4, Example: "10.100.0.1", Validate: normIPv4, Requires: func(get func(string) (string, bool)) bool {
		v, _ := get("wan_proto")
		return v == "static"
	}},
	{Key: "dns_servers", Type: TList, Example: "1.1.1.1,8.8.8.8", Validate: normList}, // 1..N

	// IPv6 (опционально)
	{Key: "ipv6_enable", Type: TBool, Example: "1", Validate: normBool},
	{Key: "ipv6_address", Type: TIPv6, Example: "2001:db8::2", Validate: normIPv6, Requires: func(get func(string) (string, bool)) bool {
		return has(get, "ipv6_enable") // если включён — адрес желателен
	}},
	{Key: "ipv6_prefixlen", Type: TInt, Example: "64", Validate: normInt(0, 128)},
	{Key: "ipv6_gateway", Type: TIPv6, Example: "fe80::1", Validate: normIPv6},
	{Key: "dns6_servers", Type: TList, Example: "2606:4700:4700::1111,2001:4860:4860::8888", Validate: normList},

	// LAN/Bridge/VLAN (минимум)
	{Key: "lan_iface", Type: TString, Example: "br-lan", Validate: pass},
	{Key: "lan_vlan_id", Type: TInt, Example: "1", Validate: normInt(1, 4094)},
	{Key: "mgmt_vlan_id", Type: TInt, Example: "10", Validate: normInt(1, 4094)},

	// NTP / Syslog
	{Key: "ntp_servers", Type: TList, Example: "pool.ntp.org,time.cloudflare.com", Validate: normList},
	{Key: "syslog_server", Type: TString, Example: "10.0.0.10", Validate: pass},

	// SSH
	{Key: "ssh_authorized_keys", Type: TList, Example: "ssh-ed25519 AAA...,ssh-rsa BBB...", Validate: normList},

	// Wi-Fi (базовые)
	{Key: "wifi_country", Type: TString, Example: "IT", Validate: func(s string) (string, error) {
		s = strings.ToUpper(strings.TrimSpace(s))
		if len(s) != 2 {
			return "", errors.New("country must be ISO 3166-1 alpha-2")
		}
		return s, nil
	}},
	{Key: "wifi_band", Type: TString, Example: "2g|5g|6g", Validate: func(s string) (string, error) {
		v := strings.ToLower(strings.TrimSpace(s))
		switch v {
		case "2g", "5g", "6g":
			return v, nil
		}
		return "", errors.New("wifi_band must be 2g|5g|6g")
	}},
	{Key: "wifi_channel", Type: TInt, Example: "auto|1..165", Validate: normIntOrAuto(1, 196)}, // "auto" можно трактовать как пусто и логикой шаблона
	{Key: "wifi_htmode", Type: TString, Example: "HT20|VHT40|HE80", Validate: pass},
	{Key: "wifi_ssid", Type: TString, Example: "CorpWiFi", Validate: func(s string) (string, error) {
		s = strings.TrimSpace(s)
		if s == "" {
			return "", errors.New("empty ssid")
		}
		return s, nil
	}},
	{Key: "wifi_encryption", Type: TString, Example: "psk2|sae|psk-mixed", Validate: func(s string) (string, error) {
		v := strings.ToLower(strings.TrimSpace(s))
		switch v {
		case "psk2", "psk-mixed", "sae":
			return v, nil
		}
		return "", errors.New("wifi_encryption invalid")
	}},
	{Key: "wifi_psk", Type: TString, Example: "********", Validate: normWiFiPSK},
}

/* ——— registry ——— */

var byKey map[string]VarDef

func init() {
	byKey = make(map[string]VarDef, len(Catalog))
	for _, d := range Catalog {
		byKey[d.Key] = d
	}
}

func Def(key string) (VarDef, bool) { d, ok := byKey[key]; return d, ok }

// ValidateOne validates and normalizes a single var by key.
func ValidateOne(key, value string) (string, error) {
	if def, ok := Def(key); ok {
		return def.Validate(value)
	}
	return "", fmt.Errorf("unknown variable: %s", key)
}

// ValidateAll checks conditional requirements against a getter (merged vars).
func ValidateAll(get func(string) (string, bool)) error {
	missing := []string{}
	for _, d := range Catalog {
		need := d.Required
		if d.Requires != nil && d.Requires(get) {
			need = true
		}
		if need {
			if v, ok := get(d.Key); !ok || strings.TrimSpace(v) == "" {
				missing = append(missing, d.Key)
			}
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required vars: %v", missing)
	}
	return nil
}

func normIntOrAuto(min, max int) func(string) (string, error) {
	return func(v string) (string, error) {
		s := strings.TrimSpace(strings.ToLower(v))
		if s == "auto" || s == "" {
			return "", nil // пустая строка = auto, шаблон сам подставит
		}
		n, err := strconv.Atoi(s)
		if err != nil {
			return "", err
		}
		if n < min || n > max {
			return "", fmt.Errorf("int out of range [%d..%d]", min, max)
		}
		return strconv.Itoa(n), nil
	}
}
