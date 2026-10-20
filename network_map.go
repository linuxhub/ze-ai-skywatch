package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/egoist/mygo/ui"
	"github.com/linuxhub/ze-ai-skywatch/internal/network"
)

// Natural Earth 1:110m land, public domain; see resources/README.md.
//
//go:embed resources/world-land.json
var landJSON []byte
var landCache struct {
	sync.Once
	bitmap *ui.Bitmap
}

// Rasterize the coastline once. Animated frames reuse a small texture instead
// of tessellating thousands of coastline vertices at 30 fps.
func landBitmap() *ui.Bitmap {
	landCache.Do(func() {
		var rings [][][2]float64
		if json.Unmarshal(landJSON, &rings) != nil {
			return
		}
		const width, height = 1200, 500
		img := image.NewRGBA(image.Rect(0, 0, width, height))
		ink := color.RGBA{R: 22, G: 54, B: 77, A: 255}
		for _, ring := range rings {
			if len(ring) < 3 {
				continue
			}
			points := make([][2]float64, len(ring))
			for i, p := range ring {
				points[i] = [2]float64{(p[0] + 180) / 360 * width, (85 - p[1]) / 145 * height}
			}
			for y := 0; y < height; y++ {
				scan := float64(y) + 0.5
				var xs []float64
				for i, a := range points {
					b := points[(i+1)%len(points)]
					if (a[1] <= scan && b[1] > scan) || (b[1] <= scan && a[1] > scan) {
						xs = append(xs, a[0]+(scan-a[1])*(b[0]-a[0])/(b[1]-a[1]))
					}
				}
				sort.Float64s(xs)
				for i := 0; i+1 < len(xs); i += 2 {
					for x := max(0, int(math.Ceil(xs[i]))); x < min(width, int(math.Ceil(xs[i+1]))); x++ {
						img.SetRGBA(x, y, ink)
					}
				}
			}
		}
		landCache.bitmap = ui.NewBitmap(img)
	})
	return landCache.bitmap
}

type mapDestination struct {
	IP               string
	Connection       network.Connection
	Download, Upload float64
	Location         network.Location
	X, Y             float32
}

func (a *app) mapDestinations() []mapDestination {
	key := a.networkProjection()
	key.Order = ""
	if key == a.network.mapKey && a.network.mapRows != nil {
		return a.network.mapRows
	}
	groups := map[string]*mapDestination{}
	for _, c := range a.networkConnections() {
		ip := c.RemoteIP()
		if ip == "" {
			continue
		}
		g := groups[ip]
		if g == nil {
			g = &mapDestination{IP: ip, Connection: c, Location: a.network.geo[ip]}
			groups[ip] = g
		}
		g.Download += c.Download
		g.Upload += c.Upload
		if c.Key() == a.network.selectedConnection {
			g.Connection = c
		}
	}
	var rows []mapDestination
	for _, g := range groups {
		rows = append(rows, *g)
	}
	sort.Slice(rows, func(i, j int) bool {
		aSelected := rows[i].Connection.Key() == a.network.selectedConnection
		bSelected := rows[j].Connection.Key() == a.network.selectedConnection
		if aSelected != bSelected {
			return aSelected
		}
		if rows[i].Download+rows[i].Upload != rows[j].Download+rows[j].Upload {
			return rows[i].Download+rows[i].Upload > rows[j].Download+rows[j].Upload
		}
		return rows[i].IP < rows[j].IP
	})
	if len(rows) > 20 {
		rows = rows[:20]
	}
	unknown := 0
	for i := range rows {
		d := &rows[i]
		if d.Location.Known {
			d.X = float32((d.Location.Longitude + 180) / 360)
			d.Y = float32((85 - d.Location.Latitude) / 145)
			d.X = max(0.03, min(0.97, d.X))
			d.Y = max(0.08, min(0.78, d.Y))
		} else {
			// Unlocated / local connections use visibly separate schematic docks.
			col := unknown % 2
			row := unknown / 2
			d.X = 0.04 + float32(col)*0.92
			d.Y = 0.16 + float32(row)*0.06
			unknown++
		}
	}
	a.network.mapKey = key
	a.network.mapRows = rows
	return rows
}

