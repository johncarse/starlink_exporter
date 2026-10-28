package exporter

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

const dashboardPath = "../../dashboards/starlink.json"

func loadDashboard(t *testing.T) map[string]interface{} {
	t.Helper()
	raw, err := os.ReadFile(dashboardPath)
	if err != nil {
		t.Fatalf("read dashboard: %v", err)
	}
	var dash map[string]interface{}
	if err := json.Unmarshal(raw, &dash); err != nil {
		t.Fatalf("dashboard is not valid JSON: %v", err)
	}
	return dash
}

// exprs returns every PromQL expression in the dashboard: panel targets,
// annotations and template variables.
func exprs(dash map[string]interface{}) []string {
	var out []string
	var walk func(v interface{})
	walk = func(v interface{}) {
		switch v := v.(type) {
		case map[string]interface{}:
			for k, child := range v {
				if s, ok := child.(string); ok && (k == "expr" || k == "definition") {
					out = append(out, s)
				}
				walk(child)
			}
		case []interface{}:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(dash)
	return out
}

var describedName = regexp.MustCompile(`fqName: "([^"]+)"`)

func describedMetrics(t *testing.T) map[string]bool {
	t.Helper()
	ch := make(chan *prometheus.Desc, 256)
	go func() {
		(&Exporter{Client: healthyDish()}).Describe(ch)
		close(ch)
	}()
	names := map[string]bool{}
	for d := range ch {
		m := describedName.FindStringSubmatch(d.String())
		if m == nil {
			t.Fatalf("cannot read the name of descriptor %s", d)
		}
		names[m[1]] = true
	}
	return names
}

func TestDashboardQueriesOnlyMetricsTheExporterDescribes(t *testing.T) {
	described := describedMetrics(t)
	metric := regexp.MustCompile(`\bstarlink_[a-z0-9_]+`)
	seen := 0
	for _, e := range exprs(loadDashboard(t)) {
		for _, name := range metric.FindAllString(e, -1) {
			seen++
			if !described[name] {
				t.Errorf("dashboard queries %s, which the exporter does not export (in %q)", name, e)
			}
		}
	}
	if seen == 0 {
		t.Error("found no starlink_ metrics in the dashboard")
	}
}

func TestDashboardReferencesPrometheusThroughTheDatasourceVariableWhenItNamesADatasource(t *testing.T) {
	var walk func(v interface{})
	datasources := 0
	walk = func(v interface{}) {
		switch v := v.(type) {
		case map[string]interface{}:
			if ds, ok := v["datasource"].(map[string]interface{}); ok {
				datasources++
				if uid := ds["uid"]; uid != "${DS_PROMETHEUS}" {
					t.Errorf("datasource uid is %v, want ${DS_PROMETHEUS}", uid)
				}
			}
			for _, child := range v {
				walk(child)
			}
		case []interface{}:
			for _, child := range v {
				walk(child)
			}
		}
	}
	walk(loadDashboard(t))
	if datasources == 0 {
		t.Error("found no datasource references in the dashboard")
	}
}

func TestDashboardPanelsHaveUniqueIdsAndTitles(t *testing.T) {
	ids, titles := map[float64]bool{}, map[string]bool{}
	for _, p := range loadDashboard(t)["panels"].([]interface{}) {
		panel := p.(map[string]interface{})
		id, title := panel["id"].(float64), panel["title"].(string)
		if ids[id] {
			t.Errorf("panel id %v is used twice", id)
		}
		if titles[strings.ToLower(title)] {
			t.Errorf("panel title %q is used twice", title)
		}
		ids[id], titles[strings.ToLower(title)] = true, true
	}
}
