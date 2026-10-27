<p align="center">
  <img alt="logo" src="https://github.com/danopstech/starlink_exporter/raw/main/.docs/assets/logo.jpg" height="150" />
  <h3 align="center">Starlink Prometheus Exporter</h3>
</p>

---
A [Starlink](https://www.starlink.com/) exporter for Prometheus. Not affiliated with or acting on behalf of Starlink(™)

This is a fork of [danopstech/starlink_exporter](https://github.com/danopstech/starlink_exporter), which has been dormant since 2023. The upstream protobuf definitions predate current dish firmware, so against a current dish (API v43, firmware 2026.09) upstream reports `state`, `snr` and `uptime_seconds` as 0, leaves the `starlink_dish_info` labels empty and never decodes most alerts. This fork:

- regenerates `pkg/` from the descriptors the dish serves over gRPC reflection (`hack/regen-protos.sh`);
- reads device info and uptime from `get_status`, because current firmware answers `dish_get_context` with `PermissionDenied`;
- fetches `get_status` once per scrape instead of three times;
- adds ethernet speed, SNR flags, subsystem readiness, GPS, alignment, software update state and the full alert set (see below).

Metric names that existed upstream and still exist on current firmware are unchanged, so the [Grafana dashboard](https://grafana.com/grafana/dashboards/14337) keeps working.

## Usage:

### Flags

`starlink_exporter` is configured through the use of optional command line flags

```bash
$ ./starlink_exporter --help
Usage of starlink_exporter
  -address string
        IP address and port to reach dish (default "192.168.100.1:9200")
  -port string
        listening port to expose metrics on (default "9817")

```

### Build

```bash
go build ./cmd/starlink_exporter
./starlink_exporter [flags]
```

### Container image

`Dockerfile.build` builds from source (the upstream `Dockerfile` expects a goreleaser binary). `hack/build-image.sh` builds it on an in-cluster BuildKit daemon and pushes `registry.example.com/starlink_exporter:fb-<commit>`; run it from a clean checkout with a kubectl context for the cluster.

### Regenerating the protos

When dish firmware changes its schema, with the dish reachable:

```bash
hack/regen-protos.sh [dish-address]
```

This needs `grpcurl`, `protoc` and `protoc-gen-go` v1.27.1 (the version matching `google.golang.org/protobuf` in `go.mod`).

### Setup Prometheus to scrape `starlink_exporter`

Configure [Prometheus](https://prometheus.io/) to scrape metrics from localhost:9817/metrics

```yaml
...
scrape_configs
    - job_name: starlink
      scrape_interval: 3s
      scrape_timeout:  3s
      static_configs:
        - targets: ['localhost:9817']
...
```

## Exported Metrics:

```text
# HELP starlink_dish_alert_dish_water_detected Whether water is detected in the dish
# TYPE starlink_dish_alert_dish_water_detected gauge
# HELP starlink_dish_alert_install_pending Whether a software update is pending installation
# TYPE starlink_dish_alert_install_pending gauge
# HELP starlink_dish_alert_is_heating Whether the dish is heating (snow melt)
# TYPE starlink_dish_alert_is_heating gauge
# HELP starlink_dish_alert_is_power_save_idle Whether the dish is idle in power save mode
# TYPE starlink_dish_alert_is_power_save_idle gauge
# HELP starlink_dish_alert_low_motor_current Status of motor current
# TYPE starlink_dish_alert_low_motor_current gauge
# HELP starlink_dish_alert_mast_not_near_vertical Status of mast position
# TYPE starlink_dish_alert_mast_not_near_vertical gauge
# HELP starlink_dish_alert_motors_stuck Status of motor stuck
# TYPE starlink_dish_alert_motors_stuck gauge
# HELP starlink_dish_alert_no_ethernet_link Whether the dish has no ethernet link
# TYPE starlink_dish_alert_no_ethernet_link gauge
# HELP starlink_dish_alert_power_supply_thermal_throttle Status of power supply thermal throttling
# TYPE starlink_dish_alert_power_supply_thermal_throttle gauge
# HELP starlink_dish_alert_router_water_detected Whether water is detected in the router
# TYPE starlink_dish_alert_router_water_detected gauge
# HELP starlink_dish_alert_signal_lower_than_predicted Whether the signal is lower than predicted
# TYPE starlink_dish_alert_signal_lower_than_predicted gauge
# HELP starlink_dish_alert_slow_eth_speeds Status of ethernet
# TYPE starlink_dish_alert_slow_eth_speeds gauge
# HELP starlink_dish_alert_slow_eth_speeds_100 Status of ethernet negotiating 100 Mbps
# TYPE starlink_dish_alert_slow_eth_speeds_100 gauge
# HELP starlink_dish_alert_thermal_shutdown Status of thermal shutdown
# TYPE starlink_dish_alert_thermal_shutdown gauge
# HELP starlink_dish_alert_thermal_throttle Status of thermal throttling
# TYPE starlink_dish_alert_thermal_throttle gauge
# HELP starlink_dish_alert_unexpected_location Status of location
# TYPE starlink_dish_alert_unexpected_location gauge
# HELP starlink_dish_alert_upsu_router_port_slow Status of the power supply router port speed
# TYPE starlink_dish_alert_upsu_router_port_slow gauge
# HELP starlink_dish_boot_count Number of times the dish has booted
# TYPE starlink_dish_boot_count gauge
# HELP starlink_dish_bore_sight_azimuth_deg azimuth in degrees
# TYPE starlink_dish_bore_sight_azimuth_deg gauge
# HELP starlink_dish_bore_sight_elevation_deg elevation in degrees
# TYPE starlink_dish_bore_sight_elevation_deg gauge
# HELP starlink_dish_currently_obstructed Status of view of the sky
# TYPE starlink_dish_currently_obstructed gauge
# HELP starlink_dish_desired_bore_sight_azimuth_deg Azimuth the dish wants to point at in degrees
# TYPE starlink_dish_desired_bore_sight_azimuth_deg gauge
# HELP starlink_dish_desired_bore_sight_elevation_deg Elevation the dish wants to point at in degrees
# TYPE starlink_dish_desired_bore_sight_elevation_deg gauge
# HELP starlink_dish_downlink_throughput_bytes Amount of bandwidth in bytes per second download
# TYPE starlink_dish_downlink_throughput_bytes gauge
# HELP starlink_dish_eth_speed_mbps Negotiated speed of the dish ethernet link in Mbps
# TYPE starlink_dish_eth_speed_mbps gauge
# HELP starlink_dish_first_nonempty_slot_seconds Seconds to next non empty slot
# TYPE starlink_dish_first_nonempty_slot_seconds gauge
# HELP starlink_dish_fraction_obstruction_ratio Percentage of obstruction
# TYPE starlink_dish_fraction_obstruction_ratio gauge
# HELP starlink_dish_gps_satellites Number of GPS satellites the dish is using
# TYPE starlink_dish_gps_satellites gauge
# HELP starlink_dish_gps_valid Whether the dish has a valid GPS fix
# TYPE starlink_dish_gps_valid gauge
# HELP starlink_dish_info Running software versions and IDs of hardware
# TYPE starlink_dish_info gauge
# HELP starlink_dish_pop_ping_drop_ratio Percent of pings dropped
# TYPE starlink_dish_pop_ping_drop_ratio gauge
# HELP starlink_dish_pop_ping_latency_seconds Latency of connection in seconds
# TYPE starlink_dish_pop_ping_latency_seconds gauge
# HELP starlink_dish_prolonged_obstruction_duration_seconds Average in seconds of prolonged obstructions
# TYPE starlink_dish_prolonged_obstruction_duration_seconds gauge
# HELP starlink_dish_prolonged_obstruction_interval_seconds Average prolonged obstruction interval in seconds
# TYPE starlink_dish_prolonged_obstruction_interval_seconds gauge
# HELP starlink_dish_ready_state Whether a dish subsystem reports ready
# TYPE starlink_dish_ready_state gauge
# HELP starlink_dish_scrape_duration_seconds Time to scrape metrics from starlink dish
# TYPE starlink_dish_scrape_duration_seconds gauge
# HELP starlink_dish_seconds_until_software_update_reboot_possible Seconds until the dish may reboot for a software update, -1 when not applicable
# TYPE starlink_dish_seconds_until_software_update_reboot_possible gauge
# HELP starlink_dish_snr_above_noise_floor Whether the signal to noise ratio is above the noise floor
# TYPE starlink_dish_snr_above_noise_floor gauge
# HELP starlink_dish_snr_persistently_low Whether the signal to noise ratio is persistently low
# TYPE starlink_dish_snr_persistently_low gauge
# HELP starlink_dish_software_update_progress Software update progress, 0 to 1
# TYPE starlink_dish_software_update_progress gauge
# HELP starlink_dish_software_update_reboot_ready Whether the dish is ready to reboot to apply a software update
# TYPE starlink_dish_software_update_reboot_ready gauge
# HELP starlink_dish_software_update_reboot_scheduled_timestamp_seconds Unix time of the scheduled software update reboot, 0 when none is scheduled
# TYPE starlink_dish_software_update_reboot_scheduled_timestamp_seconds gauge
# HELP starlink_dish_software_update_state Current software update state, 1 for the active state
# TYPE starlink_dish_software_update_state gauge
# HELP starlink_dish_tilt_angle_deg Tilt of the dish from vertical in degrees
# TYPE starlink_dish_tilt_angle_deg gauge
# HELP starlink_dish_up Was the last query of Starlink dish successful.
# TYPE starlink_dish_up gauge
# HELP starlink_dish_uplink_throughput_bytes Amount of bandwidth in bytes per second upload
# TYPE starlink_dish_uplink_throughput_bytes gauge
# HELP starlink_dish_uptime_seconds Dish running time
# TYPE starlink_dish_uptime_seconds gauge
# HELP starlink_dish_valid_seconds Unknown
# TYPE starlink_dish_valid_seconds gauge
```

Notes:

- `starlink_dish_*_throughput_bytes` keep their upstream names but carry bits per second.
- `starlink_dish_software_update_state{state}` is 1 for the active state and 0 for the rest.
- `starlink_dish_software_update_reboot_scheduled_timestamp_seconds` is the Unix time of the scheduled update reboot, 0 when none is scheduled.
- `starlink_dish_cell_id`, `pop_rack_id`, `initial_satellite_id`, `initial_gateway_id`, `backup_beam` and `time_to_slot_end_seconds` come from `dish_get_context`, which current firmware denies. They are exported only when the dish allows that call, so they are not in the list above.
- Removed because current firmware no longer sends the fields: `starlink_dish_state`, `starlink_dish_snr`, `starlink_dish_last_24h_obstructed_seconds` and the `wedge` obstruction ratios. Use `snr_above_noise_floor` and `ready_state` instead.

## Example Grafana Dashboard:

https://grafana.com/grafana/dashboards/14337

<p align="center">
	<img src="https://github.com/danopstech/starlink_exporter/raw/main/.docs/assets/screenshot.jpg" width="95%">
</p>
