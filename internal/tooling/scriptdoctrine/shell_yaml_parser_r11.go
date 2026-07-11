package scriptdoctrine

import (
	"fmt"
	"strings"

	yaml "go.yaml.in/yaml/v3"
)

// YAML AST workflow walker (R11).
//
// GitHub Actions workflows mix shell (`run:`) and reusable
// (`uses:`) steps. The shell that GitHub uses to execute a
// `run:` block is determined by the most specific setting:
//
//   - step-level `shell:` override
//   - else job-level `defaults.run.shell`
//   - else workflow-level `defaults.run.shell`
//   - else platform default (bash on Linux/macOS, pwsh on Windows)
//
// The verifier must use the YAML AST (not indentation heuristics)
// so workflow shell precedence is computed structurally. Custom
// shell templates like `python -u {0}` are recognised by their
// first whitespace-delimited word; templates whose first word is
// not the platform default must be parsed as a string and the
// command word extracted.
//
// R11.5 mandates:
//
//   - One usable document (we reject multi-document streams).
//   - Required mapping / sequence node kinds at known positions.
//   - Decoded scalar values via Node.Value rather than quote
//     stripping.
//   - Line/Column information surfaced for diagnostics.
//   - Alias resolution with a cycle guard.

// parseYAMLWorkflow decodes a workflow file into a yaml.Node tree
// and returns the root DocumentNode. It enforces the R11.5
// one-document contract: malformed YAML or multi-document streams
// surface as errors.
func parseYAMLWorkflow(data []byte) (*yaml.Node, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("malformed workflow YAML: %w", err)
	}
	if doc.Kind != yaml.DocumentNode {
		return nil, fmt.Errorf("workflow YAML: top-level is not a document (kind=%d)", doc.Kind)
	}
	if len(doc.Content) == 0 {
		return nil, fmt.Errorf("workflow YAML: empty document")
	}
	if len(doc.Content) > 1 {
		return nil, fmt.Errorf("workflow YAML: multiple documents found (count=%d)", len(doc.Content))
	}
	return &doc, nil
}

// resolveAliases walks node (a yaml.Node tree) and substitutes
// alias nodes with their target nodes. A cycle guard prevents
// unbounded recursion. Mapping/sequence alias replacement happens
// in-place so subsequent walk steps see the resolved values.
func resolveAliases(node *yaml.Node, aliasTable map[string]*yaml.Node, visiting map[string]bool, path string) (*yaml.Node, error) {
	if node == nil {
		return nil, nil
	}
	if node.Alias != nil {
		if visiting[node.Value] {
			return nil, fmt.Errorf("workflow YAML: alias cycle at %s -> %q", path, node.Value)
		}
		target, ok := aliasTable[node.Value]
		if !ok {
			return nil, fmt.Errorf("workflow YAML: unresolved alias %q at %s", node.Value, path)
		}
		visiting[node.Value] = true
		defer delete(visiting, node.Value)
		return resolveAliases(target, aliasTable, visiting, path+"->"+node.Value)
	}
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			k := node.Content[i]
			v := node.Content[i+1]
			resolved, err := resolveAliases(v, aliasTable, visiting, path+"/"+k.Value)
			if err != nil {
				return nil, err
			}
			node.Content[i+1] = resolved
		}
	case yaml.SequenceNode:
		for i, c := range node.Content {
			resolved, err := resolveAliases(c, aliasTable, visiting, fmt.Sprintf("%s[%d]", path, i))
			if err != nil {
				return nil, err
			}
			node.Content[i] = resolved
		}
	}
	return node, nil
}

// buildAliasTable returns a map of `*anchorName -> targetNode`.
// Anchors must point at scalar or mapping / sequence values.
func buildAliasTable(root *yaml.Node) map[string]*yaml.Node {
	out := make(map[string]*yaml.Node)
	var walk func(n *yaml.Node)
	walk = func(n *yaml.Node) {
		if n == nil {
			return
		}
		if n.Anchor != "" {
			out["*"+n.Anchor] = n
		}
		switch n.Kind {
		case yaml.MappingNode:
			for _, c := range n.Content {
				walk(c)
			}
		case yaml.SequenceNode:
			for _, c := range n.Content {
				walk(c)
			}
		}
	}
	walk(root)
	return out
}

