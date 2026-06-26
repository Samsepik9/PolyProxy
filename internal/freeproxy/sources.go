// Package freeproxy — sources.go: built-in free proxy sources (adapted from jhao104/proxy_pool).
package freeproxy

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// ProxyEntry is a raw proxy discovered from a public source.
type ProxyEntry struct {
	Addr   string `json:"addr"`   // host:port
	Type   string `json:"type"`   // http, socks5
	Source string `json:"source"` // which source it came from
}

// SourceType describes how to parse a source's response.
type SourceType string

const (
	SourceText SourceType = "text" // plain text, one ip:port per line
	SourceJSON SourceType = "json" // JSON response with a known path
	SourceHTML SourceType = "html" // HTML with regex extraction
)

// Source defines a proxy source.
type Source struct {
	Name    string     `json:"name" yaml:"name"`
	URL     string     `json:"url" yaml:"url"`
	Type    SourceType `json:"type" yaml:"type"`
	Enabled bool       `json:"enabled" yaml:"enabled"`

	// For JSON type: JSONPath-like expression (e.g. "data[*].ip")
	JSONPath string `json:"json_path,omitempty" yaml:"json_path,omitempty"`
	// For HTML/Text type: regex to extract ip:port
	Regex string `json:"regex,omitempty" yaml:"regex,omitempty"`
	// HTTP headers
	Headers map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`
}

// BuiltinSources returns all 14 built-in free proxy sources.
func BuiltinSources() []Source {
	return []Source{
		// 1. 快代理
		{
			Name: "kuaidaili", URL: "https://www.kuaidaili.com/free/inha/1/",
			Type: SourceHTML, Enabled: true,
			Regex: `<td[^>]*>(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})</td>\s*<td[^>]*>(\d{2,5})</td>`,
		},
		// 2. 小幻代理
		{
			Name: "ihuan", URL: "https://ip.ihuan.me/",
			Type: SourceHTML, Enabled: true,
			Regex: `<td[^>]*>(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})</td>\s*<td[^>]*>(\d{2,5})</td>`,
		},
		// 3. 89免费代理
		{
			Name: "ip89", URL: "https://www.89ip.cn/index_1.html",
			Type: SourceHTML, Enabled: true,
			Regex: `<td[^>]*>[\s\S]*?(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})[\s\S]*?</td>[\s\S]*?<td[^>]*>[\s\S]*?(\d+)[\s\S]*?</td>`,
		},
		// 4. 云代理
		{
			Name: "ip3366", URL: "http://www.ip3366.net/free/?stype=1",
			Type: SourceHTML, Enabled: true,
			Regex: `<td>(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})</td>[\s\S]*?<td>(\d+)</td>`,
		},
		// 5. 开心代理
		{
			Name: "kxdaili", URL: "http://www.kxdaili.com/dailiip.html",
			Type: SourceHTML, Enabled: true,
			Regex: `<td[^>]*>(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})</td>\s*<td[^>]*>(\d{2,5})</td>`,
		},
		// 6. 66代理
		{
			Name: "daili66", URL: "http://api.66daili.com/?format=json",
			Type: SourceJSON, Enabled: true,
			JSONPath: "data[*].ip,data[*].port",
		},
		// 7. 稻壳代理
		{
			Name: "docip", URL: "https://www.docip.net/data/free.json",
			Type: SourceJSON, Enabled: true,
			JSONPath: "data[*].ip",
		},
		// 8. FreeVPNNode
		{
			Name: "freevpnnode", URL: "https://cn.freevpnnode.com/free-proxy/",
			Type: SourceHTML, Enabled: true,
			Regex: `(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})[:\s]+(\d{2,5})`,
		},
		// 9. Geonode
		{
			Name: "geonode", URL: "https://proxylist.geonode.com/api/proxy-list?limit=100&page=1&sort_by=lastChecked&sort_type=desc",
			Type: SourceJSON, Enabled: true,
			JSONPath: "data[*].ip,data[*].port",
		},
		// 10. 谷德代理
		{
			Name: "goodips", URL: "https://www.goodips.com/",
			Type: SourceHTML, Enabled: true,
			Regex: `<li[^>]*>(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})</li>\s*<li[^>]*>(\d{2,5})</li>`,
		},
		// 11. Proxifly
		{
			Name: "proxifly", URL: "https://cdn.jsdelivr.net/gh/proxifly/free-proxy-list@main/proxies/all/data.json",
			Type: SourceJSON, Enabled: true,
			JSONPath: "[*].proxy",
		},
		// 12. RoundProxies
		{
			Name: "roundproxies", URL: "https://roundproxies.com/api/get-free-proxies/?limit=50&page=1&sort_by=lastChecked&sort_type=desc",
			Type: SourceJSON, Enabled: true,
			JSONPath: "data[*].ip,data[*].port",
		},
		// 13. SCDN
		{
			Name: "scdn", URL: "https://proxy.scdn.io/get_proxies.php?protocol=&country=&per_page=100&page=1",
			Type: SourceJSON, Enabled: true,
			JSONPath: "data[*].ip,data[*].port",
		},
		// 14. 站大爷
		{
			Name: "zdaye", URL: "https://www.zdaye.com/free/",
			Type: SourceHTML, Enabled: true,
			Regex: `<td[^>]*>(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})</td>\s*<td[^>]*>(\d{2,5})</td>`,
		},
	}
}

// --- Source parsing helpers ---

var defaultIPPortRegex = regexp.MustCompile(`(\d{1,3}\.\d{1,3}\.\d{1,3}\.\d{1,3})[:\s]+(\d{2,5})`)

// ParseSourceResponse extracts proxy entries from a source's HTTP response body.
func ParseSourceResponse(src Source, body []byte) ([]ProxyEntry, error) {
	switch src.Type {
	case SourceText:
		return parseText(body, src)
	case SourceJSON:
		return parseJSON(body, src)
	case SourceHTML:
		return parseHTML(body, src)
	default:
		return parseText(body, src)
	}
}

func parseText(body []byte, src Source) ([]ProxyEntry, error) {
	re := defaultIPPortRegex
	if src.Regex != "" {
		var err error
		re, err = regexp.Compile(src.Regex)
		if err != nil {
			return nil, fmt.Errorf("compile regex: %w", err)
		}
	}
	matches := re.FindAllSubmatch(body, -1)
	seen := map[string]bool{}
	var out []ProxyEntry
	for _, m := range matches {
		addr := string(m[1]) + ":" + string(m[2])
		if seen[addr] {
			continue
		}
		seen[addr] = true
		out = append(out, ProxyEntry{Addr: addr, Type: "http", Source: src.Name})
	}
	return out, nil
}

func parseHTML(body []byte, src Source) ([]ProxyEntry, error) {
	// HTML parsing uses regex — same as text
	return parseText(body, src)
}

func parseJSON(body []byte, src Source) ([]ProxyEntry, error) {
	if src.JSONPath == "" {
		// Fall back to regex on the raw JSON text
		return parseText(body, src)
	}

	var data any
	if err := json.Unmarshal(body, &data); err != nil {
		return nil, fmt.Errorf("json unmarshal: %w", err)
	}

	entries := extractByPath(data, src.JSONPath, src.Name)
	return entries, nil
}

// extractByPath is a simple JSON path extractor supporting:
//
//	"data[*].ip"              → single field, returns "ip" values
//	"data[*].ip,data[*].port" → two fields, joins as "ip:port"
//	"[*].proxy"               → root array, each item's "proxy" field
func extractByPath(data any, path, sourceName string) []ProxyEntry {
	parts := strings.Split(path, ",")
	if len(parts) == 2 {
		// Two-field mode: "data[*].ip,data[*].port"
		ips := extractField(data, strings.TrimSpace(parts[0]))
		ports := extractField(data, strings.TrimSpace(parts[1]))
		var out []ProxyEntry
		seen := map[string]bool{}
		for i := 0; i < len(ips) && i < len(ports); i++ {
			addr := ips[i] + ":" + ports[i]
			if seen[addr] {
				continue
			}
			seen[addr] = true
			out = append(out, ProxyEntry{Addr: addr, Type: "http", Source: sourceName})
		}
		return out
	}

	// Single-field mode
	values := extractField(data, strings.TrimSpace(path))
	var out []ProxyEntry
	seen := map[string]bool{}
	for _, v := range values {
		// v might already be "ip:port" or just "ip"
		addr := v
		if !strings.Contains(v, ":") {
			continue // can't use without port
		}
		if seen[addr] {
			continue
		}
		seen[addr] = true
		out = append(out, ProxyEntry{Addr: addr, Type: "http", Source: sourceName})
	}
	return out
}

func extractField(data any, path string) []string {
	segments := splitPath(path)
	if len(segments) == 0 {
		return nil
	}

	current := data
	for i, seg := range segments {
		if seg == "[*]" {
			arr, ok := current.([]any)
			if !ok {
				return nil
			}
			// Collect remaining path from all array elements
			remaining := strings.Join(segments[i+1:], ".")
			var result []string
			for _, item := range arr {
				result = append(result, extractField(item, remaining)...)
			}
			return result
		}
		m, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = m[seg]
		if current == nil {
			return nil
		}
	}

	switch v := current.(type) {
	case string:
		return []string{v}
	case float64:
		return []string{fmt.Sprintf("%.0f", v)}
	case []any:
		var result []string
		for _, item := range v {
			switch iv := item.(type) {
			case string:
				result = append(result, iv)
			case float64:
				result = append(result, fmt.Sprintf("%.0f", iv))
			}
		}
		return result
	}
	return nil
}

// splitPath splits a JSON path like "data[*].ip" into ["data", "[*]", "ip"].
func splitPath(path string) []string {
	var segments []string
	start := 0
	for i := 0; i < len(path); i++ {
		if path[i] == '.' {
			if i > start {
				segments = append(segments, path[start:i])
			}
			start = i + 1
		} else if path[i] == '[' && i+2 < len(path) && path[i:i+3] == "[*]" {
			if i > start {
				segments = append(segments, path[start:i])
			}
			segments = append(segments, "[*]")
			start = i + 3
			i = start - 1 // will be incremented by loop
		}
	}
	if start < len(path) {
		segments = append(segments, path[start:])
	}
	return segments
}

// FetchSourceBody fetches the raw body from a source URL.
func FetchSourceBody(url string, timeout time.Duration, headers map[string]string) ([]byte, error) {
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	return io.ReadAll(io.LimitReader(resp.Body, 2<<20)) // 2 MB max
}
