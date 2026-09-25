package reconciler

import (
	"context"

	"github.com/Fhuss/syl/pkg/card"
	"github.com/Fhuss/syl/pkg/watcher"
)

// MockReconciler delivers deterministic mutations for testing and demo runs.
type MockReconciler struct {
	turnIndex int
}

func NewMockReconciler() *MockReconciler {
	return &MockReconciler{turnIndex: 0}
}

func (m *MockReconciler) Reconcile(ctx context.Context, currentCards []*card.Card, turn *watcher.Turn) ([]Mutation, error) {
	m.turnIndex++

	switch m.turnIndex {
	case 1:
		// Turn 1: Initial multi-step procedure
		return []Mutation{
			{
				Op:     OpAdd,
				CardID: "proc-ecs-fargate",
				Type:   card.TypeProcedure,
				Title:  "Deploy Service to AWS ECS Fargate",
				Content: `### Prerequisites
Ensure AWS CLI is configured with admin credentials.

### Steps
1. Create an Amazon ECS cluster:
   ` + "```bash\naws ecs create-cluster --cluster-name app-cluster --region us-east-1\n```" + `

2. Register task definition:
   ` + "```bash\naws ecs register-task-definition --cli-input-json file://task-def.json --region us-east-1\n```" + `

3. Create the Fargate service:
   ` + "```bash\naws ecs create-service --cluster app-cluster --service-name web-svc --task-definition web-task:1 --desired-count 2 --launch-type FARGATE --region us-east-1\n```" + `
`,
				Reason: "Extracted ECS Fargate deployment procedure",
			},
		}, nil

	case 2:
		// Turn 2: User hit an IAM permission error on ECR
		return []Mutation{
			{
				Op:     OpUpdate,
				CardID: "proc-ecs-fargate",
				Type:   card.TypeProcedure,
				Title:  "Deploy Service to AWS ECS Fargate",
				Content: `### Prerequisites
Ensure AWS CLI is configured with admin credentials.

### Steps
1. Create an Amazon ECS cluster:
   ` + "```bash\naws ecs create-cluster --cluster-name app-cluster --region us-east-1\n```" + `

2. **[ADDED]** Attach ECR pull permissions to execution role:
   ` + "```bash\naws iam attach-role-policy --role-name ecsTaskExecutionRole --policy-arn arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly\n```" + `

3. Register task definition:
   ` + "```bash\naws ecs register-task-definition --cli-input-json file://task-def.json --region us-east-1\n```" + `

4. Create the Fargate service:
   ` + "```bash\naws ecs create-service --cluster app-cluster --service-name web-svc --task-definition web-task:1 --desired-count 2 --launch-type FARGATE --region us-east-1\n```" + `
`,
				Reason: "Inserted prerequisite step for ECR pull permissions after AccessDenied error",
			},
		}, nil

	case 3:
		// Turn 3: User changed region to eu-central-1
		return []Mutation{
			{
				Op:     OpUpdate,
				CardID: "proc-ecs-fargate",
				Type:   card.TypeProcedure,
				Title:  "Deploy Service to AWS ECS Fargate (Frankfurt)",
				Content: `### Prerequisites
Ensure AWS CLI is configured with admin credentials.

### Steps
1. Create an Amazon ECS cluster:
   ` + "```bash\naws ecs create-cluster --cluster-name app-cluster --region eu-central-1\n```" + `

2. Attach ECR pull permissions to execution role:
   ` + "```bash\naws iam attach-role-policy --role-name ecsTaskExecutionRole --policy-arn arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly\n```" + `

3. Register task definition:
   ` + "```bash\naws ecs register-task-definition --cli-input-json file://task-def.json --region eu-central-1\n```" + `

4. Create the Fargate service:
   ` + "```bash\naws ecs create-service --cluster app-cluster --service-name web-svc --task-definition web-task:1 --desired-count 2 --launch-type FARGATE --region eu-central-1\n```" + `
`,
				Reason: "Updated AWS region to eu-central-1 across all commands",
			},
			{
				Op:     OpAdd,
				CardID: "dec-target-region",
				Type:   card.TypeDecision,
				Title:  "Target AWS Region",
				Content: `**Decision**: Deploy workload to Frankfurt (` + "`eu-central-1`" + `) for GDPR compliance and lower European latency.`,
				Reason: "Recorded region architectural decision",
			},
		}, nil

	default:
		// Conversational filler -> NOOP
		return []Mutation{
			{
				Op:     OpNoop,
				Reason: "Conversational discussion without new or revised procedures",
			},
		}, nil
	}
}
