package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestEnrichAddsRequestAndCorrelationID(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, nil))

	ctx := WithRequestID(context.Background(), "req-123")
	ctx = WithCorrelationID(ctx, "corr-456")

	Enrich(base, ctx).InfoContext(ctx, "test event")

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("invalid JSON: %v — output: %s", err, buf.String())
	}
	if record["request_id"] != "req-123" {
		t.Errorf("expected request_id=req-123, got %v", record["request_id"])
	}
	if record["correlation_id"] != "corr-456" {
		t.Errorf("expected correlation_id=corr-456, got %v", record["correlation_id"])
	}
}

func TestEnrichOmitsFieldsWhenContextEmpty(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	base := slog.New(slog.NewJSONHandler(&buf, nil))

	Enrich(base, context.Background()).InfoContext(context.Background(), "test event")

	var record map[string]any
	if err := json.Unmarshal(buf.Bytes(), &record); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if _, ok := record["request_id"]; ok {
		t.Error("expected no request_id field when context is empty")
	}
	if _, ok := record["correlation_id"]; ok {
		t.Error("expected no correlation_id field when context is empty")
	}
}

func TestGenerateIDIsNonEmptyAndHex(t *testing.T) {
	t.Parallel()

	id := GenerateID()
	if len(id) != 16 {
		t.Errorf("expected 16 hex chars, got %d: %q", len(id), id)
	}
	for _, c := range id {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			t.Errorf("non-hex character %q in GenerateID output %q", c, id)
		}
	}
}
