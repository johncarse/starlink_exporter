package exporter

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/danopstech/starlink_exporter/pkg/spacex.com/api/device"
)

// fakeDish answers the four requests the exporter makes. A nil response with
// a non-nil error makes that request fail.
type fakeDish struct {
	status     *device.DishGetStatusResponse
	statusErr  error
	context    *device.DishGetContextResponse
	contextErr error
	history    *device.DishGetHistoryResponse
	historyErr error

	obstruction    *device.DishGetObstructionMapResponse
	obstructionErr error
}

func (f *fakeDish) Stream(ctx context.Context, opts ...grpc.CallOption) (device.Device_StreamClient, error) {
	return nil, errors.New("stream not used")
}

func (f *fakeDish) Handle(ctx context.Context, in *device.Request, opts ...grpc.CallOption) (*device.Response, error) {
	switch in.GetRequest().(type) {
	case *device.Request_GetStatus:
		if f.statusErr != nil {
			return nil, f.statusErr
		}
		return &device.Response{Response: &device.Response_DishGetStatus{DishGetStatus: f.status}}, nil
	case *device.Request_DishGetContext:
		if f.contextErr != nil {
			return nil, f.contextErr
		}
		return &device.Response{Response: &device.Response_DishGetContext{DishGetContext: f.context}}, nil
	case *device.Request_GetHistory:
		if f.historyErr != nil {
			return nil, f.historyErr
		}
		return &device.Response{Response: &device.Response_DishGetHistory{DishGetHistory: f.history}}, nil
	case *device.Request_DishGetObstructionMap:
		if f.obstructionErr != nil {
			return nil, f.obstructionErr
		}
		return &device.Response{Response: &device.Response_DishGetObstructionMap{DishGetObstructionMap: f.obstruction}}, nil
	}
	return nil, errors.New("unexpected request")
}

// healthyDish mirrors a current-firmware dish: get_context is denied and
// get_status carries everything else.
func healthyDish() *fakeDish {
	return &fakeDish{
		contextErr: status.Error(codes.PermissionDenied, "Permission denied"),
		status: &device.DishGetStatusResponse{
			DeviceInfo: &device.DeviceInfo{
				Id:              "ut00000000-00000000-00000000",
				HardwareVersion: "rev3_proto2",
				SoftwareVersion: "2026.09.29.mr87948.1",
				CountryCode:     "US",
				UtcOffsetS:      -21600,
				Bootcount:       338,
			},
			DeviceState:           &device.DeviceState{UptimeS: 318818},
			EthSpeedMbps:          1000,
			IsSnrAboveNoiseFloor:  true,
			DownlinkThroughputBps: 6101328,
			UplinkThroughputBps:   2529844,
			PopPingLatencyMs:      23,
			ReadyStates:           &device.DishReadyStates{Cady: true, Scp: true, L1L2: true, Xphy: true, Aap: true, Rf: true},
			Alerts:                &device.DishAlerts{InstallPending: true},
			SoftwareUpdateState:   device.SoftwareUpdateState_REBOOT_REQUIRED,
			SoftwareUpdateStats:   &device.SoftwareUpdateStats{RebootScheduledUtcTime: 1793178934},
		},
		history: &device.DishGetHistoryResponse{
			Current: 3,
			PowerIn: []float32{30, 40, 50, 60},
		},
	}
}

// gather runs one scrape and returns the metric families by name.
func gather(t *testing.T, dish *fakeDish) map[string]*dto.MetricFamily {
	t.Helper()
	reg := prometheus.NewRegistry()
	reg.MustRegister(&Exporter{Client: dish})
	families, err := reg.Gather()
	if err != nil {
		t.Fatalf("gather failed: %v", err)
	}
	byName := map[string]*dto.MetricFamily{}
	for _, f := range families {
		byName[f.GetName()] = f
	}
	return byName
}

// value returns the single unlabelled gauge value of a metric, failing the
// test if the metric is absent.
func value(t *testing.T, families map[string]*dto.MetricFamily, name string) float64 {
	t.Helper()
	f, ok := families[name]
	if !ok {
		t.Fatalf("metric %s not exported", name)
	}
	return f.GetMetric()[0].GetGauge().GetValue()
}

