package storage

import (
	"fmt"
	"math/rand"
	"sort"
	"testing"
	"time"

	"github.com/VictoriaMetrics/VictoriaMetrics/lib/fs"
)

func TestTableSearch(t *testing.T) {
	rng := rand.New(rand.NewSource(1))

	var trData TimeRange
	trData.fromPartitionTime(time.Now())
	trData.MinTimestamp -= 5 * 365 * 24 * 3600 * 1000

	t.Run("SinglePartition", func(t *testing.T) {
		trSearch := TimeRange{
			MinTimestamp: trData.MinTimestamp + 4e3,
			MaxTimestamp: trData.MaxTimestamp - 4e3,
		}
		testTableSearchEx(t, rng, trData, trSearch, 1, 10, 1000, 10)
	})

	t.Run("SinglePartPerPartition", func(t *testing.T) {
		trSearch := TimeRange{
			MinTimestamp: trData.MinTimestamp + 4e3,
			MaxTimestamp: trData.MaxTimestamp - 4e3,
		}
		testTableSearchEx(t, rng, trData, trSearch, 12, 1, 1000, 10)
	})

	t.Run("SingleRowPerPartition", func(t *testing.T) {
		trSearch := TimeRange{
			MinTimestamp: trData.MinTimestamp + 4e3,
			MaxTimestamp: trData.MaxTimestamp - 4e3,
		}
		testTableSearchEx(t, rng, trData, trSearch, 12, 20, 1, 10)
	})

	t.Run("SingleTSID", func(t *testing.T) {
		trSearch := TimeRange{
			MinTimestamp: trData.MinTimestamp + 4e3,
			MaxTimestamp: trData.MaxTimestamp - 4e3,
		}
		testTableSearchEx(t, rng, trData, trSearch, 12, 5, 1000, 1)
	})

	t.Run("ManyPartitions", func(t *testing.T) {
		trSearch := TimeRange{
			MinTimestamp: trData.MinTimestamp + 4e3,
			MaxTimestamp: trData.MaxTimestamp - 4e3,
		}
		testTableSearchEx(t, rng, trData, trSearch, 20, 10, 30, 20)
	})

	t.Run("ManyTSIDs", func(t *testing.T) {
		trSearch := TimeRange{
			MinTimestamp: trData.MinTimestamp + 4e3,
			MaxTimestamp: trData.MaxTimestamp - 4e3,
		}
		testTableSearchEx(t, rng, trData, trSearch, 2, 5, 5000, 1000)
	})

	t.Run("ExactTimeRange", func(t *testing.T) {
		trSearch := TimeRange{
			MinTimestamp: trData.MinTimestamp,
			MaxTimestamp: trData.MaxTimestamp,
		}
		testTableSearchEx(t, rng, trData, trSearch, 2, 5, 1000, 10)
	})

	t.Run("InnerTimeRange", func(t *testing.T) {
		trSearch := TimeRange{
			MinTimestamp: trData.MinTimestamp + 4e3,
			MaxTimestamp: trData.MaxTimestamp - 4e3,
		}
		testTableSearchEx(t, rng, trData, trSearch, 2, 5, 1000, 10)
	})

	t.Run("OuterTimeRange", func(t *testing.T) {
		trSearch := TimeRange{
			MinTimestamp: trData.MinTimestamp - 1e6,
			MaxTimestamp: trData.MaxTimestamp + 1e6,
		}
		testTableSearchEx(t, rng, trData, trSearch, 2, 5, 1000, 10)
	})

	t.Run("LowTimeRange", func(t *testing.T) {
		trSearch := TimeRange{
			MinTimestamp: trData.MinTimestamp - 2e6,
			MaxTimestamp: trData.MinTimestamp - 1e6,
		}
		testTableSearchEx(t, rng, trData, trSearch, 2, 5, 1000, 10)
	})

	t.Run("HighTimeRange", func(t *testing.T) {
		trSearch := TimeRange{
			MinTimestamp: trData.MaxTimestamp + 1e6,
			MaxTimestamp: trData.MaxTimestamp + 2e6,
		}
		testTableSearchEx(t, rng, trData, trSearch, 2, 5, 1000, 10)
	})

	t.Run("LowerEndTimeRange", func(t *testing.T) {
		trSearch := TimeRange{
			MinTimestamp: trData.MinTimestamp - 1e6,
			MaxTimestamp: trData.MaxTimestamp - 4e3,
		}
		testTableSearchEx(t, rng, trData, trSearch, 2, 5, 1000, 10)
	})

	t.Run("HigherEndTimeRange", func(t *testing.T) {
		trSearch := TimeRange{
			MinTimestamp: trData.MinTimestamp + 4e3,
			MaxTimestamp: trData.MaxTimestamp + 1e6,
		}
		testTableSearchEx(t, rng, trData, trSearch, 2, 5, 1000, 10)
	})
}

