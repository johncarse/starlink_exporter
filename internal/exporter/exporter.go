package exporter

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/danopstech/starlink_exporter/pkg/spacex.com/api/device"
)

const (
	// DishAddress to reach Starlink dish ip:port
	DishAddress = "192.168.100.1:9200"
	namespace   = "starlink"
)

var (
	dishUp = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "up"),
		"Was the last query of Starlink dish successful.",
		nil, nil,
	)
	dishScrapeDurationSeconds = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "scrape_duration_seconds"),
		"Time to scrape metrics from starlink dish",
		nil, nil,
	)

	// collectDishContext
	dishInfo = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "info"),
		"Running software versions and IDs of hardware",
		[]string{"device_id", "hardware_version", "software_version", "country_code", "utc_offset"}, nil,
	)
	dishUptimeSeconds = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "uptime_seconds"),
		"Dish running time",
		nil, nil,
	)
	dishCellId = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "cell_id"),
		"Cell ID dish is located in",
		nil, nil,
	)
	dishPopRackId = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "pop_rack_id"),
		"pop rack id",
		nil, nil,
	)
	dishInitialSatelliteId = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "initial_satellite_id"),
		"initial satellite id",
		nil, nil,
	)
	dishInitialGatewayId = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "initial_gateway_id"),
		"initial gateway id",
		nil, nil,
	)
	dishOnBackupBeam = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "backup_beam"),
		"connected to backup beam",
		nil, nil,
	)
	dishSecondsToSlotEnd = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "time_to_slot_end_seconds"),
		"Seconds left on current slot",
		nil, nil,
	)

	// collectDishStatus
	dishSecondsToFirstNonemptySlot = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "first_nonempty_slot_seconds"),
		"Seconds to next non empty slot",
		nil, nil,
	)
	dishPopPingDropRatio = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "pop_ping_drop_ratio"),
		"Percent of pings dropped",
		nil, nil,
	)
	dishPopPingLatencySeconds = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "pop_ping_latency_seconds"),
		"Latency of connection in seconds",
		nil, nil,
	)
	dishUplinkThroughputBps = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "uplink_throughput_bps"),
		"Amount of bandwidth in bits per second upload",
		nil, nil,
	)
	dishDownlinkThroughputBps = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "downlink_throughput_bps"),
		"Amount of bandwidth in bits per second download",
		nil, nil,
	)

	dishBoreSightAzimuthDeg = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "bore_sight_azimuth_deg"),
		"azimuth in degrees",
		nil, nil,
	)

	dishBoreSightElevationDeg = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "bore_sight_elevation_deg"),
		"elevation in degrees",
		nil, nil,
	)

	// collectDishObstructions
	dishCurrentlyObstructed = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "currently_obstructed"),
		"Status of view of the sky",
		nil, nil,
	)
	dishFractionObstructionRatio = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "fraction_obstruction_ratio"),
		"Percentage of obstruction",
		nil, nil,
	)
	dishValidSeconds = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "valid_seconds"),
		"Unknown",
		nil, nil,
	)
	dishProlongedObstructionDurationSeconds = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "prolonged_obstruction_duration_seconds"),
		"Average in seconds of prolonged obstructions",
		nil, nil,
	)
	dishProlongedObstructionIntervalSeconds = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "prolonged_obstruction_interval_seconds"),
		"Average prolonged obstruction interval in seconds",
		nil, nil,
	)

	// collectDishHealth
	dishBootCount = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "boot_count"),
		"Number of times the dish has booted",
		nil, nil,
	)
	dishEthSpeedMbps = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "eth_speed_mbps"),
		"Negotiated speed of the dish ethernet link in Mbps",
		nil, nil,
	)
	dishSnrAboveNoiseFloor = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "snr_above_noise_floor"),
		"Whether the signal to noise ratio is above the noise floor",
		nil, nil,
	)
	dishSnrPersistentlyLow = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "snr_persistently_low"),
		"Whether the signal to noise ratio is persistently low",
		nil, nil,
	)
	dishReadyState = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "ready_state"),
		"Whether a dish subsystem reports ready",
		[]string{"subsystem"}, nil,
	)
	dishGpsValid = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "gps_valid"),
		"Whether the dish has a valid GPS fix",
		nil, nil,
	)
	dishGpsSatellites = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "gps_satellites"),
		"Number of GPS satellites the dish is using",
		nil, nil,
	)
	dishTiltAngleDeg = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "tilt_angle_deg"),
		"Tilt of the dish from vertical in degrees",
		nil, nil,
	)
	dishDesiredBoreSightAzimuthDeg = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "desired_bore_sight_azimuth_deg"),
		"Azimuth the dish wants to point at in degrees",
		nil, nil,
	)
	dishDesiredBoreSightElevationDeg = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "desired_bore_sight_elevation_deg"),
		"Elevation the dish wants to point at in degrees",
		nil, nil,
	)

	dishState = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "state"),
		"Derived dish connectivity state, 1 for the active state (CONNECTED when the dish reports no outage)",
		[]string{"state"}, nil,
	)

	// collectDishPower
	dishPowerInputWatts = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "power_input_watts"),
		"Most recent power draw of the dish in watts",
		nil, nil,
	)
	dishPowerInputMeanWatts = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "power_input_mean_watts"),
		"Mean power draw of the dish in watts over the dish's history window (about 15 minutes)",
		nil, nil,
	)

	// collectDishSoftwareUpdate
	dishSoftwareUpdateState = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "software_update_state"),
		"Current software update state, 1 for the active state",
		[]string{"state"}, nil,
	)
	dishSoftwareUpdateProgress = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "software_update_progress"),
		"Software update progress, 0 to 1",
		nil, nil,
	)
	dishSoftwareUpdateRebootReady = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "software_update_reboot_ready"),
		"Whether the dish is ready to reboot to apply a software update",
		nil, nil,
	)
	dishSoftwareUpdateRebootScheduled = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "software_update_reboot_scheduled_timestamp_seconds"),
		"Unix time of the scheduled software update reboot, 0 when none is scheduled",
		nil, nil,
	)
	dishSecondsUntilSoftwareUpdateRebootPossible = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "seconds_until_software_update_reboot_possible"),
		"Seconds until the dish may reboot for a software update, -1 when not applicable",
		nil, nil,
	)

	// collectDishAlerts
	dishAlertMotorsStuck = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "alert_motors_stuck"),
		"Status of motor stuck",
		nil, nil,
	)
	dishAlertThermalThrottle = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "alert_thermal_throttle"),
		"Status of thermal throttling",
		nil, nil,
	)
	dishAlertThermalShutdown = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "alert_thermal_shutdown"),
		"Status of thermal shutdown",
		nil, nil,
	)
	dishAlertMastNotNearVertical = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "alert_mast_not_near_vertical"),
		"Status of mast position",
		nil, nil,
	)
	dishUnexpectedLocation = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "alert_unexpected_location"),
		"Status of location",
		nil, nil,
	)
	dishSlowEthernetSpeeds = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "alert_slow_eth_speeds"),
		"Status of ethernet",
		nil, nil,
	)
	dishAlertInstallPending = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "alert_install_pending"),
		"Whether a software update is pending installation",
		nil, nil,
	)
	dishAlertIsHeating = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "alert_is_heating"),
		"Whether the dish is heating (snow melt)",
		nil, nil,
	)
	dishAlertPowerSupplyThermalThrottle = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "alert_power_supply_thermal_throttle"),
		"Status of power supply thermal throttling",
		nil, nil,
	)
	dishAlertIsPowerSaveIdle = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "alert_is_power_save_idle"),
		"Whether the dish is idle in power save mode",
		nil, nil,
	)
	dishAlertLowMotorCurrent = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "alert_low_motor_current"),
		"Status of motor current",
		nil, nil,
	)
	dishAlertLowerSignalThanPredicted = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "alert_signal_lower_than_predicted"),
		"Whether the signal is lower than predicted",
		nil, nil,
	)
	dishAlertSlowEthernetSpeeds100 = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "alert_slow_eth_speeds_100"),
		"Status of ethernet negotiating 100 Mbps",
		nil, nil,
	)
	dishAlertNoEthernetLink = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "alert_no_ethernet_link"),
		"Whether the dish has no ethernet link",
		nil, nil,
	)
	dishAlertUpsuRouterPortSlow = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "alert_upsu_router_port_slow"),
		"Status of the power supply router port speed",
		nil, nil,
	)
	dishAlertDishWaterDetected = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "alert_dish_water_detected"),
		"Whether water is detected in the dish",
		nil, nil,
	)
	dishAlertRouterWaterDetected = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "alert_router_water_detected"),
		"Whether water is detected in the router",
		nil, nil,
	)

	// collectDishHistory
	dishHistoryWindowSeconds = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "history_window_seconds"),
		"Length of the dish's 1 Hz history window the history_* metrics summarise (up to 900, less just after a reboot)",
		nil, nil,
	)
	dishHistoryDownlinkThroughputBps = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "history_downlink_throughput_bps"),
		"Downlink throughput over the dish's history window, by statistic (mean, p95, max); real traffic, so a floor on capacity, not a speedtest",
		[]string{"stat"}, nil,
	)
	dishHistoryUplinkThroughputBps = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "history_uplink_throughput_bps"),
		"Uplink throughput over the dish's history window, by statistic (mean, p95, max); real traffic, so a floor on capacity, not a speedtest",
		[]string{"stat"}, nil,
	)
	dishHistoryPopPingLatencySeconds = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "history_pop_ping_latency_seconds"),
		"Latency to the Starlink PoP over the dish's history window, by statistic (mean, p95, max), ignoring samples where every ping was lost",
		[]string{"stat"}, nil,
	)
	dishHistoryPopPingDropRatio = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "history_pop_ping_drop_ratio"),
		"Ping drop ratio over the dish's history window, by statistic (mean, p95, max)",
		[]string{"stat"}, nil,
	)
	dishOutagesTotal = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "outages_total"),
		"Outages the dish reported since this exporter started, by cause",
		[]string{"cause"}, nil,
	)
	dishOutageSecondsTotal = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "outage_seconds_total"),
		"Time spent in outage since this exporter started, by cause",
		[]string{"cause"}, nil,
	)

	// collectDishObstructionMap
	dishObstructionSectorFraction = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "obstruction_sector_fraction"),
		"Share of the observed sky in a compass sector that is obstructed, from the dish obstruction map",
		[]string{"sector"}, nil,
	)
	dishObstructionSectorClearAboveDeg = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "obstruction_sector_clear_above_elevation_deg"),
		"Elevation from which a compass sector is clear all the way up: the top of the skyline, or the lowest observed elevation when nothing is obstructed",
		[]string{"sector"}, nil,
	)
	dishObstructionSectorObservedFraction = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "dish", "obstruction_sector_observed_fraction"),
		"Share of a compass sector of the sky that the obstruction map has data for (it empties when the dish reboots)",
		[]string{"sector"}, nil,
	)
)