// extractYAMLSteps decodes a workflow YAML AST and returns one
// WorkflowRunStep per `run:` step, with line/column information.
// `uses:` steps and any other step kind are skipped; the policy
// does not classify reusable actions.
func extractYAMLSteps(data []byte) ([]WorkflowRunStep, error) {
	root, err := parseYAMLWorkflow(data)
	if err != nil {
		return nil, err
	}
	document := root.Content[0]
	aliasTable := buildAliasTable(document)
	if _, err := resolveAliases(document, aliasTable, map[string]bool{}, "/"); err != nil {
		return nil, err
	}
	if document.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("workflow YAML: root must be a mapping (kind=%d)", document.Kind)
	}

	workflowDefaults, err := readShellFromDefaults(document)
	if err != nil {
		return nil, err
	}

	jobsNode := findMapValue(document, "jobs")
	if jobsNode == nil {
		// No jobs is a valid (idle) workflow; return empty.
		return nil, nil
	}
	if jobsNode.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("workflow YAML: jobs is not a mapping at line %d column %d", jobsNode.Line, jobsNode.Column)
	}

	var steps []WorkflowRunStep
	jobKeys := collectTopLevelKeys(jobsNode)
	for _, jobID := range jobKeys {
		jobVal := findMapValue(jobsNode, jobID)
		if jobVal == nil {
			continue
		}
		if jobVal.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("workflow YAML: job %q is not a mapping at line %d column %d", jobID, jobVal.Line, jobVal.Column)
		}
		jobDefaults, err := readShellFromDefaults(jobVal)
		if err != nil {
			return nil, err
		}
		stepsNode := findMapValue(jobVal, "steps")
		if stepsNode == nil {
			continue
		}
		if stepsNode.Kind != yaml.SequenceNode {
			return nil, fmt.Errorf("workflow YAML: steps for job %q is not a sequence at line %d column %d", jobID, stepsNode.Line, stepsNode.Column)
		}
		for idx, rawStep := range stepsNode.Content {
			if rawStep.Kind != yaml.MappingNode {
				return nil, fmt.Errorf("workflow YAML: step %d of job %q is not a mapping at line %d column %d", idx, jobID, rawStep.Line, rawStep.Column)
			}
			if findMapValue(rawStep, "uses") != nil {
				continue
			}
			runNode := findMapValue(rawStep, "run")
			if runNode == nil {
				continue
			}
			if runNode.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("workflow run field is not a scalar at job %q step %d line %d", jobID, idx, runNode.Line)
			}
			shellNode := findMapValue(rawStep, "shell")
			if shellNode != nil && shellNode.Kind != yaml.ScalarNode {
				return nil, fmt.Errorf("workflow shell field is not a scalar at job %q step %d line %d", jobID, idx, shellNode.Line)
			}
			stepShell := readShellFromMap(rawStep, "shell")
			step := WorkflowRunStep{
				JobID:         jobID,
				StepIndex:     idx,
				Run:           runNode.Value,
				StepShell:     stepShell,
				JobDefaults:   jobDefaults,
				WorkflowShell: workflowDefaults,
				Line:          runNode.Line,
				Column:        runNode.Column,
			}
			steps = append(steps, step)
		}
	}
	return steps, nil
}

// readShellFromShellReports scans a mapping for `defaults.run.shell`
// and returns either the shell value or an error when the value is
// present-but-not-a-scalar (e.g. a sequence). The verifier must
// surface sequence-typed shell values as hard errors rather than
// silently downgrade them to bash-default.
func readShellFromDefaults(node *yaml.Node) (string, error) {
	if node == nil || node.Kind != yaml.MappingNode {
		return "", nil
	}
	defaults := findMapValue(node, "defaults")
	if defaults == nil {
		return "", nil
	}
	if defaults.Kind != yaml.MappingNode {
		return "", fmt.Errorf("workflow YAML: defaults is not a mapping at line %d column %d", defaults.Line, defaults.Column)
	}
	runDefaults := findMapValue(defaults, "run")
	if runDefaults == nil {
		return "", nil
	}
	if runDefaults.Kind != yaml.MappingNode {
		return "", fmt.Errorf("workflow YAML: defaults.run is not a mapping at line %d column %d", runDefaults.Line, runDefaults.Column)
	}
	v := findMapValue(runDefaults, "shell")
	if v == nil {
		return "", nil
	}
	if v.Kind != yaml.ScalarNode {
		return "", fmt.Errorf("workflow YAML: defaults.run.shell is not a scalar at line %d column %d", v.Line, v.Column)
	}
	return v.Value, nil
}

// readShellFromDefaults scans a mapping for `defaults.run.shell`
// and returns the shell value (or "" if absent).

