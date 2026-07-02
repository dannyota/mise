// Package temporal provides Temporal worker setup for ingest and detection workflows.
package temporal

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
	"go.temporal.io/sdk/workflow"
)

// defaultActivityTimeout bounds NoopActivity when the workflow has no
// execution timeout to inherit from (e.g. under the test suite, which leaves
// WorkflowExecutionTimeout unset).
const defaultActivityTimeout = 10 * time.Second

// Config holds Temporal connection parameters.
type Config struct {
	Host      string
	Namespace string
	TaskQueue string
}

// Connect opens a Temporal client.
func Connect(ctx context.Context, cfg Config) (client.Client, error) {
	c, err := client.DialContext(ctx, client.Options{
		HostPort:  cfg.Host,
		Namespace: cfg.Namespace,
	})
	if err != nil {
		return nil, fmt.Errorf("connecting to temporal: %w", err)
	}
	return c, nil
}

// NewWorker creates and configures a worker with the baseline registrations
// (NoopWorkflow/NoopActivity). Callers with pipeline dependencies should use
// NewWorkerWith to register their workflows and activities on top.
func NewWorker(c client.Client, taskQueue string) worker.Worker {
	return NewWorkerWith(c, taskQueue, nil)
}

// NewWorkerWith is NewWorker plus a registration hook: reg (when non-nil) is
// called with the configured worker so callers can register additional
// workflows and activities — e.g. the ingest pipeline — without this package
// importing them.
func NewWorkerWith(c client.Client, taskQueue string, reg func(worker.Worker)) worker.Worker {
	w := worker.New(c, taskQueue, worker.Options{})
	w.RegisterWorkflow(NoopWorkflow)
	w.RegisterActivity(NoopActivity)
	if reg != nil {
		reg(w)
	}
	return w
}

// NoopWorkflow is a trivial workflow that proves Temporal registration works.
func NoopWorkflow(ctx workflow.Context) error {
	ao := workflow.ActivityOptions{
		StartToCloseTimeout: workflow.GetInfo(ctx).WorkflowExecutionTimeout,
	}
	if ao.StartToCloseTimeout == 0 {
		ao.StartToCloseTimeout = defaultActivityTimeout
	}
	return workflow.ExecuteActivity(workflow.WithActivityOptions(ctx, ao), NoopActivity).Get(ctx, nil)
}

// NoopActivity is a trivial activity that does nothing.
func NoopActivity(ctx context.Context) error {
	slog.InfoContext(ctx, "noop activity executed")
	return nil
}
