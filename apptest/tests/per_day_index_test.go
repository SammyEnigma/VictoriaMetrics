package tests

import (
	"fmt"
	"testing"

	"github.com/VictoriaMetrics/VictoriaMetrics/apptest"
)

func TestSingleSearchWithDisabledPerDayIndex(t *testing.T) {
	tc := apptest.NewTestCase(t)
	defer tc.Stop()

	testSearchWithDisabledPerDayIndex(tc, func(name string, disablePerDayIndex bool) apptest.PrometheusWriteQuerier {
		return tc.MustStartVmsingle("vmsingle-"+name, []string{
			"-storageDataPath=" + tc.Dir() + "/vmsingle",
			"-retentionPeriod=100y",
			"-search.maxStalenessInterval=1m",
			fmt.Sprintf("-disablePerDayIndex=%t", disablePerDayIndex),
		})
	})
}

func TestClusterSearchWithDisabledPerDayIndex(t *testing.T) {
	tc := apptest.NewTestCase(t)
	defer tc.Stop()

	testSearchWithDisabledPerDayIndex(tc, func(name string, disablePerDayIndex bool) apptest.PrometheusWriteQuerier {
		// Using static ports for vmstorage because random ports may cause
		// changes in how data is sharded.
		vmstorage1 := tc.MustStartVmstorage("vmstorage1-"+name, []string{
			"-storageDataPath=" + tc.Dir() + "/vmstorage1",
			"-retentionPeriod=100y",
			"-httpListenAddr=127.0.0.1:61001",
			"-vminsertAddr=127.0.0.1:61002",
			"-vmselectAddr=127.0.0.1:61003",
			fmt.Sprintf("-disablePerDayIndex=%t", disablePerDayIndex),
		})
		vmstorage2 := tc.MustStartVmstorage("vmstorage2-"+name, []string{
			"-storageDataPath=" + tc.Dir() + "/vmstorage2",
			"-retentionPeriod=100y",
			"-httpListenAddr=127.0.0.1:62001",
			"-vminsertAddr=127.0.0.1:62002",
			"-vmselectAddr=127.0.0.1:62003",
			fmt.Sprintf("-disablePerDayIndex=%t", disablePerDayIndex),
		})
		vminsert := tc.MustStartVminsert("vminsert-"+name, []string{
			"-storageNode=" + vmstorage1.VminsertAddr() + "," + vmstorage2.VminsertAddr(),
		})
		vmselect := tc.MustStartVmselect("vmselect"+name, []string{
			"-storageNode=" + vmstorage1.VmselectAddr() + "," + vmstorage2.VmselectAddr(),
			"-search.maxStalenessInterval=1m",
		})
		return &apptest.Vmcluster{
			Vmstorages: []*apptest.Vmstorage{vmstorage1, vmstorage2},
			Vminsert:   vminsert,
			Vmselect:   vmselect,
		}
	})
}

type startSUTFunc func(name string, disablePerDayIndex bool) apptest.PrometheusWriteQuerier

