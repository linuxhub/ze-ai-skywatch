package appidentity

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
)

func inspectPlatform(ctx context.Context, info *Info) {
	if index := strings.Index(strings.ToLower(info.Executable), ".app/"); index >= 0 {
		info.AppPath = info.Executable[:index+4]
		data, err := command(ctx, nil, "/usr/bin/plutil", "-convert", "json", "-o", "-", filepath.Join(info.AppPath, "Contents", "Info.plist"))
		if err == nil {
			applyBundle(data, info)
		}
	}
	data, _ := command(ctx, nil, "/usr/bin/codesign", "-d", "--verbose=4", "--", info.Executable)
	applySignature(string(data), info)
	if _, err := command(ctx, nil, "/usr/bin/codesign", "--verify", "--strict", "--", info.Executable); err == nil {
		info.Signature = "磁盘文件签名校验通过（非安全评级）"
		if info.Signer == "" {
			info.Signature = "临时／无发布者签名，文件完整性校验通过"
		}
	} else {
		info.Signature = "签名未通过校验或不可验证"
	}
}

func applyBundle(data []byte, info *Info) {
	var metadata map[string]any
	if json.Unmarshal(data, &metadata) != nil {
		return
	}
	value := func(key string) string { text, _ := metadata[key].(string); return text }
	info.AppName = value("CFBundleDisplayName")
	if info.AppName == "" {
		info.AppName = value("CFBundleName")
	}
	info.BundleID, info.Version = value("CFBundleIdentifier"), value("CFBundleShortVersionString")
	if info.Version == "" {
		info.Version = value("CFBundleVersion")
	}
}

func applySignature(data string, info *Info) {
	for _, line := range strings.Split(data, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch key {
		case "Authority":
			if info.Signer == "" {
				info.Signer = value
			}
		case "TeamIdentifier":
			if value != "not set" {
				info.TeamID = value
			}
		}
	}
}
