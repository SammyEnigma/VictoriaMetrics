package remotewrite

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"path"
	"strings"
	"sync"

	"github.com/golang/snappy"

	"github.com/VictoriaMetrics/VictoriaMetrics/lib/httputil"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/promauth"
	"github.com/VictoriaMetrics/VictoriaMetrics/lib/prompb"
)

// DebugClient won't push series periodically, but will write data to remote endpoint
// immediately when Push() is called
type DebugClient struct {
	addr string
	c    *http.Client

	wg sync.WaitGroup
}

// NewDebugClient initiates and returns a new DebugClient
func NewDebugClient() (*DebugClient, error) {
	if *addr == "" {
		return nil, nil
	}
	if err := httputil.CheckURL(*addr); err != nil {
		return nil, fmt.Errorf("invalid -remoteWrite.url: %w", err)
	}
	tr, err := promauth.NewTLSTransport(*tlsCertFile, *tlsKeyFile, *tlsCAFile, *tlsServerName, *tlsInsecureSkipVerify, "vmalert_remotewrite_debug")
	if err != nil {
		return nil, fmt.Errorf("failed to create transport for -remoteWrite.url=%q: %w", *addr, err)
	}
	c := &DebugClient{
		c: &http.Client{
			Timeout:   *sendTimeout,
			Transport: tr,
		},
		addr: strings.TrimSuffix(*addr, "/"),
	}
	return c, nil
}

// Push sends the given timeseries to the remote storage.
func (c *DebugClient) Push(s prompb.TimeSeries) error {
	c.wg.Add(1)
	defer c.wg.Done()
	wr := &prompb.WriteRequest{Timeseries: []prompb.TimeSeries{s}}
	data := wr.MarshalProtobuf(nil)

	return c.send(data)
}

// Close stops the DebugClient
func (c *DebugClient) Close() error {
	c.wg.Wait()
	return nil
}

func (c *DebugClient) send(data []byte) error {
	b := snappy.Encode(nil, data)
	r := bytes.NewReader(b)
	req, err := http.NewRequest(http.MethodPost, c.addr, r)
	if err != nil {
		return fmt.Errorf("failed to create new HTTP request: %w", err)
	}

	// RFC standard compliant headers
	req.Header.Set("Content-Encoding", "snappy")
	req.Header.Set("Content-Type", "application/x-protobuf")

	// Prometheus compliant headers
	req.Header.Set("X-Prometheus-Remote-Write-Version", "0.1.0")

	if !*disablePathAppend {
		req.URL.Path = path.Join(req.URL.Path, "/api/v1/write")
	}
	resp, err := c.c.Do(req)
	if err != nil {
		return fmt.Errorf("error while sending request to %s: %w; Data len %d(%d)",
			req.URL.Redacted(), err, len(data), r.Size())
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode/100 == 2 {
		return nil
	}
	body, _ := io.ReadAll(resp.Body)
	return fmt.Errorf("unexpected response code %d for %s. Response body %q",
		resp.StatusCode, req.URL.Redacted(), body)
}
