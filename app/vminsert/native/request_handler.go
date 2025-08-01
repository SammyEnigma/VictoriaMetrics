package native

import (
	"net/http"
	"sync"

	"github.com/VictoriaMetrics/VictoriaMetrics/app/vminsert/common"
	"github.com/VictoriaMetrics/VictoriaMetrics/app/vminsert/relabel"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/logger"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/prompb"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/protoparser/native/stream"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/protoparser/protoparserutil"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/storage"
	"github.com/VictoriaMetrics/metrics"
)

var (
	rowsInserted  = metrics.NewCounter(`vm_rows_inserted_total{type="native"}`)
	rowsPerInsert = metrics.NewHistogram(`vm_rows_per_insert{type="native"}`)
)

// InsertHandler processes `/api/v1/import/native` request.
func InsertHandler(req *http.Request) error {
	extraLabels, err := protoparserutil.GetExtraLabels(req)
	if err != nil {
		return err
	}
	encoding := req.Header.Get("Content-Encoding")
	return stream.Parse(req.Body, encoding, func(block *stream.Block) error {
		return insertRows(block, extraLabels)
	})
}

func insertRows(block *stream.Block, extraLabels []prompb.Label) error {
	ctx := getPushCtx()
	defer putPushCtx(ctx)

	// Update rowsInserted and rowsPerInsert before actual inserting,
	// since relabeling can prevent from inserting the rows.
	rowsLen := len(block.Values)
	rowsInserted.Add(rowsLen)
	rowsPerInsert.Update(float64(rowsLen))

	ic := &ctx.Common
	ic.Reset(rowsLen)
	hasRelabeling := relabel.HasRelabeling()
	mn := &block.MetricName
	ic.Labels = ic.Labels[:0]
	ic.AddLabelBytes(nil, mn.MetricGroup)
	for j := range mn.Tags {
		tag := &mn.Tags[j]
		ic.AddLabelBytes(tag.Key, tag.Value)
	}
	for j := range extraLabels {
		label := &extraLabels[j]
		ic.AddLabel(label.Name, label.Value)
	}
	if !ic.TryPrepareLabels(hasRelabeling) {
		return nil
	}
	ctx.metricNameBuf = storage.MarshalMetricNameRaw(ctx.metricNameBuf[:0], ic.Labels)
	values := block.Values
	timestamps := block.Timestamps
	if len(timestamps) != len(values) {
		logger.Panicf("BUG: len(timestamps)=%d must match len(values)=%d", len(timestamps), len(values))
	}
	for j, value := range values {
		timestamp := timestamps[j]
		// TODO: @f41gh7 looks like it's better to use WriteDataPointExt
		// since metricName never changes inside insertRows call
		if err := ic.WriteDataPoint(ctx.metricNameBuf, ic.Labels, timestamp, value); err != nil {
			return err
		}
	}
	return ic.FlushBufs()
}

type pushCtx struct {
	Common        common.InsertCtx
	metricNameBuf []byte
}

func (ctx *pushCtx) reset() {
	ctx.Common.Reset(0)
	ctx.metricNameBuf = ctx.metricNameBuf[:0]
}

func getPushCtx() *pushCtx {
	if v := pushCtxPool.Get(); v != nil {
		return v.(*pushCtx)
	}
	return &pushCtx{}
}

func putPushCtx(ctx *pushCtx) {
	ctx.reset()
	pushCtxPool.Put(ctx)
}

var pushCtxPool sync.Pool