// Exporter collects Starlink stats from the Dish and exports them using
// the prometheus metrics package.
type Exporter struct {
	Conn        *grpc.ClientConn
	Client      device.DeviceClient
	DishID      string
	CountryCode string

	outages outageTracker
}

// New returns an initialized Exporter.
func New(address string) (*Exporter, error) {
	ctx, connCancel := context.WithTimeout(context.Background(), time.Second*3)
	defer connCancel()
	conn, err := grpc.DialContext(ctx, address, grpc.WithInsecure(), grpc.WithBlock())
	if err != nil {
		return nil, fmt.Errorf("error creating underlying gRPC connection to starlink dish: %s", err.Error())
	}

	ctx, HandleCancel := context.WithTimeout(context.Background(), time.Second*1)
	defer HandleCancel()
	resp, err := device.NewDeviceClient(conn).Handle(ctx, &device.Request{
		Request: &device.Request_GetDeviceInfo{},
	})
	if err != nil {
		return nil, fmt.Errorf("could not collect inital information from dish: %s", err.Error())
	}

	return &Exporter{
		Conn:        conn,
		Client:      device.NewDeviceClient(conn),
		DishID:      resp.GetGetDeviceInfo().GetDeviceInfo().GetId(),
		CountryCode: resp.GetGetDeviceInfo().GetDeviceInfo().GetCountryCode(),
	}, nil
}

