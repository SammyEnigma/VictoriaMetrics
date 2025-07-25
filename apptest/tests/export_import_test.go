package tests

import (
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/VictoriaMetrics/VictoriaMetrics/apptest"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/fs"
)

func TestSingleExportImportNative(t *testing.T) {
	fs.MustRemoveDir(t.Name())

	tc := apptest.NewTestCase(t)
	defer tc.Stop()

	sut := tc.MustStartDefaultVmsingle()

	testExportImportNative(tc.T(), sut)
}

func TestClusterExportImportNative(t *testing.T) {
	fs.MustRemoveDir(t.Name())

	tc := apptest.NewTestCase(t)
	defer tc.Stop()

	sut := tc.MustStartDefaultCluster()

	testExportImportNative(tc.T(), sut)
}

// testExportImportNative test export and import in VictoriaMetrics’ native format.
// see: https://docs.victoriametrics.com/#how-to-import-data-in-native-format
func testExportImportNative(t *testing.T, sut apptest.PrometheusWriteQuerier) {
	// create test data
	sut.PrometheusAPIV1ImportPrometheus(t, []string{
		`native_export_import 10 1707123456700`, // 2024-02-05T08:57:36.700Z
	}, apptest.QueryOpts{
		ExtraLabels: []string{"el1=elv1", "el2=elv2"},
	})
	sut.ForceFlush(t)

	// export test data via native export API
	exportResult := sut.PrometheusAPIV1ExportNative(t, "native_export_import", apptest.QueryOpts{
		Start: "2024-02-05T08:50:00.700Z",
		End:   "2024-02-05T09:00:00.700Z",
	})

	// re-import test data via native import API
	sut.PrometheusAPIV1ImportNative(t, exportResult, apptest.QueryOpts{})
	sut.ForceFlush(t)

	// check query result
	got := sut.PrometheusAPIV1QueryRange(t, "native_export_import", apptest.QueryOpts{
		Start: "2024-02-05T08:57:36.700Z",
		End:   "2024-02-05T08:57:36.700Z",
		Step:  "60s",
	})

	cmpOptions := []cmp.Option{
		cmpopts.IgnoreFields(apptest.PrometheusAPIV1QueryResponse{}, "Status", "Data.ResultType"),
		cmpopts.EquateNaNs(),
	}
	want := apptest.NewPrometheusAPIV1QueryResponse(t, `{"data": {"result": [{"metric": {"__name__": "native_export_import", "el1": "elv1", "el2":"elv2"}, "values": []}]}}`)
	want.Data.Result[0].Samples = []*apptest.Sample{
		apptest.NewSample(t, "2024-02-05T08:57:36.700Z", 10),
	}
	if diff := cmp.Diff(want, got, cmpOptions...); diff != "" {
		t.Errorf("unexpected response (-want, +got):\n%s", diff)
	}
}
