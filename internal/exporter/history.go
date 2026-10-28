package exporter

import (
	"math"
	"sort"
	"sync"

	"github.com/danopstech/starlink_exporter/pkg/spacex.com/api/device"
)

// recorded returns the samples the dish has actually taken. The history is a
// ring buffer and current counts every sample ever taken, so right after a
// boot only the first current slots hold data and the rest are zero.
func recorded(samples []float32, current uint64) []float32 {
	if current < uint64(len(samples)) {
		return samples[:current]
	}
	return samples
}

// newest returns the most recent sample, which sits just behind current modulo
// the buffer length.
func newest(samples []float32, current uint64) (float32, bool) {
	if len(samples) == 0 || current == 0 {
		return 0, false
	}
	return samples[(current-1)%uint64(len(samples))], true
}

// summary is the mean, 95th percentile (nearest rank) and maximum of a window.
type summary struct{ mean, p95, max float64 }

// summarise ignores NaN and infinite samples and reports false when none are
// left.
func summarise(samples []float64) (summary, bool) {
	v := make([]float64, 0, len(samples))
	var sum float64
	for _, s := range samples {
		if math.IsNaN(s) || math.IsInf(s, 0) {
			continue
		}
		v = append(v, s)
		sum += s
	}
	if len(v) == 0 {
		return summary{}, false
	}
	sort.Float64s(v)
	rank := int(math.Ceil(0.95*float64(len(v)))) - 1
	return summary{mean: sum / float64(len(v)), p95: v[rank], max: v[len(v)-1]}, true
}

func float64s(samples []float32) []float64 {
	out := make([]float64, len(samples))
	for i, s := range samples {
		out[i] = float64(s)
	}
	return out
}

// latencyWhenAnswered drops the latency of samples whose pings were all lost,
// where there is no round trip to measure.
func latencyWhenAnswered(latency, drop []float32) []float64 {
	out := make([]float64, 0, len(latency))
	for i, l := range latency {
		if i < len(drop) && drop[i] >= 1 {
			continue
		}
		out = append(out, float64(l))
	}
	return out
}

// outageKey identifies one outage: the dish keeps a bounded list of recent
// outages and each keeps its start time.
type outageKey struct {
	cause string
	start int64
}

// outageTracker turns the dish's list of recent outages into monotonic
// counters. The list is bounded, so the counters are built from what this
// process has seen: the first observation only records the outages already
// present, so a restart does not recount old ones. The zero value is ready to
// use.
type outageTracker struct {
	mu       sync.Mutex
	seeded   bool
	duration map[outageKey]uint64 // last duration seen per outage, in ns
	count    map[string]float64
	seconds  map[string]float64
}

func outageCauseName(c device.DishOutage_Cause) string {
	if name, ok := device.DishOutage_Cause_name[int32(c)]; ok {
		return name
	}
	return "UNKNOWN"
}

// observe folds the current outage list into the counters. An outage still in
// progress grows between observations, so its duration is added as a delta.
func (o *outageTracker) observe(outages []*device.DishOutage) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.duration == nil {
		o.duration, o.count, o.seconds = map[outageKey]uint64{}, map[string]float64{}, map[string]float64{}
	}

	current := make(map[outageKey]uint64, len(outages))
	for _, out := range outages {
		key := outageKey{outageCauseName(out.GetCause()), out.GetStartTimestampNs()}
		current[key] = out.GetDurationNs()
		last, known := o.duration[key]
		if !o.seeded {
			continue
		}
		if !known {
			o.count[key.cause]++
		}
		if d := out.GetDurationNs(); d > last {
			o.seconds[key.cause] += float64(d-last) / 1e9
		}
	}
	// Forget outages that have aged out of the dish's list.
	o.duration = current
	o.seeded = true
}

// totals returns copies of the per-cause counters.
func (o *outageTracker) totals() (count, seconds map[string]float64) {
	o.mu.Lock()
	defer o.mu.Unlock()
	count, seconds = map[string]float64{}, map[string]float64{}
	for k, v := range o.count {
		count[k] = v
	}
	for k, v := range o.seconds {
		seconds[k] = v
	}
	return count, seconds
}