// Describe describes all the metrics ever exported by the Starlink exporter. It
// implements prometheus.Collector.
func (e *Exporter) Describe(ch chan<- *prometheus.Desc) {
	ch <- dishUp
	ch <- dishScrapeDurationSeconds

	// collectDishContext
	ch <- dishInfo
	ch <- dishUptimeSeconds
	ch <- dishCellId
	ch <- dishPopRackId
	ch <- dishInitialSatelliteId
	ch <- dishInitialGatewayId
	ch <- dishOnBackupBeam
	ch <- dishSecondsToSlotEnd

	// collectDishStatus
	ch <- dishSecondsToFirstNonemptySlot
	ch <- dishPopPingDropRatio
	ch <- dishPopPingLatencySeconds
	ch <- dishUplinkThroughputBps
	ch <- dishDownlinkThroughputBps
	ch <- dishBoreSightAzimuthDeg
	ch <- dishBoreSightElevationDeg

	// collectDishObstructions
	ch <- dishCurrentlyObstructed
	ch <- dishFractionObstructionRatio
	ch <- dishValidSeconds
	ch <- dishProlongedObstructionDurationSeconds
	ch <- dishProlongedObstructionIntervalSeconds

	// collectDishHealth
	ch <- dishBootCount
	ch <- dishEthSpeedMbps
	ch <- dishSnrAboveNoiseFloor
	ch <- dishSnrPersistentlyLow
	ch <- dishReadyState
	ch <- dishGpsValid
	ch <- dishGpsSatellites
	ch <- dishTiltAngleDeg
	ch <- dishDesiredBoreSightAzimuthDeg
	ch <- dishDesiredBoreSightElevationDeg

	ch <- dishState

	// collectDishPower
	ch <- dishPowerInputWatts
	ch <- dishPowerInputMeanWatts

	// collectDishSoftwareUpdate
	ch <- dishSoftwareUpdateState
	ch <- dishSoftwareUpdateProgress
	ch <- dishSoftwareUpdateRebootReady
	ch <- dishSoftwareUpdateRebootScheduled
	ch <- dishSecondsUntilSoftwareUpdateRebootPossible

	// collectDishHistory
	ch <- dishHistoryWindowSeconds
	ch <- dishHistoryDownlinkThroughputBps
	ch <- dishHistoryUplinkThroughputBps
	ch <- dishHistoryPopPingLatencySeconds
	ch <- dishHistoryPopPingDropRatio
	ch <- dishOutagesTotal
	ch <- dishOutageSecondsTotal

	// collectDishObstructionMap
	ch <- dishObstructionSectorFraction
	ch <- dishObstructionSectorClearAboveDeg
	ch <- dishObstructionSectorObservedFraction

	// collectDishAlerts
	ch <- dishAlertMotorsStuck
	ch <- dishAlertThermalThrottle
	ch <- dishAlertThermalShutdown
	ch <- dishAlertMastNotNearVertical
	ch <- dishUnexpectedLocation
	ch <- dishSlowEthernetSpeeds
	ch <- dishAlertInstallPending
	ch <- dishAlertIsHeating
	ch <- dishAlertPowerSupplyThermalThrottle
	ch <- dishAlertIsPowerSaveIdle
	ch <- dishAlertLowMotorCurrent
	ch <- dishAlertLowerSignalThanPredicted
	ch <- dishAlertSlowEthernetSpeeds100
	ch <- dishAlertNoEthernetLink
	ch <- dishAlertUpsuRouterPortSlow
	ch <- dishAlertDishWaterDetected
	ch <- dishAlertRouterWaterDetected
}

