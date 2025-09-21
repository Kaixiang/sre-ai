package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/example/sre-ai/internal/config"
	"github.com/example/sre-ai/internal/credentials"
	"github.com/example/sre-ai/internal/providers"
	"gopkg.in/yaml.v3"
)

// Workflow describes an agent workflow configuration.
type Workflow struct {
	Version     string                `yaml:"version"`
	Name        string                `yaml:"name"`
	Description string                `yaml:"description"`
	Agent       AgentSpec             `yaml:"agent"`
	Inputs      map[string]InputSpec  `yaml:"inputs"`
	Tools       map[string]ToolSpec   `yaml:"tools"`
	Workflow    WorkflowSpec          `yaml:"workflow"`
	Outputs     map[string]OutputSpec `yaml:"outputs"`
	Macros      map[string]MacroSpec  `yaml:"macros"`
}

// AgentSpec defines execution defaults for a workflow.
type AgentSpec struct {
	Model       string   `yaml:"model"`
	Provider    string   `yaml:"provider"`
	Temperature *float64 `yaml:"temperature"`
}

// InputSpec documents a required or optional workflow input.
type InputSpec struct {
	Type        string      `yaml:"type"`
	Description string      `yaml:"description"`
	Default     interface{} `yaml:"default"`
	Required    *bool       `yaml:"required"`
}

// ToolSpec registers a tool available to workflow steps.
type ToolSpec struct {
	Kind        string      `yaml:"kind"`
	Description string      `yaml:"description"`
	SampleFile  string      `yaml:"sample_file"`
	SampleData  interface{} `yaml:"sample_data"`
}

// WorkflowSpec contains the ordered stages to execute.
type WorkflowSpec struct {
	Stages []StageSpec `yaml:"stages"`
}

// StageSpec models a workflow stage.
type StageSpec struct {
	ID          string     `yaml:"id"`
	Kind        string     `yaml:"kind"`
	Description string     `yaml:"description"`
	Steps       []StepSpec `yaml:"steps"`
}

// StepSpec defines a single step inside a stage.
type StepSpec struct {
	Name        string                 `yaml:"name"`
	Type        string                 `yaml:"type"`
	Description string                 `yaml:"description"`
	Tool        string                 `yaml:"tool"`
	Template    string                 `yaml:"template"`
	Params      map[string]interface{} `yaml:"params"`
	Capture     map[string]string      `yaml:"capture"`
	Expect      ExpectSpec             `yaml:"expect"`
}

// ExpectSpec constrains the shape of a step result.
type ExpectSpec struct {
	Format string `yaml:"format"`
}

// OutputSpec describes a rendered workflow output.
type OutputSpec struct {
	Template string `yaml:"template"`
}

// MacroSpec provides reusable step sequences (unused in MVP but parsed).
type MacroSpec struct {
	Params []string          `yaml:"params"`
	Steps  []StepSpec        `yaml:"steps"`
	Notes  map[string]string `yaml:"notes"`
}

// Runner orchestrates workflow execution.
type Runner struct {
	workflow  *Workflow
	baseDir   string
	inputs    map[string]interface{}
	stepState map[string]map[string]interface{}
	opts      *config.GlobalOptions
}

// StepResult captures the outcome of a single executed (or planned) step.
type StepResult struct {
	StageID  string      `json:"stage"`
	StepName string      `json:"step"`
	Type     string      `json:"type"`
	Status   string      `json:"status"`
	Details  string      `json:"details,omitempty"`
	Output   interface{} `json:"output,omitempty"`
	Error    string      `json:"error,omitempty"`
}

// Result is returned by a workflow execution.
type Result struct {
	Workflow    string                 `json:"workflow"`
	Description string                 `json:"description,omitempty"`
	PlanOnly    bool                   `json:"plan_only"`
	Inputs      map[string]interface{} `json:"inputs"`
	Steps       []StepResult           `json:"steps"`
	Outputs     map[string]interface{} `json:"outputs,omitempty"`
}

// LoadWorkflow parses a workflow file and returns the structured representation.
func LoadWorkflow(path string) (*Workflow, string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, "", err
	}

	var wf Workflow
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, "", err
	}

	baseDir := filepath.Dir(path)
	return &wf, baseDir, nil
}

// NewRunner loads the workflow and prepares it for execution.
func NewRunner(workflowPath string, opts *config.GlobalOptions, provided map[string]string) (*Runner, error) {
	wf, baseDir, err := LoadWorkflow(workflowPath)
	if err != nil {
		return nil, err
	}

	inputs, err := resolveInputs(wf.Inputs, provided)
	if err != nil {
		return nil, err
	}

	return &Runner{
		workflow:  wf,
		baseDir:   baseDir,
		inputs:    inputs,
		stepState: make(map[string]map[string]interface{}),
		opts:      opts,
	}, nil
}