// labelled returns the gauge value of the series whose label matches.
func labelled(t *testing.T, families map[string]*dto.MetricFamily, name, label, want string) float64 {
	t.Helper()
	f, ok := families[name]
	if !ok {
		t.Fatalf("metric %s not exported", name)
	}
	for _, m := range f.GetMetric() {
		for _, l := range m.GetLabel() {
			if l.GetName() == label && l.GetValue() == want {
				return m.GetGauge().GetValue()
			}
		}
	}
	t.Fatalf("metric %s has no series with %s=%q", name, label, want)
	return 0
}

func TestScrapeExportsDeviceInfoAndUptimeWhenContextIsDenied(t *testing.T) {
	families := gather(t, healthyDish())

	info := families["starlink_dish_info"].GetMetric()[0]
	got := map[string]string{}
	for _, l := range info.GetLabel() {
		got[l.GetName()] = l.GetValue()
	}
	if got["device_id"] != "ut00000000-00000000-00000000" || got["hardware_version"] != "rev3_proto2" ||
		got["software_version"] != "2026.09.29.mr87948.1" || got["country_code"] != "US" {
		t.Errorf("starlink_dish_info labels = %v", got)
	}
	if v := value(t, families, "starlink_dish_uptime_seconds"); v != 318818 {
		t.Errorf("uptime_seconds = %v, want 318818", v)
	}
	if v := value(t, families, "starlink_dish_boot_count"); v != 338 {
		t.Errorf("boot_count = %v, want 338", v)
	}
	if v := value(t, families, "starlink_dish_up"); v != 1 {
		t.Errorf("dish_up = %v, want 1", v)
	}
}

func TestScrapeOmitsContextOnlyMetricsWhenContextIsDenied(t *testing.T) {
	families := gather(t, healthyDish())

	for _, name := range []string{"starlink_dish_cell_id", "starlink_dish_pop_rack_id", "starlink_dish_backup_beam"} {
		if _, ok := families[name]; ok {
			t.Errorf("%s exported although dish_get_context was denied", name)
		}
	}
}

func TestScrapeExportsContextMetricsWhenContextIsAllowed(t *testing.T) {
	dish := healthyDish()
	dish.contextErr = nil
	dish.context = &device.DishGetContextResponse{CellId: 7, PopRackId: 9}

	families := gather(t, dish)

	if v := value(t, families, "starlink_dish_cell_id"); v != 7 {
		t.Errorf("cell_id = %v, want 7", v)
	}
	if v := value(t, families, "starlink_dish_pop_rack_id"); v != 9 {
		t.Errorf("pop_rack_id = %v, want 9", v)
	}
}

func TestScrapeReportsDishDownWhenContextFailsForAnotherReason(t *testing.T) {
	dish := healthyDish()
	dish.contextErr = status.Error(codes.Unavailable, "connection refused")

	families := gather(t, dish)

	if v := value(t, families, "starlink_dish_up"); v != 0 {
		t.Errorf("dish_up = %v, want 0", v)
	}
}

func TestScrapeReportsDishDownAndNoStatusMetricsWhenStatusFails(t *testing.T) {
	dish := healthyDish()
	dish.statusErr = status.Error(codes.DeadlineExceeded, "timeout")

	families := gather(t, dish)

	if v := value(t, families, "starlink_dish_up"); v != 0 {
		t.Errorf("dish_up = %v, want 0", v)
	}
	if _, ok := families["starlink_dish_eth_speed_mbps"]; ok {
		t.Error("eth_speed_mbps exported although get_status failed")
	}
}

func TestThroughputIsExportedAsBitsPerSecond(t *testing.T) {
	families := gather(t, healthyDish())

	if v := value(t, families, "starlink_dish_downlink_throughput_bps"); v != 6101328 {
		t.Errorf("downlink_throughput_bps = %v, want 6101328", v)
	}
	if v := value(t, families, "starlink_dish_uplink_throughput_bps"); v != 2529844 {
		t.Errorf("uplink_throughput_bps = %v, want 2529844", v)
	}
	if _, ok := families["starlink_dish_downlink_throughput_bytes"]; ok {
		t.Error("the mislabelled _bytes metric is still exported")
	}
}

func TestStateIsConnectedWhenDishReportsNoOutage(t *testing.T) {
	families := gather(t, healthyDish())

	if v := labelled(t, families, "starlink_dish_state", "state", "CONNECTED"); v != 1 {
		t.Errorf("CONNECTED = %v, want 1", v)
	}
	if v := labelled(t, families, "starlink_dish_state", "state", "BOOTING"); v != 0 {
		t.Errorf("BOOTING = %v, want 0", v)
	}
}