// testDisablePerDayIndex_Search shows what search results to expect when data
// is first inserted with per-day index enabled and then with per-day index
// disabled.
//
// The data inserted with enabled per-day index must be searchable with disabled
// per-day index.
//
// The data inserted with disabled per-day index is not searchable with per-day
// index enabled unless the search time range is > 40 days.
func testSearchWithDisabledPerDayIndex(tc *apptest.TestCase, start startSUTFunc) {
	t := tc.T()

	type opts struct {
		start, end       string
		wantSeries       []map[string]string
		wantQueryResults []*apptest.QueryResult
	}
	assertSearchResults := func(sut apptest.PrometheusQuerier, opts *opts) {
		t.Helper()
		tc.Assert(&apptest.AssertOptions{
			Msg: "unexpected /api/v1/series response",
			Got: func() any {
				return sut.PrometheusAPIV1Series(t, `{__name__=~".*"}`, apptest.QueryOpts{
					Start: opts.start,
					End:   opts.end,
				}).Sort()
			},
			Want: &apptest.PrometheusAPIV1SeriesResponse{
				Status: "success",
				Data:   opts.wantSeries,
			},
		})
		tc.Assert(&apptest.AssertOptions{
			Msg: "unexpected /api/v1/query_range response",
			Got: func() any {
				return sut.PrometheusAPIV1QueryRange(t, `{__name__=~".*"}`, apptest.QueryOpts{
					Start: opts.start,
					End:   opts.end,
					Step:  "1d",
				})
			},
			Want: &apptest.PrometheusAPIV1QueryResponse{
				Status: "success",
				Data: &apptest.QueryData{
					ResultType: "matrix",
					Result:     opts.wantQueryResults,
				},
			},
		})
	}

	// Start vmsingle with enabled per-day index, insert sample1, and confirm it
	// is searchable.
	sut := start("with-per-day-index", false)
	sample1 := []string{"metric1 111 1704067200000"} // 2024-01-01T00:00:00Z
	sut.PrometheusAPIV1ImportPrometheus(t, sample1, apptest.QueryOpts{})
	sut.ForceFlush(t)
	assertSearchResults(sut, &opts{
		start:      "2024-01-01T00:00:00Z",
		end:        "2024-01-01T23:59:59Z",
		wantSeries: []map[string]string{{"__name__": "metric1"}},
		wantQueryResults: []*apptest.QueryResult{
			{
				Metric:  map[string]string{"__name__": "metric1"},
				Samples: []*apptest.Sample{{Timestamp: 1704067200000, Value: float64(111)}},
			},
		},
	})

	// Restart vmsingle with disabled per-day index, insert sample2, and confirm
	// that both sample1 and sample2 is searchable.
	tc.StopPrometheusWriteQuerier(sut)
	sut = start("without-per-day-index", true)
	sample2 := []string{"metric2 222 1704067200000"} // 2024-01-01T00:00:00Z
	sut.PrometheusAPIV1ImportPrometheus(t, sample2, apptest.QueryOpts{})
	sut.ForceFlush(t)
	assertSearchResults(sut, &opts{
		start: "2024-01-01T00:00:00Z",
		end:   "2024-01-01T23:59:59Z",
		wantSeries: []map[string]string{
			{"__name__": "metric1"},
			{"__name__": "metric2"},
		},
		wantQueryResults: []*apptest.QueryResult{
			{
				Metric:  map[string]string{"__name__": "metric1"},
				Samples: []*apptest.Sample{{Timestamp: 1704067200000, Value: float64(111)}},
			},
			{
				Metric:  map[string]string{"__name__": "metric2"},
				Samples: []*apptest.Sample{{Timestamp: 1704067200000, Value: float64(222)}},
			},
		},
	})

	// Insert sample1 but for a different date, restart vmsingle with enabled
	// per-day index and confirm that:
	// - sample1 is searchable within the time range of Jan 1st
	// - sample1 is not searchable within the time range of Jan 20th
	// - sample1 is searchable within the time range of Jan 1st-20th (because
	//   the metric1 metricID will be found in the per-day index for Jan 1st).
	// - sample2 is not searchable when the time range is <= 40 days
	// - sample2 becomes searchable when the time range is > 40 days
	sample3 := []string{"metric1 333 1705708800000"} // 2024-01-20T00:00:00Z
	sut.PrometheusAPIV1ImportPrometheus(t, sample3, apptest.QueryOpts{})
	sut.ForceFlush(t)
	tc.StopPrometheusWriteQuerier(sut)
	sut = start("with-per-day-index2", false)

	// Time range is 1 day (Jan 1st) <= 40 days
	assertSearchResults(sut, &opts{
		start: "2024-01-01T00:00:00Z",
		end:   "2024-01-01T23:59:59Z",
		wantSeries: []map[string]string{
			{"__name__": "metric1"},
		},
		wantQueryResults: []*apptest.QueryResult{
			{
				Metric:  map[string]string{"__name__": "metric1"},
				Samples: []*apptest.Sample{{Timestamp: 1704067200000, Value: float64(111)}},
			},
		},
	})

	// Time range is 1 day (Jan 20th) <= 40 days
	assertSearchResults(sut, &opts{
		start:            "2024-01-20T00:00:00Z",
		end:              "2024-01-20T23:59:59Z",
		wantSeries:       []map[string]string{},
		wantQueryResults: []*apptest.QueryResult{},
	})

	// Time range is 20 days (Jan 1st-20th) <= 40 days
	assertSearchResults(sut, &opts{
		start: "2024-01-01T00:00:00Z",
		end:   "2024-01-20T23:59:59Z",
		wantSeries: []map[string]string{
			{"__name__": "metric1"},
		},
		wantQueryResults: []*apptest.QueryResult{
			{
				Metric: map[string]string{"__name__": "metric1"},
				Samples: []*apptest.Sample{
					{Timestamp: 1704067200000, Value: float64(111)},
					{Timestamp: 1705708800000, Value: float64(333)},
				},
			},
		},
	})

	// Time range > 40 days
	assertSearchResults(sut, &opts{
		start: "2024-01-01T00:00:00Z",
		end:   "2024-02-29T23:59:59Z",
		wantSeries: []map[string]string{
			{"__name__": "metric1"},
			{"__name__": "metric2"},
		},
		wantQueryResults: []*apptest.QueryResult{
			{
				Metric: map[string]string{"__name__": "metric1"},
				Samples: []*apptest.Sample{
					{Timestamp: 1704067200000, Value: float64(111)},
					{Timestamp: 1705708800000, Value: float64(333)},
				},
			},
			{
				Metric: map[string]string{"__name__": "metric2"},
				Samples: []*apptest.Sample{
					{Timestamp: 1704067200000, Value: float64(222)},
				},
			},
		},
	})
}

