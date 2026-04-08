package outbound

import (
	"context"
	"time"

	"cerebron/internal/domain"
)

type AlertProvider interface {
	Name() string
	FetchAlerts(ctx context.Context, since time.Time) ([]domain.Event, error)
}

type DebugAlertProvider interface {
	AlertProvider
	FetchAlertsDebug(ctx context.Context, since time.Time) (AlertFetchResult, error)
}

type AlertFetchResult struct {
	Events []domain.Event
	Debug  AlertProviderDebug
}

type AlertProviderDebug struct {
	MonitorsFetched    int
	CandidateEvents    int
	IgnoredByStatus    int
	IgnoredBeforeSince int
	SampleMonitorNames []string
	SampleTags         []string
}

type LogProvider interface {
	Name() string
	SearchLogs(ctx context.Context, query domain.LogQuery) ([]domain.Event, error)
}

type ScmProvider interface {
	Name() string
	RecentChanges(ctx context.Context, query domain.CodeLookupQuery) ([]domain.CodeChange, error)
}

type ChatProvider interface {
	Name() string
	PublishIncident(ctx context.Context, message domain.ReportMessage) error
}

type LLMProvider interface {
	Name() string
	AnalyzeIncident(ctx context.Context, input domain.ReasoningInput) (domain.ReasoningOutput, error)
}