// Execute runs the workflow and returns a structured result.
func (r *Runner) Execute(ctx context.Context, planOnly bool) (*Result, error) {
	res := &Result{
		Workflow:    r.workflow.Name,
		Description: r.workflow.Description,
		PlanOnly:    planOnly,
		Inputs:      r.inputs,
		Steps:       make([]StepResult, 0),
	}

	for _, stage := range r.workflow.Workflow.Stages {
		for idx, step := range stage.Steps {
			stepName := step.Name
			if stepName == "" {
				stepName = fmt.Sprintf("%s_step_%d", stage.ID, idx+1)
			}

			sr := StepResult{
				StageID:  stage.ID,
				StepName: stepName,
				Type:     step.Type,
				Status:   "planned",
				Details:  step.Description,
			}

			if planOnly {
				res.Steps = append(res.Steps, sr)
				continue
			}

			output, err := r.executeStep(ctx, stage, stepName, step)
			if err != nil {
				sr.Status = "error"
				sr.Error = err.Error()
				res.Steps = append(res.Steps, sr)
				return res, err
			}

			sr.Status = "ok"
			sr.Output = output
			res.Steps = append(res.Steps, sr)
		}
	}

	if !planOnly {
		outs, err := r.renderOutputs()
		if err != nil {
			return res, err
		}
		res.Outputs = outs
	}

	return res, nil
}

func (r *Runner) executeStep(ctx context.Context, stage StageSpec, stepName string, step StepSpec) (map[string]interface{}, error) {
	renderedParams, err := r.renderParams(step.Params)
	if err != nil {
		return nil, err
	}

	var result map[string]interface{}
	var stepErr error

	if len(renderedParams) > 0 {
		if _, ok := r.stepState[stepName]; !ok {
			r.stepState[stepName] = make(map[string]interface{})
		}
		r.stepState[stepName]["params"] = renderedParams
	}

	switch strings.ToLower(step.Type) {
	case "tool":
		result, stepErr = r.executeTool(step, renderedParams)
	case "prompt":
		result, stepErr = r.executePrompt(ctx, step, renderedParams)
	default:
		stepErr = fmt.Errorf("unsupported step type %s", step.Type)
	}

	if stepErr != nil {
		return nil, stepErr
	}

	if len(step.Capture) > 0 {
		if _, ok := r.stepState[stepName]; !ok {
			r.stepState[stepName] = make(map[string]interface{})
		}
		for key, source := range step.Capture {
			if source == "" || source == "result" || source == "*" {
				r.stepState[stepName][key] = result
				continue
			}
			r.stepState[stepName][key] = lookupValue(result, source)
		}
	}

	if _, ok := r.stepState[stepName]; !ok {
		r.stepState[stepName] = make(map[string]interface{})
	}
	r.stepState[stepName]["_raw"] = result

	return result, nil
}

func (r *Runner) executeTool(step StepSpec, params map[string]interface{}) (map[string]interface{}, error) {
	toolName := step.Tool
	spec, ok := r.workflow.Tools[toolName]
	if !ok {
		return nil, fmt.Errorf("tool %s is not defined", toolName)
	}

	switch strings.ToLower(spec.Kind) {
	case "mock", "sample":
		data, err := r.resolveSampleData(spec)
		if err != nil {
			return nil, err
		}
		return map[string]interface{}{"data": data}, nil
	default:
		return nil, fmt.Errorf("tool kind %s not yet supported", spec.Kind)
	}
}

