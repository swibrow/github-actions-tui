package github

import (
	"context"
	"fmt"
	"sort"
	"time"

	"gopkg.in/yaml.v3"
)

// workflowFile represents the top-level structure of a GitHub Actions workflow YAML.
type workflowFile struct {
	On   workflowOn             `yaml:"on"`
	Jobs map[string]workflowJob `yaml:"jobs"`
}

type workflowOn struct {
	WorkflowDispatch *workflowDispatch `yaml:"workflow_dispatch"`
}

type workflowDispatch struct {
	Inputs map[string]workflowInputDef `yaml:"inputs"`
}

type workflowInputDef struct {
	Description string      `yaml:"description"`
	Required    bool        `yaml:"required"`
	Default     interface{} `yaml:"default"`
	Type        string      `yaml:"type"`
	Options     []string    `yaml:"options"`
}

type workflowJob struct {
	Needs interface{} `yaml:"needs"` // string or []string
}

// ParseJobDependencies parses a workflow YAML and returns a map of job name -> list of dependency job names.
func ParseJobDependencies(data []byte) (map[string][]string, error) {
	var wf workflowFile
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, err
	}

	deps := make(map[string][]string, len(wf.Jobs))
	for name, job := range wf.Jobs {
		switch v := job.Needs.(type) {
		case string:
			deps[name] = []string{v}
		case []interface{}:
			needs := make([]string, 0, len(v))
			for _, n := range v {
				if s, ok := n.(string); ok {
					needs = append(needs, s)
				}
			}
			deps[name] = needs
		default:
			deps[name] = nil
		}
	}
	return deps, nil
}

// FetchWorkflowYAML fetches a workflow file from the repository and parses its job dependencies.
func (c *Client) FetchWorkflowYAML(ctx context.Context, path string) (map[string][]string, error) {
	data, err := c.FetchFileContent(ctx, path)
	if err != nil {
		return nil, err
	}
	return ParseJobDependencies(data)
}

// ParseWorkflowInputs parses a workflow YAML and returns the workflow_dispatch inputs.
func ParseWorkflowInputs(data []byte) ([]WorkflowInput, error) {
	// The "on" key can be a string, list, or map. We need the map form.
	// Try parsing with the struct first; if that fails, there's no workflow_dispatch.
	var wf workflowFile
	if err := yaml.Unmarshal(data, &wf); err != nil {
		return nil, err
	}

	if wf.On.WorkflowDispatch == nil || len(wf.On.WorkflowDispatch.Inputs) == 0 {
		return nil, nil
	}

	inputs := make([]WorkflowInput, 0, len(wf.On.WorkflowDispatch.Inputs))
	for name, def := range wf.On.WorkflowDispatch.Inputs {
		defStr := ""
		if def.Default != nil {
			defStr = fmt.Sprintf("%v", def.Default)
		}
		typ := def.Type
		if typ == "" {
			typ = "string"
		}
		inputs = append(inputs, WorkflowInput{
			Name:        name,
			Description: def.Description,
			Required:    def.Required,
			Default:     defStr,
			Type:        typ,
			Options:     def.Options,
		})
	}

	// Sort by name for consistent ordering
	sort.Slice(inputs, func(i, j int) bool {
		return inputs[i].Name < inputs[j].Name
	})

	return inputs, nil
}

// InferJobDependencies infers job dependency tiers from start times.
// Jobs starting within 30 seconds of each other are considered parallel (same tier).
func InferJobDependencies(jobs []WorkflowJob) map[string][]string {
	if len(jobs) == 0 {
		return nil
	}

	// Sort by start time
	sorted := make([]WorkflowJob, len(jobs))
	copy(sorted, jobs)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].StartedAt.Before(sorted[j].StartedAt)
	})

	deps := make(map[string][]string, len(sorted))
	var prevTierNames []string
	var tierStart time.Time

	for i, job := range sorted {
		if i == 0 || job.StartedAt.IsZero() {
			tierStart = job.StartedAt
			deps[job.Name] = nil
			if i == 0 {
				prevTierNames = []string{job.Name}
			} else {
				prevTierNames = append(prevTierNames, job.Name)
			}
			continue
		}

		if job.StartedAt.Sub(tierStart) <= 30*time.Second {
			// Same tier as previous
			deps[job.Name] = nil
			if len(prevTierNames) > 0 && deps[prevTierNames[0]] == nil {
				// Still building the first tier
				prevTierNames = append(prevTierNames, job.Name)
			}
		} else {
			// New tier — depends on all jobs in previous tier
			deps[job.Name] = make([]string, len(prevTierNames))
			copy(deps[job.Name], prevTierNames)
			prevTierNames = []string{job.Name}
			tierStart = job.StartedAt
		}
	}

	return deps
}
