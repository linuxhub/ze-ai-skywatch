package connections

import (
	"encoding/binary"
	"testing"
)

func TestWindowsOwnerPIDTables(t *testing.T) {
	for _, tc := range []struct {
		name, protocol  string
		ipv6            bool
		size, pid, port int
	}{
		{"TCP4", "TCP", false, 24, 20, 8}, {"TCP6", "TCP", true, 56, 52, 20},
		{"UDP4", "UDP", false, 12, 8, 4}, {"UDP6", "UDP", true, 28, 24, 20},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b := make([]byte, 4+tc.size)
			binary.LittleEndian.PutUint32(b, 1)
			row := b[4:]
			binary.LittleEndian.PutUint32(row[tc.pid:], 123)
			binary.BigEndian.PutUint16(row[tc.port:], 8080)
			if tc.ipv6 {
				row[0] = 0xfe
				row[1] = 0x80
				row[15] = 1
				binary.LittleEndian.PutUint32(row[16:], 7)
				if tc.protocol == "TCP" {
					row[39] = 1
					binary.LittleEndian.PutUint32(row[40:], 9)
					binary.BigEndian.PutUint16(row[44:], 443)
					binary.LittleEndian.PutUint32(row[48:], 5)
				}
			} else if tc.protocol == "TCP" {
				binary.LittleEndian.PutUint32(row, 5)
				copy(row[4:], []byte{127, 0, 0, 1})
				copy(row[12:], []byte{1, 2, 3, 4})
				binary.BigEndian.PutUint16(row[16:], 443)
			} else {
				copy(row, []byte{127, 0, 0, 1})
			}
			rows, err := parseWindowsTable(b, tc.protocol, tc.ipv6)
			if err != nil || len(rows) != 1 {
				t.Fatalf("rows=%+v err=%v", rows, err)
			}
			r := rows[0]
			if r.PID != 123 || r.Local.Port != "8080" {
				t.Fatal(r)
			}
			if tc.ipv6 && r.Local.IP != "fe80::1%7" {
				t.Fatal(r)
			}
			if !tc.ipv6 && r.Local.IP != "127.0.0.1" {
				t.Fatal(r)
			}
			if tc.protocol == "UDP" && (r.Remote.IP != "" || r.State != "" || r.Status() != "已绑定") {
				t.Fatal(r)
			}
			if tc.protocol == "TCP" {
				if r.State != "ESTABLISHED" || r.Remote.Port != "443" {
					t.Fatal(r)
				}
				if tc.ipv6 && r.Remote.IP != "::1%9" {
					t.Fatal(r)
				}
				// LISTEN's remote fields are undefined and must be discarded.
				stateOffset := 0
				if tc.ipv6 {
					stateOffset = 48
				}
				binary.LittleEndian.PutUint32(row[stateOffset:], 2)
				rows, err = parseWindowsTable(b, tc.protocol, tc.ipv6)
				if err != nil || rows[0].Remote.IP != "" || !rows[0].Listening() {
					t.Fatalf("%+v %v", rows, err)
				}
			}
			if _, err := parseWindowsTable(b[:len(b)-1], tc.protocol, tc.ipv6); err == nil {
				t.Fatal("accepted truncated row")
			}
		})
	}
	for _, b := range [][]byte{nil, {0}, {0xff, 0xff, 0xff, 0xff}} {
		if _, err := parseWindowsTable(b, "TCP", false); err == nil {
			t.Fatal("accepted malformed count")
		}
	}
}