func TestStateIsSearchingWhenOutageCauseIsNoSchedule(t *testing.T) {
	dish := healthyDish()
	dish.status.Outage = &device.DishOutage{Cause: device.DishOutage_NO_SCHEDULE}

	families := gather(t, dish)

	if v := labelled(t, families, "starlink_dish_state", "state", "SEARCHING"); v != 1 {
		t.Errorf("SEARCHING = %v, want 1", v)
	}
	if v := labelled(t, families, "starlink_dish_state", "state", "CONNECTED"); v != 0 {
		t.Errorf("CONNECTED = %v, want 0", v)
	}
}

func TestStateIsNamedAfterTheCauseWhenOutageIsReported(t *testing.T) {
	dish := healthyDish()
	dish.status.Outage = &device.DishOutage{Cause: device.DishOutage_BOOTING}

	families := gather(t, dish)

	if v := labelled(t, families, "starlink_dish_state", "state", "BOOTING"); v != 1 {
		t.Errorf("BOOTING = %v, want 1", v)
	}
}

func TestStateMarksExactlyOneSeriesActive(t *testing.T) {
	families := gather(t, healthyDish())

	active := 0.0
	for _, m := range families["starlink_dish_state"].GetMetric() {
		active += m.GetGauge().GetValue()
	}
	if active != 1 {
		t.Errorf("%v state series are active, want exactly 1", active)
	}
}

func TestSoftwareUpdateStateMarksOnlyTheActiveState(t *testing.T) {
	families := gather(t, healthyDish())

	if v := labelled(t, families, "starlink_dish_software_update_state", "state", "REBOOT_REQUIRED"); v != 1 {
		t.Errorf("REBOOT_REQUIRED = %v, want 1", v)
	}
	if v := labelled(t, families, "starlink_dish_software_update_state", "state", "IDLE"); v != 0 {
		t.Errorf("IDLE = %v, want 0", v)
	}
	if v := value(t, families, "starlink_dish_software_update_reboot_scheduled_timestamp_seconds"); v != 1793178934 {
		t.Errorf("reboot_scheduled_timestamp_seconds = %v, want 1793178934", v)
	}
	if v := value(t, families, "starlink_dish_alert_install_pending"); v != 1 {
		t.Errorf("alert_install_pending = %v, want 1", v)
	}
}

func TestEthernetSpeedAndReadyStatesAreExported(t *testing.T) {
	dish := healthyDish()
	dish.status.ReadyStates.Rf = false

	families := gather(t, dish)

	if v := value(t, families, "starlink_dish_eth_speed_mbps"); v != 1000 {
		t.Errorf("eth_speed_mbps = %v, want 1000", v)
	}
	if v := labelled(t, families, "starlink_dish_ready_state", "subsystem", "rf"); v != 0 {
		t.Errorf("ready_state rf = %v, want 0", v)
	}
	if v := labelled(t, families, "starlink_dish_ready_state", "subsystem", "cady"); v != 1 {
		t.Errorf("ready_state cady = %v, want 1", v)
	}
}

func TestPowerReadsTheNewestSampleWhenTheHistoryRingHasWrapped(t *testing.T) {
	dish := healthyDish()
	// 4 samples, 3 taken so far this lap: the newest is index 2 (value 50).
	dish.history = &device.DishGetHistoryResponse{Current: 3, PowerIn: []float32{30, 40, 50, 60}}

	families := gather(t, dish)

	if v := value(t, families, "starlink_dish_power_input_watts"); v != 50 {
		t.Errorf("power_input_watts = %v, want 50", v)
	}
	if v := value(t, families, "starlink_dish_power_input_mean_watts"); v != 45 {
		t.Errorf("power_input_mean_watts = %v, want 45", v)
	}
}

func TestPowerReadsTheLastSlotWhenCurrentIsAMultipleOfTheRingLength(t *testing.T) {
	dish := healthyDish()
	dish.history = &device.DishGetHistoryResponse{Current: 8, PowerIn: []float32{30, 40, 50, 60}}

	families := gather(t, dish)

	if v := value(t, families, "starlink_dish_power_input_watts"); v != 60 {
		t.Errorf("power_input_watts = %v, want 60", v)
	}
}

func TestScrapeStaysUpAndSkipsPowerWhenHistoryFails(t *testing.T) {
	dish := healthyDish()
	dish.historyErr = status.Error(codes.DeadlineExceeded, "timeout")

	families := gather(t, dish)

	if v := value(t, families, "starlink_dish_up"); v != 1 {
		t.Errorf("dish_up = %v, want 1", v)
	}
	if _, ok := families["starlink_dish_power_input_watts"]; ok {
		t.Error("power_input_watts exported although get_history failed")
	}
}

