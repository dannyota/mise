package temporal_test

import (
	"testing"

	"go.temporal.io/sdk/testsuite"

	mise_temporal "danny.vn/mise/pkg/temporal"
)

func TestNoopWorkflow(t *testing.T) {
	suite := &testsuite.WorkflowTestSuite{}
	env := suite.NewTestWorkflowEnvironment()

	env.RegisterWorkflow(mise_temporal.NoopWorkflow)
	env.RegisterActivity(mise_temporal.NoopActivity)

	env.ExecuteWorkflow(mise_temporal.NoopWorkflow)
	if !env.IsWorkflowCompleted() {
		t.Fatal("workflow did not complete")
	}
	if err := env.GetWorkflowError(); err != nil {
		t.Fatalf("workflow error: %v", err)
	}
}
