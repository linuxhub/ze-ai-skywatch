package network

import (
	"fmt"
	"golang.org/x/sys/windows"
	"net"
)

// Windows reports hardware counters directly; omit virtual, filter, loopback
// and tunnel interfaces rather than stacking multiple layers of the same traffic.
func ReadCounters() ([]Counter, error) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	var rows []Counter
	var firstErr error
	for _, iface := range interfaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		row := windows.MibIfRow2{InterfaceIndex: uint32(iface.Index)}
		if err := windows.GetIfEntry2Ex(0, &row); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		const hardwareInterface = 1
		const filterInterface = 1 << 1
		if row.InterfaceAndOperStatusFlags&hardwareInterface == 0 || row.InterfaceAndOperStatusFlags&filterInterface != 0 || (row.Type != 6 && row.Type != 71) {
			continue
		}
		// Use the index as well as the name so a renamed adapter starts a baseline.
		rows = append(rows, Counter{Name: fmt.Sprintf("%s (#%d)", iface.Name, iface.Index), In: row.InOctets, Out: row.OutOctets})
	}
	if len(rows) == 0 {
		if firstErr != nil {
			return nil, fmt.Errorf("无法读取网卡计数: %w", firstErr)
		}
		return nil, fmt.Errorf("未找到可读取的物理以太网或 Wi-Fi 网卡")
	}
	return rows, nil
}