func TestEveryDescribedMetricHasUniqueName(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	// Registering runs Describe and rejects duplicate or inconsistent descriptors.
	if err := reg.Register(&Exporter{Client: healthyDish()}); err != nil {
		t.Fatalf("register failed: %v", err)
	}
	if _, err := reg.Gather(); err != nil {
		t.Fatalf("pedantic gather failed: %v", err)
	}
}

// mapSize is odd so the zenith falls on a cell; each cell then spans
// 80/(mapSize-1)*2 = about 1.3 degrees of elevation.
const mapSize = 123

// skyMap builds an earth-fixed obstruction map where fill decides each cell's
// value from its compass heading and elevation.
func skyMap(fill func(heading, elevation float64) float32) *device.DishGetObstructionMapResponse {
	centre := float64(mapSize-1) / 2
	snr := make([]float32, mapSize*mapSize)
	for row := 0; row < mapSize; row++ {
		for col := 0; col < mapSize; col++ {
			dx, dy := float64(col)-centre, centre-float64(row)
			radius := math.Hypot(dx, dy)
			if radius > centre {
				snr[row*mapSize+col] = -1
				continue
			}
			heading := math.Mod(math.Atan2(dx, dy)*180/math.Pi+360, 360)
			snr[row*mapSize+col] = fill(heading, 90-radius/centre*80)
		}
	}
	return &device.DishGetObstructionMapResponse{
		NumRows:           mapSize,
		NumCols:           mapSize,
		Snr:               snr,
		MaxThetaDeg:       80,
		MapReferenceFrame: device.ObstructionMapReferenceFrame_FRAME_EARTH,
	}
}

func inSector(heading, centre float64) bool {
	d := math.Abs(math.Mod(heading-centre+540, 360) - 180)
	return d <= 11.25
}

func scrapeSky(t *testing.T, m *device.DishGetObstructionMapResponse) map[string]*dto.MetricFamily {
	t.Helper()
	dish := healthyDish()
	dish.obstruction = m
	return gather(t, dish)
}

func TestObstructionSectorFractionIsReportedOnlyForTheObstructedSector(t *testing.T) {
	families := scrapeSky(t, skyMap(func(heading, elevation float64) float32 {
		if inSector(heading, 315) && elevation < 30 {
			return 0.1
		}
		return 1
	}))

	if v := labelled(t, families, "starlink_dish_obstruction_sector_fraction", "sector", "NW"); v < 0.3 {
		t.Errorf("NW fraction = %v, want a substantial share obstructed", v)
	}
	for _, sector := range []string{"N", "NNW", "W", "S", "SE"} {
		if v := labelled(t, families, "starlink_dish_obstruction_sector_fraction", "sector", sector); v != 0 {
			t.Errorf("%s fraction = %v, want 0", sector, v)
		}
	}
}

func TestObstructionMapIsReadWithNorthUpAndEastRight(t *testing.T) {
	centre := (mapSize - 1) / 2
	cases := map[string][2]int{ // sector -> row, col of a cell on that axis
		"N": {centre - 40, centre},
		"E": {centre, centre + 40},
		"S": {centre + 40, centre},
		"W": {centre, centre - 40},
	}
	for sector, cell := range cases {
		m := skyMap(func(float64, float64) float32 { return 1 })
		// Blank out a block so the sector clears the band threshold.
		for dr := -6; dr <= 6; dr++ {
			for dc := -6; dc <= 6; dc++ {
				m.Snr[(cell[0]+dr)*mapSize+cell[1]+dc] = 0
			}
		}
		families := scrapeSky(t, m)

		for _, other := range []string{"N", "E", "S", "W"} {
			got := labelled(t, families, "starlink_dish_obstruction_sector_fraction", "sector", other)
			if other == sector && got == 0 {
				t.Errorf("block on the %s axis not reported in sector %s", sector, other)
			}
			if other != sector && got != 0 {
				t.Errorf("block on the %s axis leaked into sector %s (%v)", sector, other, got)
			}
		}
	}
}