// readShellFromMap returns the value of the `shell:` key in a
// mapping, or "" if absent. The value is rejected with a ""
// fallback if it is not a scalar node - structural validation
// happens upstream.
func readShellFromMap(node *yaml.Node, key string) string {
	v := findMapValue(node, key)
	if v == nil || v.Kind != yaml.ScalarNode {
		return ""
	}
	return v.Value
}

// findMapValue returns the value at the given key in a mapping
// node. Returns nil if the key is missing or the node is not a
// mapping.
func findMapValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// collectTopLevelKeys returns the keys of a top-level mapping.
func collectTopLevelKeys(node *yaml.Node) []string {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	seen := make(map[string]bool, len(node.Content)/2)
	var keys []string
	for i := 0; i+1 < len(node.Content); i += 2 {
		k := node.Content[i].Value
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	return keys
}

// isPythonShell returns true when the effective shell for a step
// (computed as step -> job defaults -> workflow defaults) names a
// python interpreter. A dynamic template (one whose first token
// contains `$` or “) is fail-closed: the caller treats such a
// step as a hard internal error.
func isPythonShell(stepShell, jobDefaults, workflowShell string) bool {
	tpl, dynamic := effectiveShellTemplate(stepShell, jobDefaults, workflowShell)
	if dynamic {
		return false
	}
	exe, ok := shellTemplateExecutable(tpl)
	if !ok {
		return false
	}
	return isPythonCommandWord(exe)
}

// effectiveShellTemplate returns the most-specific shell template
// for a step. The second return is true when the most-specific
// template is dynamic (contains a parameter expansion or command
// substitution) and cannot be resolved statically; the verifier
// fails closed in that case.
func effectiveShellTemplate(stepShell, jobDefaults, workflowShell string) (string, bool) {
	for _, tpl := range []string{stepShell, jobDefaults, workflowShell} {
		if strings.TrimSpace(tpl) == "" {
			continue
		}
		if strings.ContainsAny(tpl, "$`") {
			return tpl, true
		}
		return tpl, false
	}
	return "", false
}

// classifyEffectiveShell returns the resolved effective shell
// status for a step. The boolean return values follow the R11.6
// closure contract: a dynamic template is treated as an explicit
// policy violation (errors are surfaced by the calling extractor)
// rather than silently ignored.
type effectiveShell struct {
	Template string
	Python   bool
	Dynamic  bool
}

func classifyEffectiveShell(stepShell, jobDefaults, workflowShell string) effectiveShell {
	tpl, dynamic := effectiveShellTemplate(stepShell, jobDefaults, workflowShell)
	if tpl == "" {
		return effectiveShell{Template: ""}
	}
	if dynamic {
		return effectiveShell{Template: tpl, Dynamic: true}
	}
	exe, ok := shellTemplateExecutable(tpl)
	if !ok {
		return effectiveShell{Template: tpl}
	}
	return effectiveShell{Template: tpl, Python: isPythonCommandWord(exe)}
}

// shellTemplateExecutable returns the first whitespace-delimited
// word of a custom shell template. If the template does not
// contain the GitHub `{0}` placeholder, the entire template is
// treated as the executable name (preserving the R9 fallback for
// `shell: bash` and `shell: python` plain scalars). The boolean
// return is false when the template cannot be parsed as a custom
// shell template at all.
//
// R11.6 implementation note: we split the template on the
// `{0}` placeholder rather than matching a single hand-crafted
// regex. This supports arbitrary flag ordering
// (`python -u {0}`, `bash --noprofile --norc -c {0}`,
// `/usr/bin/python3 {0} --flag`) without ambiguity.
func shellTemplateExecutable(tpl string) (string, bool) {
	tpl = strings.TrimSpace(tpl)
	if tpl == "" {
		return "", false
	}
	if !strings.Contains(tpl, "{0}") {
		return tpl, true
	}
	idx := strings.Index(tpl, "{0}")
	if idx <= 0 {
		return "", false
	}
	prefix := strings.TrimSpace(tpl[:idx])
	if prefix == "" {
		return "", false
	}
	fields := strings.Fields(prefix)
	if len(fields) == 0 {
		return "", false
	}
	return fields[0], true
}

// isDynamicShell returns true when the effective shell for a step
// is a dynamic GitHub Actions substitution (e.g. `shell: ${{
// matrix.shell }}`) that cannot be statically resolved. The
// verifier must surface these as hard errors per the R12 closure.
func isDynamicShell(stepShell, jobDefaults, workflowShell string) bool {
	return classifyEffectiveShell(stepShell, jobDefaults, workflowShell).Dynamic
}
