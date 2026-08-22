// Package swarm implements the Reasonix 3.0 swarm core runtime: a real,
// provider-agnostic orchestrator over the existing Reasonix agent turn
// primitive. It owns task decomposition, dependency-aware scheduling with
// bounded parallelism, worker execution through agent.Agent, structured shared
// state, result integration, task-specific verification, budget/cancel/
// recovery, and typed swarm events.
package swarm

import "time"

// Status is the lifecycle of the whole swarm run.
type Status string

const (
	StatusPending     Status = "pending"
	StatusPlanning    Status = "planning"
	StatusRunning     Status = "running"
	StatusIntegrating Status = "integrating"
	StatusDone        Status = "done"
	StatusFailed      Status = "failed"
	StatusCancelled   Status = "cancelled"
)

// TaskStatus is the authoritative lifecycle of one task in the task graph.
type TaskStatus string

const (
	TaskPending   TaskStatus = "pending"
	TaskReady     TaskStatus = "ready"
	TaskRunning   TaskStatus = "running"
	TaskWaiting   TaskStatus = "waiting"
	TaskSucceeded TaskStatus = "succeeded"
	TaskFailed    TaskStatus = "failed"
	TaskCancelled TaskStatus = "cancelled"
)

// FailureClass models the failure taxonomy the swarm must distinguish.
type FailureClass string

const (
	FailureTemporary     FailureClass = "temporary"
	FailurePermanent     FailureClass = "permanent"
	FailureApprovalWait  FailureClass = "approval_wait"
	FailureToolMissing   FailureClass = "tool_missing"
	FailureSchemaError   FailureClass = "schema_error"
	FailureProviderError FailureClass = "provider_error"
	FailureBudgetStop    FailureClass = "budget_stop"
	FailureTimeout       FailureClass = "timeout"
	FailureNoProgress    FailureClass = "no_progress"
	FailureDependency    FailureClass = "dependency_failure"
	FailureMergeConflict FailureClass = "merge_conflict"
	FailureCancelled     FailureClass = "cancelled"
)

// EvidenceKind names a required, host-verifiable piece of task evidence.
type EvidenceKind string

const (
	EvidenceDiff         EvidenceKind = "diff"
	EvidenceCompile      EvidenceKind = "compile"
	EvidenceUnitTest     EvidenceKind = "unit_test"
	EvidenceRuntime      EvidenceKind = "runtime"
	EvidenceProvider     EvidenceKind = "provider"
	EvidenceReadback     EvidenceKind = "readback"
	EvidenceArtifactHash EvidenceKind = "artifact_hash"
)

// FindingKind classifies an entry in the structured shared state.
type FindingKind string

const (
	FindingDecision   FindingKind = "decision"
	FindingFinding    FindingKind = "finding"
	FindingArtifact   FindingKind = "artifact"
	FindingPatch      FindingKind = "patch"
	FindingEvidence   FindingKind = "evidence"
	FindingError      FindingKind = "error"
	FindingTestResult FindingKind = "test_result"
	FindingReference  FindingKind = "reference"
)

// Finding is one structured exchange in the shared state.
type Finding struct {
	ID       string      `json:"id,omitempty"`
	TaskID   string      `json:"taskId,omitempty"`
	Kind     FindingKind `json:"kind,omitempty"`
	Summary  string      `json:"summary,omitempty"`
	Ref      string      `json:"ref,omitempty"`
	Verified bool        `json:"verified,omitempty"`
}