func TestClearAboveElevationIsTheTopOfTheSkylineWhenSectorIsObstructedBelowIt(t *testing.T) {
	families := scrapeSky(t, skyMap(func(heading, elevation float64) float32 {
		if inSector(heading, 337.5) && elevation < 30 {
			return 0
		}
		return 1
	}))

	got := labelled(t, families, "starlink_dish_obstruction_sector_clear_above_elevation_deg", "sector", "NNW")
	if got < 28 || got > 30 {
		t.Errorf("NNW clear above = %v, want just under 30", got)
	}
}

func TestClearAboveElevationIsTheLowestObservedWhenNothingIsObstructed(t *testing.T) {
	families := scrapeSky(t, skyMap(func(heading, elevation float64) float32 {
		if elevation < 25 {
			return -1 // never observed below 25 degrees
		}
		return 1
	}))

	got := labelled(t, families, "starlink_dish_obstruction_sector_clear_above_elevation_deg", "sector", "E")
	if got < 25 || got > 27 {
		t.Errorf("E clear above = %v, want the lowest observed elevation, about 25", got)
	}
}

func TestSkylineIgnoresAnIsolatedObstructedCellWhenItsBandIsOtherwiseClear(t *testing.T) {
	m := skyMap(func(float64, float64) float32 { return 1 })
	centre := (mapSize - 1) / 2
	m.Snr[(centre-10)*mapSize+centre] = 0 // one bad cell high in the north

	families := scrapeSky(t, m)

	if got := labelled(t, families, "starlink_dish_obstruction_sector_clear_above_elevation_deg", "sector", "N"); got > 15 {
		t.Errorf("N clear above = %v, want the lowest observed elevation, not the noisy cell", got)
	}
	if got := labelled(t, families, "starlink_dish_obstruction_sector_fraction", "sector", "N"); got == 0 {
		t.Error("N fraction = 0, want the noisy cell still counted")
	}
}

func TestUnobservedSectorReportsCoverageOnlyWhenTheMapHasNoDataThere(t *testing.T) {
	families := scrapeSky(t, skyMap(func(heading, elevation float64) float32 {
		if inSector(heading, 180) {
			return -1
		}
		return 1
	}))

	if got := labelled(t, families, "starlink_dish_obstruction_sector_observed_fraction", "sector", "S"); got != 0 {
		t.Errorf("S observed = %v, want 0", got)
	}
	for _, name := range []string{"starlink_dish_obstruction_sector_fraction", "starlink_dish_obstruction_sector_clear_above_elevation_deg"} {
		for _, m := range families[name].GetMetric() {
			for _, l := range m.GetLabel() {
				if l.GetValue() == "S" {
					t.Errorf("%s exported for S although it has no data", name)
				}
			}
		}
	}
}

func TestEverySectorIsExportedWhenTheMapIsFullyObserved(t *testing.T) {
	families := scrapeSky(t, skyMap(func(float64, float64) float32 { return 1 }))

	for _, name := range []string{
		"starlink_dish_obstruction_sector_fraction",
		"starlink_dish_obstruction_sector_clear_above_elevation_deg",
		"starlink_dish_obstruction_sector_observed_fraction",
	} {
		if got := len(families[name].GetMetric()); got != 16 {
			t.Errorf("%s has %d series, want 16", name, got)
		}
	}
}

func TestObstructionMetricsAreSkippedWhenTheMapIsNotEarthFixed(t *testing.T) {
	m := skyMap(func(float64, float64) float32 { return 1 })
	m.MapReferenceFrame = device.ObstructionMapReferenceFrame_FRAME_UT

	families := scrapeSky(t, m)

	if _, ok := families["starlink_dish_obstruction_sector_fraction"]; ok {
		t.Error("sector metrics exported for a dish-relative map")
	}
}

func TestObstructionMetricsAreSkippedWhenTheMapIsMalformed(t *testing.T) {
	m := skyMap(func(float64, float64) float32 { return 1 })
	m.Snr = m.Snr[:100]

	families := scrapeSky(t, m)

	if _, ok := families["starlink_dish_obstruction_sector_fraction"]; ok {
		t.Error("sector metrics exported for a map whose size does not match its cells")
	}
}

func TestScrapeStaysUpAndSkipsObstructionMapWhenRequestFails(t *testing.T) {
	dish := healthyDish()
	dish.obstructionErr = status.Error(codes.DeadlineExceeded, "timeout")

	families := gather(t, dish)

	if v := value(t, families, "starlink_dish_up"); v != 1 {
		t.Errorf("dish_up = %v, want 1", v)
	}
	if _, ok := families["starlink_dish_obstruction_sector_fraction"]; ok {
		t.Error("sector metrics exported although the obstruction map request failed")
	}
}