// Collect fetches the stats from Starlink dish and delivers them
// as Prometheus metrics. It implements prometheus.Collector.
func (e *Exporter) Collect(ch chan<- prometheus.Metric) {
	start := time.Now()

	ok := e.collectDishContext(ch)
	if dishStatus, statusOk := e.getDishStatus(); statusOk {
		e.collectDishStatus(ch, dishStatus)
		e.collectDishHealth(ch, dishStatus)
		e.collectDishSoftwareUpdate(ch, dishStatus)
		e.collectDishObstructions(ch, dishStatus)
		e.collectDishAlerts(ch, dishStatus)
		e.collectDishHistory(ch)
		e.collectDishObstructionMap(ch)
	} else {
		ok = false
	}

	if ok {
		ch <- prometheus.MustNewConstMetric(
			dishUp, prometheus.GaugeValue, 1.0,
		)
		ch <- prometheus.MustNewConstMetric(
			dishScrapeDurationSeconds, prometheus.GaugeValue, time.Since(start).Seconds(),
		)
	} else {
		ch <- prometheus.MustNewConstMetric(
			dishUp, prometheus.GaugeValue, 0.0,
		)
	}
}

func (e *Exporter) collectDishContext(ch chan<- prometheus.Metric) bool {
	req := &device.Request{
		Request: &device.Request_DishGetContext{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()
	resp, err := e.Client.Handle(ctx, req)
	if err != nil {
		st, ok := status.FromError(err)
		if ok && st.Code() == codes.PermissionDenied {
			// Current firmware no longer allows dish_get_context. Device info and
			// uptime are exported from get_status instead; skip the context-only
			// metrics rather than exporting zeros.
			return true
		}
		log.Errorf("failed to collect dish context: %s", err.Error())
		return false
	}

	dishC := resp.GetDishGetContext()

	ch <- prometheus.MustNewConstMetric(
		dishCellId, prometheus.GaugeValue, float64(dishC.GetCellId()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishPopRackId, prometheus.GaugeValue, float64(dishC.GetPopRackId()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishInitialSatelliteId, prometheus.GaugeValue, float64(dishC.GetInitialSatelliteId()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishInitialGatewayId, prometheus.GaugeValue, float64(dishC.GetInitialGatewayId()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishOnBackupBeam, prometheus.GaugeValue, flool(dishC.GetOnBackupBeam()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishSecondsToSlotEnd, prometheus.GaugeValue, float64(dishC.GetSecondsToSlotEnd()),
	)

	return true
}

// getDishStatus fetches get_status once per scrape; every status-derived
// collector reads from the same response.
func (e *Exporter) getDishStatus() (*device.DishGetStatusResponse, bool) {
	req := &device.Request{
		Request: &device.Request_GetStatus{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*1)
	defer cancel()
	resp, err := e.Client.Handle(ctx, req)
	if err != nil {
		log.Errorf("failed to collect status from dish: %s", err.Error())
		return nil, false
	}

	return resp.GetDishGetStatus(), true
}

func (e *Exporter) collectDishStatus(ch chan<- prometheus.Metric, dishStatus *device.DishGetStatusResponse) {
	dishI := dishStatus.GetDeviceInfo()

	ch <- prometheus.MustNewConstMetric(
		dishInfo, prometheus.GaugeValue, 1.00,
		dishI.GetId(),
		dishI.GetHardwareVersion(),
		dishI.GetSoftwareVersion(),
		dishI.GetCountryCode(),
		fmt.Sprint(dishI.GetUtcOffsetS()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishUptimeSeconds, prometheus.GaugeValue, float64(dishStatus.GetDeviceState().GetUptimeS()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishBootCount, prometheus.GaugeValue, float64(dishI.GetBootcount()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishSecondsToFirstNonemptySlot, prometheus.GaugeValue, float64(dishStatus.GetSecondsToFirstNonemptySlot()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishPopPingDropRatio, prometheus.GaugeValue, float64(dishStatus.GetPopPingDropRate()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishPopPingLatencySeconds, prometheus.GaugeValue, float64(dishStatus.GetPopPingLatencyMs()/1000),
	)

	ch <- prometheus.MustNewConstMetric(
		dishUplinkThroughputBps, prometheus.GaugeValue, float64(dishStatus.GetUplinkThroughputBps()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishDownlinkThroughputBps, prometheus.GaugeValue, float64(dishStatus.GetDownlinkThroughputBps()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishBoreSightAzimuthDeg, prometheus.GaugeValue, float64(dishStatus.GetBoresightAzimuthDeg()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishBoreSightElevationDeg, prometheus.GaugeValue, float64(dishStatus.GetBoresightElevationDeg()),
	)
}

func (e *Exporter) collectDishHealth(ch chan<- prometheus.Metric, dishStatus *device.DishGetStatusResponse) {
	ch <- prometheus.MustNewConstMetric(
		dishEthSpeedMbps, prometheus.GaugeValue, float64(dishStatus.GetEthSpeedMbps()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishSnrAboveNoiseFloor, prometheus.GaugeValue, flool(dishStatus.GetIsSnrAboveNoiseFloor()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishSnrPersistentlyLow, prometheus.GaugeValue, flool(dishStatus.GetIsSnrPersistentlyLow()),
	)

	ready := dishStatus.GetReadyStates()
	for name, v := range map[string]bool{
		"cady": ready.GetCady(),
		"scp":  ready.GetScp(),
		"l1l2": ready.GetL1L2(),
		"xphy": ready.GetXphy(),
		"aap":  ready.GetAap(),
		"rf":   ready.GetRf(),
	} {
		ch <- prometheus.MustNewConstMetric(
			dishReadyState, prometheus.GaugeValue, flool(v), name,
		)
	}

	state := dishConnectionState(dishStatus)
	for _, name := range dishConnectionStates() {
		ch <- prometheus.MustNewConstMetric(
			dishState, prometheus.GaugeValue, flool(name == state), name,
		)
	}

	gps := dishStatus.GetGpsStats()
	ch <- prometheus.MustNewConstMetric(
		dishGpsValid, prometheus.GaugeValue, flool(gps.GetGpsValid()),
	)
	ch <- prometheus.MustNewConstMetric(
		dishGpsSatellites, prometheus.GaugeValue, float64(gps.GetGpsSats()),
	)

	align := dishStatus.GetAlignmentStats()
	ch <- prometheus.MustNewConstMetric(
		dishTiltAngleDeg, prometheus.GaugeValue, float64(align.GetTiltAngleDeg()),
	)
	ch <- prometheus.MustNewConstMetric(
		dishDesiredBoreSightAzimuthDeg, prometheus.GaugeValue, float64(align.GetDesiredBoresightAzimuthDeg()),
	)
	ch <- prometheus.MustNewConstMetric(
		dishDesiredBoreSightElevationDeg, prometheus.GaugeValue, float64(align.GetDesiredBoresightElevationDeg()),
	)
}

func (e *Exporter) collectDishSoftwareUpdate(ch chan<- prometheus.Metric, dishStatus *device.DishGetStatusResponse) {
	stats := dishStatus.GetSoftwareUpdateStats()
	current := dishStatus.GetSoftwareUpdateState()
	for num, name := range device.SoftwareUpdateState_name {
		ch <- prometheus.MustNewConstMetric(
			dishSoftwareUpdateState, prometheus.GaugeValue, flool(int32(current) == num), name,
		)
	}

	ch <- prometheus.MustNewConstMetric(
		dishSoftwareUpdateProgress, prometheus.GaugeValue, float64(stats.GetSoftwareUpdateProgress()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishSoftwareUpdateRebootReady, prometheus.GaugeValue, flool(dishStatus.GetSwupdateRebootReady()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishSoftwareUpdateRebootScheduled, prometheus.GaugeValue, float64(stats.GetRebootScheduledUtcTime()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishSecondsUntilSoftwareUpdateRebootPossible, prometheus.GaugeValue, float64(dishStatus.GetSecondsUntilSwupdateRebootPossible()),
	)
}

func (e *Exporter) collectDishObstructions(ch chan<- prometheus.Metric, dishStatus *device.DishGetStatusResponse) {
	obstructions := dishStatus.GetObstructionStats()

	ch <- prometheus.MustNewConstMetric(
		dishCurrentlyObstructed, prometheus.GaugeValue, flool(obstructions.GetCurrentlyObstructed()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishFractionObstructionRatio, prometheus.GaugeValue, float64(obstructions.GetFractionObstructed()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishValidSeconds, prometheus.GaugeValue, float64(obstructions.GetValidS()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishProlongedObstructionDurationSeconds, prometheus.GaugeValue, float64(obstructions.GetAvgProlongedObstructionDurationS()),
	)

	ch <- prometheus.MustNewConstMetric(
		dishProlongedObstructionIntervalSeconds, prometheus.GaugeValue, float64(obstructions.GetAvgProlongedObstructionIntervalS()),
	)
}

func (e *Exporter) collectDishAlerts(ch chan<- prometheus.Metric, dishStatus *device.DishGetStatusResponse) {
	alerts := dishStatus.GetAlerts()

	for desc, v := range map[*prometheus.Desc]bool{
		dishAlertMotorsStuck:                alerts.GetMotorsStuck(),
		dishAlertThermalThrottle:            alerts.GetThermalThrottle(),
		dishAlertThermalShutdown:            alerts.GetThermalShutdown(),
		dishAlertMastNotNearVertical:        alerts.GetMastNotNearVertical(),
		dishUnexpectedLocation:              alerts.GetUnexpectedLocation(),
		dishSlowEthernetSpeeds:              alerts.GetSlowEthernetSpeeds(),
		dishAlertInstallPending:             alerts.GetInstallPending(),
		dishAlertIsHeating:                  alerts.GetIsHeating(),
		dishAlertPowerSupplyThermalThrottle: alerts.GetPowerSupplyThermalThrottle(),
		dishAlertIsPowerSaveIdle:            alerts.GetIsPowerSaveIdle(),
		dishAlertLowMotorCurrent:            alerts.GetLowMotorCurrent(),
		dishAlertLowerSignalThanPredicted:   alerts.GetLowerSignalThanPredicted(),
		dishAlertSlowEthernetSpeeds100:      alerts.GetSlowEthernetSpeeds_100(),
		dishAlertNoEthernetLink:             alerts.GetNoEthernetLink(),
		dishAlertUpsuRouterPortSlow:         alerts.GetUpsuRouterPortSlow(),
		dishAlertDishWaterDetected:          alerts.GetDishWaterDetected(),
		dishAlertRouterWaterDetected:        alerts.GetRouterWaterDetected(),
	} {
		ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, flool(v))
	}
}

// dishConnectionState derives a connectivity state the way sparky8512's
// starlink-grpc-tools does: current firmware has no plain state field, only an
// outage record. No outage means CONNECTED; NO_SCHEDULE is reported as
// SEARCHING; any other cause is reported under its own name.
func dishConnectionState(dishStatus *device.DishGetStatusResponse) string {
	outage := dishStatus.GetOutage()
	if outage == nil {
		return "CONNECTED"
	}
	if outage.GetCause() == device.DishOutage_NO_SCHEDULE {
		return "SEARCHING"
	}
	if name, ok := device.DishOutage_Cause_name[int32(outage.GetCause())]; ok {
		return name
	}
	return "UNKNOWN"
}

// dishConnectionStates lists every value dishConnectionState can return, in a
// stable order, so each scrape exports the full set with 0/1 values.
func dishConnectionStates() []string {
	states := []string{"CONNECTED", "SEARCHING"}
	var causes []int
	for num, name := range device.DishOutage_Cause_name {
		if name != "NO_SCHEDULE" {
			causes = append(causes, int(num))
		}
	}
	sort.Ints(causes)
	for _, num := range causes {
		states = append(states, device.DishOutage_Cause_name[int32(num)])
	}
	return states
}

// collectDishHistory exports power draw and a summary of the dish's 15 minute
// history, which get_status's single snapshot misses (bursts, worst latency),
// and counts the outages the dish lists. History is the heaviest call, so
// failures are logged and skipped rather than failing the scrape.
func (e *Exporter) collectDishHistory(ch chan<- prometheus.Metric) {
	req := &device.Request{
		Request: &device.Request_GetHistory{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	resp, err := e.Client.Handle(ctx, req)
	if err != nil {
		log.Errorf("failed to collect history from dish: %s", err.Error())
		return
	}

	history := resp.GetDishGetHistory()
	current := history.GetCurrent()

	power := recorded(history.GetPowerIn(), current)
	if latest, ok := newest(history.GetPowerIn(), current); ok {
		var sum float64
		for _, v := range power {
			sum += float64(v)
		}
		ch <- prometheus.MustNewConstMetric(
			dishPowerInputWatts, prometheus.GaugeValue, float64(latest),
		)
		ch <- prometheus.MustNewConstMetric(
			dishPowerInputMeanWatts, prometheus.GaugeValue, sum/float64(len(power)),
		)
	}

	downlink := recorded(history.GetDownlinkThroughputBps(), current)
	uplink := recorded(history.GetUplinkThroughputBps(), current)
	drop := recorded(history.GetPopPingDropRate(), current)
	latency := recorded(history.GetPopPingLatencyMs(), current)
	if window := len(downlink); window > 0 {
		ch <- prometheus.MustNewConstMetric(
			dishHistoryWindowSeconds, prometheus.GaugeValue, float64(window),
		)
	}
	emitSummary(ch, dishHistoryDownlinkThroughputBps, float64s(downlink), 1)
	emitSummary(ch, dishHistoryUplinkThroughputBps, float64s(uplink), 1)
	emitSummary(ch, dishHistoryPopPingDropRatio, float64s(drop), 1)
	emitSummary(ch, dishHistoryPopPingLatencySeconds, latencyWhenAnswered(latency, drop), 1e-3)

	e.outages.observe(history.GetOutages())
	count, seconds := e.outages.totals()
	causes := make([]string, 0, len(device.DishOutage_Cause_name))
	for _, name := range device.DishOutage_Cause_name {
		causes = append(causes, name)
	}
	sort.Strings(causes)
	for _, cause := range causes {
		ch <- prometheus.MustNewConstMetric(
			dishOutagesTotal, prometheus.CounterValue, count[cause], cause,
		)
		ch <- prometheus.MustNewConstMetric(
			dishOutageSecondsTotal, prometheus.CounterValue, seconds[cause], cause,
		)
	}
}

// emitSummary exports the mean, p95 and max of samples, scaled, as three
// series of desc told apart by a stat label.
func emitSummary(ch chan<- prometheus.Metric, desc *prometheus.Desc, samples []float64, scale float64) {
	s, ok := summarise(samples)
	if !ok {
		return
	}
	ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, s.mean*scale, "mean")
	ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, s.p95*scale, "p95")
	ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, s.max*scale, "max")
}

// collectDishObstructionMap exports the obstruction map folded into 16 compass
// sectors. The raw map is a 123x123 grid, far too many cells for labels, and it
// is a separate, larger request, so failures are logged and skipped rather than
// failing the scrape.
func (e *Exporter) collectDishObstructionMap(ch chan<- prometheus.Metric) {
	req := &device.Request{
		Request: &device.Request_DishGetObstructionMap{},
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second*3)
	defer cancel()
	resp, err := e.Client.Handle(ctx, req)
	if err != nil {
		log.Errorf("failed to collect obstruction map from dish: %s", err.Error())
		return
	}

	for _, s := range sectorObstructions(resp.GetDishGetObstructionMap()) {
		ch <- prometheus.MustNewConstMetric(
			dishObstructionSectorObservedFraction, prometheus.GaugeValue, s.observed, s.name,
		)
		if s.observed == 0 {
			continue // no data: a zero fraction or elevation would read as "clear"
		}
		ch <- prometheus.MustNewConstMetric(
			dishObstructionSectorFraction, prometheus.GaugeValue, s.obstructed, s.name,
		)
		ch <- prometheus.MustNewConstMetric(
			dishObstructionSectorClearAboveDeg, prometheus.GaugeValue, s.clearAbove, s.name,
		)
	}
}

func flool(b bool) float64 {
	if b {
		return 1.00
	}
	return 0.00
}
