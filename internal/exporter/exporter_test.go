package exporter

import (
	"context"
	"errors"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/danopstech/starlink_exporter/pkg/spacex.com/api/device"
)

// fakeDish answers the three requests the exporter makes. A nil response with
// a non-nil error makes that request fail.
type fakeDish struct {
	status     *device.DishGetStatusResponse
	statusErr  error
	context    *device.DishGetContextResponse
	contextErr error
	history    *device.DishGetHistoryResponse
	historyErr error
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