func (a *app) networkMap(c *ui.Context) {
	n := &a.network
	destinations := a.mapDestinations()
	bitmap := landBitmap()
	animate := false
	if n.enabled && n.visible && a.config.NetworkAnimation {
		for _, d := range destinations {
			if d.Download+d.Upload > 0 {
				animate = true
				break
			}
		}
	}
	ui.Column(c).Grow(1).MinHeight(0).Radius(10).Background(ui.Hex("#071421")).Clip().Children(func() {
		ui.Row(c).Shrink(0).Padding(9, 12).Gap(7).Children(func() {
			ui.Text(c, "连接去向").FontSize(12).Bold().TextColor(ui.Hex("#edf5ff"))
			ui.Spacer(c)
			ui.Checkbox(c.Key("map-animation"), &a.config.NetworkAnimation, "光纤动画").FontSize(10).TextColor(ui.Hex("#b5c9dc")).OnChange(a.persist)
		})
		viewport := ui.Box(c.Key("network-map-viewport")).Grow(1).MinHeight(0).Clip().Center()
		bounds := viewport.Bounds()
		width, height := c.Size()
		windowSize := [2]float32{width, height}
		viewportSize := [2]float32{bounds.W, bounds.H}
		notices := [4]bool{n.light.Err != "", n.err != "", n.snapshot.Dropped > 0, n.geoPaused}
		// Bounds are from the committed frame. Settle the fitted layer after
		// a resize/notice changes the layout; painting itself stays stateless.
		if n.mapWindowSize != windowSize || n.mapViewportSize != viewportSize || n.mapLayoutNotices != notices {
			n.mapWindowSize, n.mapViewportSize, n.mapLayoutNotices = windowSize, viewportSize, notices
			c.Invalidate()
		}
		viewport.Children(func() {
			layer := ui.Box(c.Key("network-map-layer")).Shrink(0).Label("连接去向地图")
			if bounds.W > 0 && bounds.H > 0 {
				// Fit both the bitmap and interactive nodes in the same 12:5
				// plane so geographic locations keep their alignment.
				w := min(bounds.W, bounds.H*2.4)
				layer.Size(w, w/2.4)
			} else {
				layer.Grow(1).MinHeight(0)
			}
			layer.Clip().Draw(func(p *ui.Painter, r ui.Rect) {
				// Repaint only: particle movement never rebuilds tables or re-sorts
				// connection data. Re-check visibility so a queued timer stops on hide.
				moving := animate && n.visible && n.enabled && a.config.NetworkAnimation && a.page == "network" && !n.detailOpen
				if moving {
					p.After(time.Second / 30)
				}
				phase := float64(p.Now().UnixMilli()%60000) / 1000
				if bitmap != nil {
					p.Image(bitmap, r, ui.FillBox)
				}
				for i := 1; i < 6; i++ {
					y := r.Y + r.H*float32(i)/6
					p.Line(r.X, y, r.X+r.W, y, 0.5, ui.RGBA(105, 169, 211, 0.09))
				}
				for i := 1; i < 12; i++ {
					x := r.X + r.W*float32(i)/12
					p.Line(x, r.Y, x, r.Y+r.H, 0.5, ui.RGBA(105, 169, 211, 0.07))
				}
				origin := [2]float32{r.X + r.W*0.5, r.Y + r.H*0.88}
				for i, d := range destinations {
					end := [2]float32{r.X + d.X*r.W, r.Y + d.Y*r.H}
					ctrl := [2]float32{(origin[0] + end[0]) / 2, min(origin[1], end[1]) - r.H*0.20}
					width := float32(0.8 + math.Min(2, math.Log1p((d.Download+d.Upload)/1024)/4))
					selected := d.Connection.Key() == n.selectedConnection
					if selected {
						width += 1
					}
					for _, direction := range []struct {
						rate    float64
						color   ui.Color
						reverse bool
						offset  float32
					}{{d.Download, networkCyan, true, -1}, {d.Upload, networkOrange, false, 1}} {
						if direction.rate == 0 && d.Download+d.Upload > 0 {
							continue
						}
						var path ui.Path
						path.MoveTo(origin[0], origin[1]).QuadTo(ctrl[0], ctrl[1]+direction.offset*3, end[0], end[1])
						alpha := float32(0.45)
						if direction.rate == 0 {
							alpha = 0.12
						}
						p.StrokePath(&path, width+4, direction.color.Alpha(alpha*0.08))
						p.StrokePath(&path, width, direction.color.Alpha(alpha))
						if moving && direction.rate > 0 {
							count := 1 + min(3, int(math.Log1p(direction.rate/1024)/3))
							for j := 0; j < count; j++ {
								t := math.Mod(phase*0.32+float64(i)*0.137+float64(j)/float64(count), 1)
								if direction.reverse {
									t = 1 - t
								}
								x, y := curvePoint(origin, ctrl, end, float32(t))
								var glow, dot ui.Path
								glow.Circle(x, y, 5)
								dot.Circle(x, y, 1.8)
								p.FillPath(&glow, direction.color.Alpha(0.15))
								p.FillPath(&dot, direction.color)
							}
						}
					}
				}
				var halo, node ui.Path
				halo.Circle(origin[0], origin[1], 15)
				node.Circle(origin[0], origin[1], 7)
				p.FillPath(&halo, networkCyan.Alpha(0.12))
				p.FillPath(&node, networkCyan)
				p.Text(origin[0]+12, origin[1]-5, "本机（示意）", 10, ui.Hex("#d7e9fa"))
			}).Children(func() {
				for i, d := range destinations {
					color := networkCyan
					if d.Upload > d.Download {
						color = networkOrange
					}
					label := d.IP + " · " + a.noteSearch(d.IP) + " · " + d.Location.Label() + " · ↓ " + rateText(d.Download, true) + " ↑ " + rateText(d.Upload, true)
					ui.ButtonBase(c.Key(d.IP)).Absolute().LeftPercent(d.X*100).TopPercent(d.Y*100).Translate(-5, -5).Size(11, 11).Radius(6).Background(color).Border(1, ui.Hex("#edf5ff")).Label(label).Tooltip(label).OnClick(func() { n.selectedConnection = d.Connection.Key(); n.detail = d.Connection; n.detailOpen = true })
					if i < 3 {
						dx := float32(10)
						if d.X > 0.65 {
							dx = -130
						}
						ui.Text(c.Key("label-"+d.IP), d.IP).Absolute().LeftPercent(d.X*100).TopPercent(d.Y*100).Translate(dx, -5).Width(125).FontSize(9).TextColor(ui.Hex("#d5e7f6")).SingleLine()
					}
				}
				if len(destinations) == 0 {
					ui.Text(c, "等待连接数据…").Absolute().Left(14).Top(12).FontSize(11).TextColor(ui.Hex("#aec5da"))
				}
			})
		})
		ui.Row(c).Padding(6, 12, 8).Gap(8).Shrink(0).Children(func() {
			ui.Text(c, "↓ 下载").FontSize(9).TextColor(networkCyan)
			ui.Text(c, "↑ 上传").FontSize(9).TextColor(networkOrange)
			ui.Spacer(c)
			text := "两侧为位置未知的节点"
			if !a.config.NetworkGeo {
				text = "位置查询未开启 · 节点为示意"
			}
			ui.Text(c, text).FontSize(9).TextColor(ui.Hex("#8ca6bc"))
		})
	})
}
func curvePoint(a, b, c [2]float32, t float32) (float32, float32) {
	u := 1 - t
	return u*u*a[0] + 2*u*t*b[0] + t*t*c[0], u*u*a[1] + 2*u*t*b[1] + t*t*c[1]
}

