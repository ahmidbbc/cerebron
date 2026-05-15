package domain

import "time"

// DeploymentStatus represents the outcome of a deployment.
type DeploymentStatus string

const (
	DeploymentStatusSuccess    DeploymentStatus = "success"
	DeploymentStatusFailure    DeploymentStatus = "failure"
	DeploymentStatusInProgress DeploymentStatus = "in_progress"
	DeploymentStatusRolledBack DeploymentStatus = "rolled_back"
	DeploymentStatusUnknown    DeploymentStatus = "unknown"
)

// Deployment represents a deployment event from any CI/CD or infrastructure system.
type Deployment struct {
	ID          string
	Source      string
	Service     string
	Environment string
	Version     string
	Commit      string
	Author      string
	Branch      string
	Status      DeploymentStatus
	StartedAt   time.Time
	FinishedAt  time.Time
	URL         string
}
