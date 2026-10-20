// Package network separates inexpensive interface counters from opt-in socket analysis.
package network

import (
	"encoding/binary"
	"fmt"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/unix"
)

// Physical en* interfaces include Wi-Fi and Ethernet. Exclude utun, bridges,
// loopback and peer-to-peer interfaces rather than counting tunneled bytes twice.
func ReadCounters() ([]Counter, error) {
	rib, err := syscall.RouteRIB(unix.NET_RT_IFLIST2, 0)
	if err != nil {
		return nil, err
	}
	var out []Counter
	for len(rib) >= 4 {
		n := int(binary.NativeEndian.Uint16(rib[:2]))
		if n < 4 || n > len(rib) {
			return nil, fmt.Errorf("网卡计数消息长度无效")
		}
		msg := rib[:n]
		if msg[3] == unix.RTM_IFINFO2 && n >= unix.SizeofIfMsghdr2+8 {
			var hdr unix.IfMsghdr2
			copy(unsafe.Slice((*byte)(unsafe.Pointer(&hdr)), unix.SizeofIfMsghdr2), msg)
			sdl := msg[unix.SizeofIfMsghdr2:]
			nameLen := int(sdl[5])
			if nameLen > 0 && 8+nameLen <= len(sdl) {
				name := string(sdl[8 : 8+nameLen])
				if strings.HasPrefix(name, "en") && hdr.Flags&unix.IFF_LOOPBACK == 0 {
					out = append(out, Counter{name, hdr.Data.Ibytes, hdr.Data.Obytes})
				}
			}
		}
		rib = rib[n:]
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("未找到可读取的物理网卡（en*）")
	}
	return out, nil
}
