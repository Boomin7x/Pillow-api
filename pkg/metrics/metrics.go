package metrics

import "time"

type Metrics interface {
	RecordVerdict(checkType string, verdict string)
	RecordVendorLatency(provider string, d time.Duration)
	SetQueueDepth(n int)
	IncCircuitOpen(provider string)
}

var _ Metrics = Noop{}

type Noop struct{}

func NewNoop() Metrics {
	return Noop{}
}

func (Noop) RecordVerdict(string, string)              {}
func (Noop) RecordVendorLatency(string, time.Duration) {}
func (Noop) SetQueueDepth(int)                         {}
func (Noop) IncCircuitOpen(string)                     {}
