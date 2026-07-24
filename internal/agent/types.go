package agent

type AgentMode string

const (
	ModeChat    AgentMode = "chat"     
	ModeSimple  AgentMode = "simple"   
	ModePlanned AgentMode = "planned"  
	ModeDeep    AgentMode = "deep"     
)

type AgentContext struct {
	Mode       AgentMode
	Plan       *Plan
	Goal       string
	MaxRetries int
	TurnCount  int
}

type PlanStep struct {
	ID          string         `json:"id"`
	Description string         `json:"description"`
	Tool        string         `json:"tool"`
	Args        map[string]any `json:"args"`
	DependsOn   []string       `json:"depends_on"`
	Status      StepStatus     `json:"status"`
	Result      string         `json:"result"`
	RetryCount  int            `json:"retry_count"`
}

type StepStatus string

const (
	StepPending   StepStatus = "pending"
	StepRunning   StepStatus = "running"
	StepCompleted StepStatus = "completed"
	StepFailed    StepStatus = "failed"
	StepSkipped   StepStatus = "skipped"
)

type Plan struct {
	Goal       string     `json:"goal"`
	Steps      []PlanStep `json:"steps"`
	CurrentIdx int        `json:"current_idx"`
	Completed  bool       `json:"completed"`
}

type ErrorType int

const (
	ErrNoError          ErrorType = 0
	ErrCompilation      ErrorType = 1
	ErrCommandNotFound  ErrorType = 2
	ErrPermission       ErrorType = 3
	ErrFileNotFound     ErrorType = 4
	ErrPatternNotFound  ErrorType = 5
	ErrTimeout          ErrorType = 6
	ErrOOM              ErrorType = 7
	ErrUnknown          ErrorType = 99
)

type ReviewResult struct {
	Success     bool              `json:"success"`
	Issues      []string          `json:"issues"`
	ShouldRetry bool              `json:"should_retry"`
	Strategy    RetryStrategy     `json:"strategy"`
	Tool        string            `json:"tool,omitempty"`
	Args        map[string]any    `json:"args,omitempty"`
}

type RetryStrategy string

const (
	StrategyRetrySame   RetryStrategy = "retry_same"
	StrategyModifyArgs  RetryStrategy = "modify_args"
	StrategyAlternative RetryStrategy = "try_alternative"
	StrategyAbort       RetryStrategy = "abort"
)
