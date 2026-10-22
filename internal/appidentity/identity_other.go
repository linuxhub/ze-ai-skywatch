//go:build !darwin && !windows

package appidentity

import "context"

func inspectPlatform(ctx context.Context, info *Info) {
	info.Signature = "当前平台未提供签名检查"
}
