package exporter

import (
	"math"

	"github.com/danopstech/starlink_exporter/pkg/spacex.com/api/device"
)

const (
	// obstructedBelowSnr is the map value under which a cell counts as
	// obstructed. Cells hold a 0..1 signal quality, or -1 where no satellite
	// has been seen through that direction since the map was last cleared.
	obstructedBelowSnr = 0.5

	// bandDegrees is the elevation band height used to tell a real skyline
	// from isolated noisy cells.
	bandDegrees = 5.0

	// bandObstructedFraction is the share of a band's observed cells that must
	// be obstructed for the band to count as part of the skyline.
	bandObstructedFraction = 0.25
)

// sectorNames are the 16 compass sectors, each 22.5 degrees wide and centred
// on its heading.
var sectorNames = []string{
	"N", "NNE", "NE", "ENE", "E", "ESE", "SE", "SSE",
	"S", "SSW", "SW", "WSW", "W", "WNW", "NW", "NNW",
}

// sectorStats summarises one compass sector of the obstruction map.
type sectorStats struct {
	name string
	// observed is the share of the sector's cells that have data yet.
	observed float64
	// obstructed is the share of observed cells that are obstructed. Only
	// meaningful when observed > 0.
	obstructed float64
	// clearAbove is the elevation in degrees from which every observed cell
	// in the sector is clear: the top of the skyline, or the lowest observed
	// elevation when nothing is obstructed. Only meaningful when observed > 0.
	clearAbove float64
}

// sectorObstructions folds the dish's obstruction map into the 16 compass
// sectors. The map is a square polar plot of the sky centred on the zenith,
// north up and east right when the reference frame is FRAME_EARTH, with the
// radius growing linearly from the zenith to MaxThetaDeg at the edge. It
// returns nil when the map is unusable or not earth-fixed.
func sectorObstructions(m *device.DishGetObstructionMapResponse) []sectorStats {
	rows, cols := int(m.GetNumRows()), int(m.GetNumCols())
	snr := m.GetSnr()
	if rows < 2 || rows != cols || len(snr) != rows*cols || m.GetMaxThetaDeg() <= 0 ||
		m.GetMapReferenceFrame() != device.ObstructionMapReferenceFrame_FRAME_EARTH {
		return nil
	}

	type band struct{ observed, obstructed int }
	type acc struct {
		total, observed, obstructed int
		lowestObserved              float64
		bands                       map[int]*band
		topObstructed               map[int]float64 // highest obstructed elevation per band
	}
	sectors := make([]*acc, len(sectorNames))
	for i := range sectors {
		sectors[i] = &acc{lowestObserved: math.Inf(1), bands: map[int]*band{}, topObstructed: map[int]float64{}}
	}

	centre := float64(rows-1) / 2
	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			dx, dy := float64(col)-centre, centre-float64(row)
			radius := math.Hypot(dx, dy)
			if radius > centre {
				continue // the corners lie outside the sky circle
			}
			elevation := 90 - radius/centre*float64(m.GetMaxThetaDeg())
			heading := math.Mod(math.Atan2(dx, dy)*180/math.Pi+360, 360)
			a := sectors[int(math.Mod(heading+11.25, 360)/22.5)]

			a.total++
			v := snr[row*cols+col]
			if v < 0 {
				continue
			}
			a.observed++
			a.lowestObserved = math.Min(a.lowestObserved, elevation)
			b := int(math.Floor(elevation / bandDegrees))
			if a.bands[b] == nil {
				a.bands[b] = &band{}
			}
			a.bands[b].observed++
			if v < obstructedBelowSnr {
				a.obstructed++
				a.bands[b].obstructed++
				a.topObstructed[b] = math.Max(a.topObstructed[b], elevation)
			}
		}
	}

	stats := make([]sectorStats, len(sectors))
	for i, a := range sectors {
		s := sectorStats{name: sectorNames[i]}
		if a.total > 0 {
			s.observed = float64(a.observed) / float64(a.total)
		}
		if a.observed > 0 {
			s.obstructed = float64(a.obstructed) / float64(a.observed)
			s.clearAbove = a.lowestObserved
			for b, bd := range a.bands {
				if float64(bd.obstructed)/float64(bd.observed) >= bandObstructedFraction {
					s.clearAbove = math.Max(s.clearAbove, a.topObstructed[b])
				}
			}
		}
		stats[i] = s
	}
	return stats
}
