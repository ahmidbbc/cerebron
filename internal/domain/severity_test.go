package domain

import "testing"

func TestSeverityToScore(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		severity SignalSeverity
		expected float64
	}{
		{
			name:     "low",
			severity: SignalSeverityLow,
			expected: 0.33,
		},
		{
			name:     "medium",
			severity: SignalSeverityMedium,
			expected: 0.66,
		},
		{
			name:     "high",
			severity: SignalSeverityHigh,
			expected: 1.0,
		},
		{
			name:     "unknown",
			severity: "unknown",
			expected: 0.0,
		},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			actual := SeverityToScore(testCase.severity)
			if actual != testCase.expected {
				t.Fatalf("expected score %v for severity %q, got %v", testCase.expected, testCase.severity, actual)
			}
		})
	}
}

func TestSeverityToSignalSeverity(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		input    Severity
		expected SignalSeverity
	}{
		{name: "info maps to low", input: SeverityInfo, expected: SignalSeverityLow},
		{name: "warning maps to medium", input: SeverityWarning, expected: SignalSeverityMedium},
		{name: "alert maps to high", input: SeverityAlert, expected: SignalSeverityHigh},
		{name: "critical maps to high", input: SeverityCritical, expected: SignalSeverityHigh},
		{name: "unknown maps to empty", input: "unknown", expected: ""},
	}

	for _, testCase := range testCases {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()

			actual := SeverityToSignalSeverity(testCase.input)
			if actual != testCase.expected {
				t.Fatalf("expected %q, got %q", testCase.expected, actual)
			}
		})
	}
}
