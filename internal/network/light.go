package network

import "time"

type Counter struct {
	Name    string
	In, Out uint64
}
type LightSnapshot struct {
	At               time.Time
	Download, Upload float64
	Received, Sent   uint64
	Interfaces       []string
	Ready            bool
	Err              string
}
type LightSampler struct {
	previous       map[string]Counter
	at             time.Time
	received, sent uint64
}

func (s *LightSampler) Sample() LightSnapshot {
	rows, err := ReadCounters()
	if err != nil {
		return LightSnapshot{Err: "网速采集失败: " + err.Error()}
	}
	return s.Update(time.Now(), rows)
}

func (s *LightSampler) Update(at time.Time, rows []Counter) LightSnapshot {
	out := LightSnapshot{At: at}
	next := make(map[string]Counter, len(rows))
	var incoming, outgoing uint64
	for _, row := range rows {
		next[row.Name] = row
		out.Interfaces = append(out.Interfaces, row.Name)
		if prev, ok := s.previous[row.Name]; ok {
			if row.In >= prev.In {
				incoming += row.In - prev.In
			}
			if row.Out >= prev.Out {
				outgoing += row.Out - prev.Out
			}
		}
	}
	if dt := at.Sub(s.at).Seconds(); !s.at.IsZero() && dt > 0 {
		out.Ready = true
		out.Download = float64(incoming) / dt
		out.Upload = float64(outgoing) / dt
		s.received += incoming
		s.sent += outgoing
	}
	out.Received = s.received
	out.Sent = s.sent
	s.previous = next
	s.at = at
	return out
}