func (r *Runner) resolveSampleData(spec ToolSpec) (interface{}, error) {
	if spec.SampleData != nil {
		return spec.SampleData, nil
	}
	if spec.SampleFile == "" {
		return nil, errors.New("sample tool requires sample_data or sample_file")
	}

	path := spec.SampleFile
	if !filepath.IsAbs(path) {
		path = filepath.Join(r.baseDir, path)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var parsed interface{}
	if err := json.Unmarshal(data, &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func (r *Runner) executePrompt(ctx context.Context, step StepSpec, params map[string]interface{}) (map[string]interface{}, error) {
	prompt, err := r.renderTemplate(step.Template)
	if err != nil {
		return nil, err
	}

	model := r.workflow.Agent.Model
	if model == "" {
		model = r.opts.Model
	}
	if model == "" {
		model = providers.DefaultGeminiModel()
	}

	provider := strings.ToLower(r.workflow.Agent.Provider)
	if provider == "" {
		provider = strings.ToLower(r.opts.Provider)
	}
	if provider == "" {
		provider = "gemini"
	}

	switch provider {
	case "gemini":
		apiKey, err := credentials.LoadGeminiKey()
		if err != nil {
			return nil, err
		}
		client := providers.NewGeminiClient(apiKey, model)
		text, err := client.Generate(ctx, prompt)
		if err != nil {
			return nil, err
		}

		payload := map[string]interface{}{"text": text}
		// strip code fence if it's a ```json block
		trimmed := strings.TrimSpace(text)
		if strings.HasPrefix(strings.ToLower(trimmed), "```json") {
			// drop the leading fence line
			if i := strings.Index(trimmed, "\n"); i != -1 {
				trimmed = trimmed[i+1:]
			} else {
				trimmed = strings.TrimPrefix(trimmed, "```json")
			}
			// remove trailing fence if present
			if j := strings.LastIndex(trimmed, "```"); j != -1 {
				trimmed = trimmed[:j]
			}
			text = strings.TrimSpace(trimmed)
		}

		if strings.EqualFold(step.Expect.Format, "json") {
			var decoded interface{}
			if err := json.Unmarshal([]byte(text), &decoded); err != nil {
				return nil, fmt.Errorf("expected json response but decode failed: %w", err)
			}
			payload["json"] = decoded
		}
		return payload, nil
	default:
		return nil, fmt.Errorf("provider %s not supported for prompts", provider)
	}
}

func (r *Runner) renderOutputs() (map[string]interface{}, error) {
	if len(r.workflow.Outputs) == 0 {
		return nil, nil
	}
	outputs := make(map[string]interface{})
	for key, spec := range r.workflow.Outputs {
		rendered, err := r.renderTemplate(spec.Template)
		if err != nil {
			return nil, fmt.Errorf("render output %s: %w", key, err)
		}
		outputs[key] = rendered
	}
	return outputs, nil
}

func resolveInputs(specs map[string]InputSpec, provided map[string]string) (map[string]interface{}, error) {
	resolved := make(map[string]interface{})

	for key, spec := range specs {
		val, ok := provided[key]
		if ok {
			resolved[key] = val
			continue
		}
		if spec.Default != nil {
			resolved[key] = spec.Default
			continue
		}
		required := true
		if spec.Required != nil {
			required = *spec.Required
		}
		if required {
			return nil, fmt.Errorf("missing required input %s", key)
		}
	}

	for key, value := range provided {
		if _, ok := resolved[key]; !ok {
			resolved[key] = value
		}
	}

	return resolved, nil
}

func (r *Runner) renderTemplate(body string) (string, error) {
	tmpl, err := template.New("workflow").Funcs(template.FuncMap{
		"toJSON": func(v interface{}) string {
			b, _ := json.MarshalIndent(v, "", "  ")
			return string(b)
		},
	}).Parse(body)
	if err != nil {
		return "", err
	}

	data := map[string]interface{}{
		"inputs": r.inputs,
		"steps":  r.stepState,
	}

	var buf strings.Builder
	if err := tmpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func lookupValue(container map[string]interface{}, path string) interface{} {
	if container == nil {
		return nil
	}
	parts := strings.Split(path, ".")
	var current interface{} = container
	for _, part := range parts {
		switch typed := current.(type) {
		case map[string]interface{}:
			current = typed[part]
		default:
			return nil
		}
	}
	return current
}

// ParseInputPairs converts key=value slices into a map.
func ParseInputPairs(pairs []string) (map[string]string, error) {
	result := make(map[string]string)
	for _, pair := range pairs {
		if !strings.Contains(pair, "=") {
			return nil, fmt.Errorf("invalid input %q, expected key=value", pair)
		}
		parts := strings.SplitN(pair, "=", 2)
		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])
		if key == "" {
			return nil, fmt.Errorf("invalid input %q, empty key", pair)
		}
		result[key] = value
	}
	return result, nil
}

// Inputs returns the resolved inputs for this runner.
func (r *Runner) Inputs() map[string]interface{} {
	return r.inputs
}

// StepState exposes captured step data for testing or downstream use.
func (r *Runner) StepState() map[string]map[string]interface{} {
	return r.stepState
}

// WorkflowMeta returns the underlying workflow metadata.
func (r *Runner) WorkflowMeta() *Workflow {
	return r.workflow
}
func (r *Runner) renderParams(params map[string]interface{}) (map[string]interface{}, error) {
	if params == nil {
		return map[string]interface{}{}, nil
	}
	rendered := make(map[string]interface{}, len(params))
	for key, value := range params {
		rv, err := r.renderValue(value)
		if err != nil {
			return nil, err
		}
		rendered[key] = rv
	}
	return rendered, nil
}

func (r *Runner) renderValue(value interface{}) (interface{}, error) {
	switch typed := value.(type) {
	case string:
		return r.renderTemplate(typed)
	case []interface{}:
		out := make([]interface{}, len(typed))
		for i, elem := range typed {
			rv, err := r.renderValue(elem)
			if err != nil {
				return nil, err
			}
			out[i] = rv
		}
		return out, nil
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for k, v := range typed {
			rv, err := r.renderValue(v)
			if err != nil {
				return nil, err
			}
			out[k] = rv
		}
		return out, nil
	default:
		return value, nil
	}
}
