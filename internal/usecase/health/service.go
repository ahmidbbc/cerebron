package health

import "context"

type Service struct{}

func NewService() Service {
	return Service{}
}

func (Service) Liveness(context.Context) error {
	return nil
}

func (Service) Readiness(context.Context) error {
	return nil
}