func (a *app) networkChart(c *ui.Context, height float32) {
	surface, _ := a.palette(c)
	points := a.network.history
	peak := float64(1024)
	for _, p := range points {
		peak = max(peak, p.Download, p.Upload)
	}
	ui.Column(c).Height(height).Shrink(0).Padding(9, 12).Gap(5).Radius(9).Background(surface).Border(1, c.Theme().Border.Alpha(0.7)).Children(func() {
		ui.Row(c).Gap(8).Children(func() {
			ui.Text(c, "物理网卡趋势").FontSize(10).Bold().Grow(1)
			ui.Text(c, "↓ 下载").FontSize(9).TextColor(blue)
			ui.Text(c, "↑ 上传").FontSize(9).TextColor(networkOrange)
			ui.Text(c, fmt.Sprintf("峰值 %s", rateText(peak, true))).FontSize(9).TextColor(c.Theme().TextMuted)
		})
		border := c.Theme().Border
		ui.Box(c).Grow(1).MinHeight(0).Draw(func(p *ui.Painter, r ui.Rect) {
			for i := 0; i < 3; i++ {
				y := r.Y + r.H*float32(i)/2
				p.Line(r.X, y, r.X+r.W, y, 0.5, border.Alpha(0.65))
			}
			if len(points) < 2 {
				return
			}
			for _, series := range []struct {
				up    bool
				color ui.Color
			}{{false, blue}, {true, networkOrange}} {
				var path ui.Path
				for i, point := range points {
					v := point.Download
					if series.up {
						v = point.Upload
					}
					x := r.X + r.W*float32(i)/float32(len(points)-1)
					y := r.Y + r.H - float32(v/peak)*r.H*0.9
					if i == 0 {
						path.MoveTo(x, y)
					} else {
						path.LineTo(x, y)
					}
				}
				p.StrokePath(&path, 1.5, series.color)
			}
		})
	})
}
