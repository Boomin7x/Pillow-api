package prommetrics

import (
	"time"

	"github.com/kodiahbertrand/pillow/pkg/metrics"
)

var _ metrics.Metrics = (*Prometheus)(nil)

type Prometheus struct{}

func NewPrometheus() *Prometheus {
	return &Prometheus{}
}

func (p *Prometheus) RecordVerdict(checkType string, verdict string) {}

func (p *Prometheus) RecordVendorLatency(provider string, d time.Duration) {}

func (p *Prometheus) SetQueueDepth(n int) {}

func (p *Prometheus) IncCircuitOpen(provider string) {}
