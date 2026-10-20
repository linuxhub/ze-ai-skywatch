package connections

import (
	"encoding/binary"
	"fmt"
	"net/netip"
	"strconv"
)

// Windows OWNER_PID tables consist of a DWORD count and DWORD-aligned rows.
// Decode bytes explicitly so malformed lengths cannot become unsafe pointers.
func parseWindowsTable(data []byte, protocol string, ipv6 bool) ([]Connection, error) {
	size := 24
	if protocol == "UDP" {
		size = 12
	}
	if ipv6 {
		size = 56
		if protocol == "UDP" {
			size = 28
		}
	}
	if len(data) < 4 {
		return nil, fmt.Errorf("连接表头被截断")
	}
	count := uint64(binary.LittleEndian.Uint32(data))
	if count > uint64((len(data)-4)/size) {
		return nil, fmt.Errorf("连接表行被截断")
	}
	rows := make([]Connection, 0, min(int(count), MaxRows+1))
	states := []string{"", "CLOSED", "LISTEN", "SYN_SENT", "SYN_RECEIVED", "ESTABLISHED", "FIN_WAIT_1", "FIN_WAIT_2", "CLOSE_WAIT", "CLOSING", "LAST_ACK", "TIME_WAIT", "DELETE_TCB"}
	for i := 0; i < int(count) && i <= MaxRows; i++ {
		b := data[4+i*size : 4+(i+1)*size]
		u32 := func(offset int) uint32 { return binary.LittleEndian.Uint32(b[offset : offset+4]) }
		port := func(offset int) string { return strconv.Itoa(int(binary.BigEndian.Uint16(b[offset : offset+2]))) }
		ip := func(offset int) string {
			if !ipv6 {
				return netip.AddrFrom4([4]byte(b[offset : offset+4])).String()
			}
			addr := netip.AddrFrom16([16]byte(b[offset : offset+16]))
			if scope := u32(offset + 16); scope != 0 {
				addr = addr.WithZone(strconv.FormatUint(uint64(scope), 10))
			}
			return addr.String()
		}
		row := Connection{Protocol: protocol, Family: "IPv4"}
		var state uint32
		if ipv6 {
			row.Family = "IPv6"
			row.Local = Endpoint{ip(0), port(20)}
			if protocol == "TCP" {
				row.Remote = Endpoint{ip(24), port(44)}
				state = u32(48)
				row.PID = int32(u32(52))
			} else {
				row.PID = int32(u32(24))
			}
		} else if protocol == "TCP" {
			state = u32(0)
			row.Local = Endpoint{ip(4), port(8)}
			row.Remote = Endpoint{ip(12), port(16)}
			row.PID = int32(u32(20))
		} else {
			row.Local = Endpoint{ip(0), port(4)}
			row.PID = int32(u32(8))
		}
		if state < uint32(len(states)) {
			row.State = states[state]
		}
		if row.State == "LISTEN" {
			row.Remote = Endpoint{}
		}
		// UDP tables expose local bindings only; never invent a connected peer.
		rows = append(rows, row)
	}
	return rows, nil
}
