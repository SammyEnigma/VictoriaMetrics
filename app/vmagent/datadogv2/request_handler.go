package datadogv2

import (
	"net/http"

	"github.com/VictoriaMetrics/VictoriaMetrics/app/vmagent/common"
	"github.com/VictoriaMetrics/VictoriaMetrics/app/vmagent/remotewrite"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/auth"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/prompb"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/protoparser/datadogutil"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/protoparser/datadogv2"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/protoparser/datadogv2/stream"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/protoparser/protoparserutil"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/tenantmetrics"
	"github.com/VictoriaMetrics/metrics"
)

var (
	rowsInserted       = metrics.NewCounter(`vmagent_rows_inserted_total{type="datadogv2"}`)
	rowsTenantInserted = tenantmetrics.NewCounterMap(`vmagent_tenant_inserted_rows_total{type="datadogv2"}`)
	rowsPerInsert      = metrics.NewHistogram(`vmagent_rows_per_insert{type="datadogv2"}`)
)

// InsertHandlerForHTTP processes remote write for DataDog POST /api/v2/series request.
//
// See https://docs.datadoghq.com/api/latest/metrics/#submit-metrics
func InsertHandlerForHTTP(at *auth.Token, req *http.Request) error {
	extraLabels, err := protoparserutil.GetExtraLabels(req)
	if err != nil {
		return err
	}
	ct := req.Header.Get("Content-Type")
	ce := req.Header.Get("Content-Encoding")
	return stream.Parse(req.Body, ce, ct, func(series []datadogv2.Series) error {
		return insertRows(at, series, extraLabels)
	})
}

func insertRows(at *auth.Token, series []datadogv2.Series, extraLabels []prompb.Label) error {
	ctx := common.GetPushCtx()
	defer common.PutPushCtx(ctx)

	rowsTotal := 0
	tssDst := ctx.WriteRequest.Timeseries[:0]
	labels := ctx.Labels[:0]
	samples := ctx.Samples[:0]
	for i := range series {
		ss := &series[i]
		rowsTotal += len(ss.Points)
		labelsLen := len(labels)
		labels = append(labels, prompb.Label{
			Name:  "__name__",
			Value: ss.Metric,
		})
		for _, rs := range ss.Resources {
			labels = append(labels, prompb.Label{
				Name:  rs.Type,
				Value: rs.Name,
			})
		}
		if ss.SourceTypeName != "" {
			labels = append(labels, prompb.Label{
				Name:  "source_type_name",
				Value: ss.SourceTypeName,
			})
		}
		for _, tag := range ss.Tags {
			name, value := datadogutil.SplitTag(tag)
			if name == "host" {
				name = "exported_host"
			}
			labels = append(labels, prompb.Label{
				Name:  name,
				Value: value,
			})
		}
		labels = append(labels, extraLabels...)
		samplesLen := len(samples)
		for _, pt := range ss.Points {
			samples = append(samples, prompb.Sample{
				Timestamp: pt.Timestamp * 1000,
				Value:     pt.Value,
			})
		}
		tssDst = append(tssDst, prompb.TimeSeries{
			Labels:  labels[labelsLen:],
			Samples: samples[samplesLen:],
		})
	}
	ctx.WriteRequest.Timeseries = tssDst
	ctx.Labels = labels
	ctx.Samples = samples
	if !remotewrite.TryPush(at, &ctx.WriteRequest) {
		return remotewrite.ErrQueueFullHTTPRetry
	}
	rowsInserted.Add(rowsTotal)
	if at != nil {
		rowsTenantInserted.Get(at).Add(rowsTotal)
	}
	rowsPerInsert.Update(float64(rowsTotal))
	return nil
}
