package reporting

import "cerebron/internal/domain"

type Service struct {
	policy Policy
}

func NewService(policy Policy) Service {
	return Service{policy: policy}
}

func (s Service) ShouldPublish(events []domain.Event) bool {
	return s.policy.ShouldReport(events)
}

func (s Service) Score(events []domain.Event) int {
	return s.policy.Score(events)
}
