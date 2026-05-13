package domain

import (
	"testing"
	"time"
)

func TestComputeFingerprint_Deterministic(t *testing.T) {
	a := IncidentAnalysis{
		Service:      "svc-a",
		ModelVersion: "v1",
		Groups: []SignalGroup{
			{
				Signals: []Signal{
					{Source: "datadog", Service: "svc-a", Summary: "high error rate", Severity: SignalSeverityHigh, Timestamp: time.Now()},
				},
			},
		},
	}
	fp1 := ComputeFingerprint(a)
	fp2 := ComputeFingerprint(a)
	if fp1 != fp2 {
		t.Fatalf("fingerprint not deterministic: %s != %s", fp1, fp2)
	}
	if fp1 == "" {
		t.Fatal("fingerprint must not be empty")
	}
}

func TestComputeFingerprint_SignalOrderIndependent(t *testing.T) {
	sig1 := Signal{Source: "datadog", Service: "svc-a", Summary: "error rate spike", Severity: SignalSeverityHigh, Timestamp: time.Now()}
	sig2 := Signal{Source: "elastic", Service: "svc-a", Summary: "slow queries", Severity: SignalSeverityMedium, Timestamp: time.Now()}

	a1 := IncidentAnalysis{Service: "svc-a", ModelVersion: "v1", Groups: []SignalGroup{{Signals: []Signal{sig1, sig2}}}}
	a2 := IncidentAnalysis{Service: "svc-a", ModelVersion: "v1", Groups: []SignalGroup{{Signals: []Signal{sig2, sig1}}}}

	if ComputeFingerprint(a1) != ComputeFingerprint(a2) {
		t.Fatal("fingerprint should be identical regardless of signal order")
	}
}

func TestComputeFingerprint_DifferentServices(t *testing.T) {
	base := IncidentAnalysis{
		ModelVersion: "v1",
		Groups: []SignalGroup{{Signals: []Signal{
			{Source: "datadog", Service: "svc-a", Summary: "error", Severity: SignalSeverityHigh, Timestamp: time.Now()},
		}}},
	}
	a := base
	a.Service = "svc-a"
	b := base
	b.Service = "svc-b"

	if ComputeFingerprint(a) == ComputeFingerprint(b) {
		t.Fatal("different services must produce different fingerprints")
	}
}