// Artifact is a produced file/patch with a content hash for verification.
type Artifact struct {
	Path   string `json:"path,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Bytes  int64  `json:"bytes,omitempty"`
	Kind   string `json:"kind,omitempty"`
}

// Evidence is one observed verification receipt.
type Evidence struct {
	Kind   EvidenceKind `json:"kind,omitempty"`
	Result string       `json:"result,omitempty"`
	Ref    string       `json:"ref,omitempty"`
	At     time.Time    `json:"at,omitempty"`
}

// TestResult is a bounded unit/integration test receipt.
type TestResult struct {
	Name   string `json:"name,omitempty"`
	Pass   bool   `json:"pass"`
	Output string `json:"output,omitempty"`
}

// TaskResult is the structured outcome of one worker task. It is not a raw
// transcript: transcripts stay in the worker's own session; only structured
// results are shared.
type TaskResult struct {
	Summary   string       `json:"summary,omitempty"`
	Findings  []Finding    `json:"findings,omitempty"`
	Artifacts []Artifact   `json:"artifacts,omitempty"`
	Evidence  []Evidence   `json:"evidence,omitempty"`
	Tests     []TestResult `json:"tests,omitempty"`
}

// TaskFailure records how and why a task stopped.
type TaskFailure struct {
	Class     FailureClass `json:"class,omitempty"`
	Message   string       `json:"message,omitempty"`
	Retryable bool         `json:"retryable,omitempty"`
	Retries   int          `json:"retries,omitempty"`
	At        time.Time    `json:"at,omitempty"`
}

// Task is one node in the structured task graph. Its state is authoritative;
// worker transcripts are not.
type Task struct {
	ID               string         `json:"id,omitempty"`
	Objective        string         `json:"objective,omitempty"`
	Status           TaskStatus     `json:"status,omitempty"`
	Dependencies     []string       `json:"dependencies,omitempty"`
	Profile          string         `json:"profile,omitempty"`
	Model            string         `json:"model,omitempty"`
	Provider         string         `json:"provider,omitempty"`
	Scope            []string       `json:"scope,omitempty"`
	RequiredEvidence []EvidenceKind `json:"requiredEvidence,omitempty"`
	Result           *TaskResult    `json:"result,omitempty"`
	Failure          *TaskFailure   `json:"failure,omitempty"`
	CreatedAt        time.Time      `json:"createdAt,omitempty"`
	StartedAt        time.Time      `json:"startedAt,omitempty"`
	FinishedAt       time.Time      `json:"finishedAt,omitempty"`
	UpdatedAt        time.Time      `json:"updatedAt,omitempty"`
	WorkerID         string         `json:"workerId,omitempty"`
	Attempts         int            `json:"attempts,omitempty"`
}

// Profile carries the configurable worker behavior for one task. The swarm
// never depends on specific profile names.
type Profile struct {
	Name             string         `json:"name,omitempty"`
	Instructions     string         `json:"instructions,omitempty"`
	AllowedTools     []string       `json:"allowedTools,omitempty"`
	ModelPreference  string         `json:"modelPreference,omitempty"`
	ContextWindow    int            `json:"contextWindow,omitempty"`
	MaxSteps         int            `json:"maxSteps,omitempty"`
	Timeout          time.Duration  `json:"timeout,omitempty"`
	BudgetCost       float64        `json:"budgetCost,omitempty"`
	BudgetTokens     int            `json:"budgetTokens,omitempty"`
	RequiredEvidence []EvidenceKind `json:"requiredEvidence,omitempty"`
	// ReadOnly forces the native host-side read-only execution boundary for
	// this worker: mutating tools and mutating bash commands are blocked at
	// dispatch even if they are registered.
	ReadOnly bool `json:"readOnly,omitempty"`
}

// Plan is the orchestrator's decomposition decision.
type Plan struct {
	Objective      string         `json:"objective,omitempty"`
	Tasks          []*Task        `json:"tasks,omitempty"`
	Concurrency    int            `json:"concurrency,omitempty"`
	ProviderLimits map[string]int `json:"providerLimits,omitempty"`
}

// SwarmState is the durable, authoritative swarm state. Only structured
// content is persisted: no raw transcripts, prompts, or hidden reasoning.
type SwarmState struct {
	ID         string           `json:"id,omitempty"`
	Objective  string           `json:"objective,omitempty"`
	Status     Status           `json:"status,omitempty"`
	Tasks      map[string]*Task `json:"tasks,omitempty"`
	Findings   []Finding        `json:"findings,omitempty"`
	CreatedAt  time.Time        `json:"createdAt,omitempty"`
	UpdatedAt  time.Time        `json:"updatedAt,omitempty"`
	FinishedAt time.Time        `json:"finishedAt,omitempty"`
	Budget     BudgetState      `json:"budget,omitempty"`
	Result     string           `json:"result,omitempty"`
	Failures   []TaskFailure    `json:"failures,omitempty"`
	Verified   bool             `json:"verified,omitempty"`
	Failed     bool             `json:"failed,omitempty"`
}
