package network

import (
	"context"
	"fmt"
)

func Stream(ctx context.Context, publish func(Snapshot)) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("Windows 版暂不支持逐进程／逐连接流量统计；请在「网络连接」查看端点，在「网络流量」查看整机网速")
}
