// Package ipnotes stores user-authored IP labels, separate from geo lookups.
package ipnotes

import (
	"fmt"
	"net/netip"
	"strings"
)

type Note struct {
	IP          string `json:"ip"`
	Name        string `json:"name"`
	Group       string `json:"group,omitempty"`
	Description string `json:"description,omitempty"`
}

func NormalizeIP(raw string) string {
	ip, err := netip.ParseAddr(strings.Trim(strings.TrimSpace(raw), "[]"))
	if err != nil {
		return ""
	}
	return ip.WithZone("").Unmap().String()
}

func Validate(n Note) (Note, error) {
	n.IP = NormalizeIP(n.IP)
	n.Name, n.Group, n.Description = strings.TrimSpace(n.Name), strings.TrimSpace(n.Group), strings.TrimSpace(n.Description)
	if n.IP == "" {
		return n, fmt.Errorf("请输入有效的 IPv4 或 IPv6 地址，不包含端口或网段")
	}
	if n.Name == "" {
		return n, fmt.Errorf("请填写标注名称")
	}
	if len([]rune(n.Name)) > 80 || len([]rune(n.Group)) > 40 || len([]rune(n.Description)) > 1000 {
		return n, fmt.Errorf("名称最多 80 字，分组最多 40 字，说明最多 1000 字")
	}
	return n, nil
}