func testTableSearchEx(t *testing.T, rng *rand.Rand, trData, trSearch TimeRange, partitionsCount, maxPartsPerPartition, maxRowsPerPart, tsidsCount int) {
	t.Helper()

	// Generate tsids to search.
	var tsids []TSID
	var tsid TSID
	for i := 0; i < 25; i++ {
		tsid.MetricID = uint64(rng.Intn(tsidsCount * 2))
		tsids = append(tsids, tsid)
	}
	sort.Slice(tsids, func(i, j int) bool { return tsids[i].Less(&tsids[j]) })

	// Generate the expected blocks.

	var r rawRow
	r.PrecisionBits = 24

	rowsCountExpected := int64(0)
	rbsExpected := []rawBlock{}
	var ptr TimeRange
	ptr.fromPartitionTimestamp(trData.MinTimestamp)
	var rowss [][]rawRow
	for i := 0; i < partitionsCount; i++ {
		partsCount := rng.Intn(maxPartsPerPartition) + 1
		for j := 0; j < partsCount; j++ {
			var rows []rawRow
			timestamp := ptr.MinTimestamp
			rowsCount := rng.Intn(maxRowsPerPart) + 1
			for k := 0; k < rowsCount; k++ {
				r.TSID.MetricID = uint64(rng.Intn(tsidsCount))
				r.Timestamp = timestamp
				r.Value = float64(int(rng.NormFloat64() * 1e5))

				timestamp += int64(rng.Intn(1e4)) + 1
				if timestamp > ptr.MaxTimestamp {
					break
				}
				rows = append(rows, r)
				rowsCountExpected++
			}
			rbs := getTestExpectedRawBlocks(rows, tsids, trSearch)
			rbsExpected = append(rbsExpected, rbs...)
			rowss = append(rowss, rows)
		}
		// Go to the next partition.
		ptr.fromPartitionTimestamp(ptr.MaxTimestamp + 1)
		if ptr.MaxTimestamp > trData.MaxTimestamp {
			break
		}
	}
	sort.Slice(rbsExpected, func(i, j int) bool {
		a, b := rbsExpected[i], rbsExpected[j]
		if a.TSID.Less(&b.TSID) {
			return true
		}
		if b.TSID.Less(&a.TSID) {
			return false
		}
		return a.Timestamps[0] < b.Timestamps[0]
	})

	// Create a table from rowss and test search on it.
	strg := newTestStorage()
	tb := mustOpenTable("test-table", strg)
	defer fs.MustRemoveDir("test-table")
	for _, rows := range rowss {
		tb.MustAddRows(rows)

		// Flush rows to parts.
		tb.DebugFlush()
	}
	testTableSearch(t, tb, tsids, trSearch, rbsExpected, -1)
	tb.MustClose()

	// Open the created table and test search on it.
	tb = mustOpenTable("test-table", strg)
	testTableSearch(t, tb, tsids, trSearch, rbsExpected, rowsCountExpected)
	tb.MustClose()
	stopTestStorage(strg)
}

func testTableSearch(t *testing.T, tb *table, tsids []TSID, tr TimeRange, rbsExpected []rawBlock, rowsCountExpected int64) {
	t.Helper()

	if err := testTableSearchSerial(tb, tsids, tr, rbsExpected, rowsCountExpected); err != nil {
		t.Fatalf("unexpected error in serial table search: %s", err)
	}

	ch := make(chan error, 5)
	for i := 0; i < cap(ch); i++ {
		go func() {
			ch <- testTableSearchSerial(tb, tsids, tr, rbsExpected, rowsCountExpected)
		}()
	}
	for i := 0; i < cap(ch); i++ {
		select {
		case err := <-ch:
			if err != nil {
				t.Fatalf("unexpected error in concurrent table search: %s", err)
			}
		case <-time.After(time.Second):
			t.Fatalf("timeout in concurrent table search")
		}
	}
}

func testTableSearchSerial(tb *table, tsids []TSID, tr TimeRange, rbsExpected []rawBlock, rowsCountExpected int64) error {
	if rowsCountExpected >= 0 {
		// Verify rows count only on the table opened from file.
		//
		// The online created table may not contain all the rows, since
		// they may race with raw rows flusher.
		var m TableMetrics
		tb.UpdateMetrics(&m)
		if rowsCount := m.TotalRowsCount(); rowsCount != uint64(rowsCountExpected) {
			return fmt.Errorf("unexpected rows count in the table; got %d; want %d", rowsCount, rowsCountExpected)
		}
	}

	bs := []Block{}
	var ts tableSearch
	ts.Init(tb, tsids, tr)
	for ts.NextBlock() {
		var b Block
		ts.BlockRef.MustReadBlock(&b)
		bs = append(bs, b)
	}
	if err := ts.Error(); err != nil {
		return fmt.Errorf("unexpected error: %w", err)
	}
	ts.MustClose()
	rbs := newTestRawBlocks(bs, tr)
	if err := testEqualRawBlocks(rbs, rbsExpected); err != nil {
		return fmt.Errorf("unequal blocks: %w", err)
	}

	if rowsCountExpected >= 0 {
		var m TableMetrics
		tb.UpdateMetrics(&m)
		if rowsCount := m.TotalRowsCount(); rowsCount != uint64(rowsCountExpected) {
			return fmt.Errorf("unexpected rows count in the table; got %d; want %d", rowsCount, rowsCountExpected)
		}
	}

	// verify that empty tsids returns empty result
	ts.Init(tb, []TSID{}, tr)
	if ts.NextBlock() {
		return fmt.Errorf("unexpected block got for an empty tsids list: %+v", ts.BlockRef)
	}
	if err := ts.Error(); err != nil {
		return fmt.Errorf("unexpected error on empty tsids list: %w", err)
	}
	ts.MustClose()

	return nil
}