func TestSingleActiveTimeseriesMetric_enabledPerDayIndex(t *testing.T) {
	testSingleActiveTimeseriesMetric(t, false)
}

func TestSingleActiveTimeseriesMetric_disabledPerDayIndex(t *testing.T) {
	testSingleActiveTimeseriesMetric(t, true)
}

func testSingleActiveTimeseriesMetric(t *testing.T, disablePerDayIndex bool) {
	tc := apptest.NewTestCase(t)
	defer tc.Stop()

	vmsingle := tc.MustStartVmsingle("vmsingle", []string{
		fmt.Sprintf("-storageDataPath=%s/vmsingle-%t", tc.Dir(), disablePerDayIndex),
		fmt.Sprintf("-disablePerDayIndex=%t", disablePerDayIndex),
	})

	testActiveTimeseriesMetric(tc, vmsingle, func() int {
		return vmsingle.GetIntMetric(t, `vm_cache_entries{type="storage/hour_metric_ids"}`)
	})
}

func TestClusterActiveTimeseriesMetric_enabledPerDayIndex(t *testing.T) {
	testClusterActiveTimeseriesMetric(t, false)
}

func TestClusterActiveTimeseriesMetric_disabledPerDayIndex(t *testing.T) {
	testClusterActiveTimeseriesMetric(t, true)
}

func testClusterActiveTimeseriesMetric(t *testing.T, disablePerDayIndex bool) {
	tc := apptest.NewTestCase(t)
	defer tc.Stop()

	vmstorage1 := tc.MustStartVmstorage("vmstorage1", []string{
		fmt.Sprintf("-storageDataPath=%s/vmstorage1-%t", tc.Dir(), disablePerDayIndex),
		fmt.Sprintf("-disablePerDayIndex=%t", disablePerDayIndex),
	})
	vmstorage2 := tc.MustStartVmstorage("vmstorage2", []string{
		fmt.Sprintf("-storageDataPath=%s/vmstorage2-%t", tc.Dir(), disablePerDayIndex),
		fmt.Sprintf("-disablePerDayIndex=%t", disablePerDayIndex),
	})
	vminsert := tc.MustStartVminsert("vminsert", []string{
		"-storageNode=" + vmstorage1.VminsertAddr() + "," + vmstorage2.VminsertAddr(),
	})

	vmcluster := &apptest.Vmcluster{
		Vmstorages: []*apptest.Vmstorage{vmstorage1, vmstorage2},
		Vminsert:   vminsert,
	}

	testActiveTimeseriesMetric(tc, vmcluster, func() int {
		cnt1 := vmstorage1.GetIntMetric(t, `vm_cache_entries{type="storage/hour_metric_ids"}`)
		cnt2 := vmstorage2.GetIntMetric(t, `vm_cache_entries{type="storage/hour_metric_ids"}`)
		return cnt1 + cnt2
	})
}

func testActiveTimeseriesMetric(tc *apptest.TestCase, sut apptest.PrometheusWriteQuerier, getActiveTimeseries func() int) {
	t := tc.T()
	const numSamples = 1000
	samples := make([]string, numSamples)
	for i := range numSamples {
		samples[i] = fmt.Sprintf("metric_%03d %d", i, i)
	}
	sut.PrometheusAPIV1ImportPrometheus(t, samples, apptest.QueryOpts{})
	sut.ForceFlush(t)
	tc.Assert(&apptest.AssertOptions{
		Msg: `unexpected vm_cache_entries{type="storage/hour_metric_ids"} metric value`,
		Got: func() any {
			return getActiveTimeseries()
		},
		Want: numSamples,
	})
}
