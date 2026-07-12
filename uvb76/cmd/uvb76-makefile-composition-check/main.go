// ACT-UVB76-HULK05R4R1A-R2: recipe-aware Makefile composition verifier.
//
// GNU Make permits multiple rules per target; prerequisites from every
// rule are merged, and at most one rule supplies the recipe. The
// previous version of this tool only matched top-level target lines
// and counted each one as a recipe-bearing definition, which
// misclassified legitimate prerequisite-only duplicates as
// "duplicate recipes" and applied shell-parsing to the wrong line.
//
// This implementation models each rule as a RuleDefinition and asserts:
//
//   - exactly one rule per gate target has len(RecipeLines) > 0;
//   - any number of prerequisite-only rules are accepted (the
//     deferred-prereq pattern is legal GNU Make);
//   - the secret target's effective prerequisite set contains the
//     producer target;
//   - every actual recipe shell line parses (quotes balanced, no
//     dangling operator);
//   - make -n secret-target reaches producer BEFORE secret banners;
//   - make -n aggregate-gate reaches producer BEFORE secret banners.
//
// Every assertion has at least one mutation test in
// makefile_composition_check_test.go.
package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

const (
	producerTarget  = "hulk-uvb76-artifact-producer-gate"
	secretTarget    = "hulk-uvb76-artifact-secret-gate"
	aggregateTarget = "gate"
)

// RuleDefinition is one Makefile rule for a target.
//
// GNU Make permits multiple rules per target. The first rule usually
// defines prerequisites; at most one rule may provide the recipe
// (TAB-indented shell lines). All prerequisites across rules merge.
//
// Reference:
// https://www.gnu.org/software/make/manual/html_node/Multiple-Rules.html
type RuleDefinition struct {
	Target        string
	Prerequisites []string
	RecipeLines   []string // TAB-trimmed; empty when this rule is prereq-only
	DefLine       int      // 1-based source line of the target definition
}

// targetDefRe matches a top-level target definition of the form
//   target: deps
// where target is an identifier. It rejects lines that begin with TAB
// (those are recipe lines, not rules), with "." (directives), or with
// "#" (comments).
var targetDefRe = regexp.MustCompile(`^([A-Za-z0-9_./-]+):(\s|$)`)

func main() {
	makefile := flag.String("makefile", "Makefile", "path to the repository Makefile")
	repoRoot := flag.String("repo", ".", "repository root for the make command")
	flag.Parse()

	content, err := os.ReadFile(*makefile)
	if err != nil {
		fail("read Makefile: %v", err)
	}
	c := string(content)

	producerRules := parseRules(c, producerTarget)
	secretRules := parseRules(c, secretTarget)

	var errs []string
	errs = append(errs, checkRecipeRuleCount(producerTarget, producerRules)...)
	errs = append(errs, checkRecipeRuleCount(secretTarget, secretRules)...)

	if !hasWord(strings.Join(effectivePrereqs(secretRules), " "), producerTarget) {
		errs = append(errs, fmt.Sprintf(
			"secret target %q must depend on %q; effective deps=%v",
			secretTarget, producerTarget, effectivePrereqs(secretRules)))
	}

	errs = append(errs, checkRecipeShellLines(producerRules)...)
	errs = append(errs, checkRecipeShellLines(secretRules)...)

	if err := verifyDryRunOrdering(*repoRoot); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		fmt.Fprintln(os.Stderr, "FAIL: Makefile composition is invalid")
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, "  - "+e)
		}
		os.Exit(1)
	}
	fmt.Println("PASS: Makefile composition is valid")
}

// parseRules returns every rule definition for target in source order.
// A rule is a top-level non-comment line matching targetDefRe whose
// target name equals target. TAB-indented lines that immediately follow
// are attached as the rule's recipe.
func parseRules(content, target string) []RuleDefinition {
	lines := strings.Split(content, "\n")
	var rules []RuleDefinition
	for i, line := range lines {
		if isCommentOrDirective(line) {
			continue
		}
		m := targetDefRe.FindStringSubmatch(line)
		if m == nil || m[1] != target {
			continue
		}
		if strings.Contains(line, "=") {
			continue // recursive variable assignment
		}
		prereqs := parsePrereqs(line[len(m[0]):])
		recipe := collectRecipeLines(lines, i+1)
		rules = append(rules, RuleDefinition{
			Target:        target,
			Prerequisites: prereqs,
			RecipeLines:   recipe,
			DefLine:       i + 1,
		})
	}
	return rules
}

func isCommentOrDirective(line string) bool {
	if line == "" {
		return true
	}
	if strings.HasPrefix(line, "#") {
		return true
	}
	if strings.HasPrefix(line, ".") {
		return true
	}
	return false
}

// parsePrereqs tokenises the dependency substring of a rule definition.
// Line continuations (\) are skipped; everything else is a token.
func parsePrereqs(rest string) []string {
	rest = strings.TrimSpace(rest)
	if rest == "" {
		return nil
	}
	var out []string
	for _, f := range strings.Fields(rest) {
		if f == "\\" {
			continue
		}
		out = append(out, f)
	}
	return out
}

// collectRecipeLines returns TAB-indented lines immediately following
// the rule definition at lines[defIdx]. A blank line or non-TAB line
// terminates the recipe (matches GNU Make's rule).
func collectRecipeLines(lines []string, defIdx int) []string {
	var recipe []string
	for j := defIdx; j < len(lines); j++ {
		line := lines[j]
		if strings.HasPrefix(line, "\t") {
			recipe = append(recipe, strings.TrimPrefix(line, "\t"))
			continue
		}
		break
	}
	return recipe
}

// effectivePrereqs returns the union of prerequisites across all rules
// for the target, in first-seen order. GNU Make merges prerequisites
// across multiple rules.
func effectivePrereqs(rules []RuleDefinition) []string {
	seen := map[string]bool{}
	var out []string
	for _, r := range rules {
		for _, p := range r.Prerequisites {
			if seen[p] {
				continue
			}
			seen[p] = true
			out = append(out, p)
		}
	}
	return out
}

func hasWord(s, word string) bool {
	for _, w := range strings.Fields(s) {
		if w == word {
			return true
		}
	}
	return false
}

// checkRecipeRuleCount asserts exactly one rule supplies a recipe.
// Prerequisite-only extra rules are explicitly accepted (GNU Make
// semantics). Failing this check means two rules competed for the
// recipe, which GNU Make resolves by using the second — almost
// certainly a mistake.
func checkRecipeRuleCount(target string, rules []RuleDefinition) []string {
	var errs []string
	recipeCount := 0
	for _, r := range rules {
		if len(r.RecipeLines) > 0 {
			recipeCount++
		}
	}
	if recipeCount != 1 {
		errs = append(errs, fmt.Sprintf(
			"target %q has %d recipe-bearing rule(s) (expected exactly 1); total rules=%d",
			target, recipeCount, len(rules)))
	}
	return errs
}

// checkRecipeShellLines validates the syntactic shape of every actual
// recipe line. Blank lines inside recipes are allowed (Make ignores
// them); we only check shell lines.
func checkRecipeShellLines(rules []RuleDefinition) []string {
	var errs []string
	for _, r := range rules {
		for idx, line := range r.RecipeLines {
			trimmed := strings.TrimSpace(line)
			if trimmed == "" {
				continue
			}
			if err := shellLineParses(line); err != nil {
				errs = append(errs, fmt.Sprintf(
					"target %q recipe at line %d (step %d) failed shell parse: %v: %q",
					r.Target, r.DefLine+1+idx, idx+1, err, line))
			}
		}
	}
	return errs
}

func fail(format string, a ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", a...)
	os.Exit(1)
}

// shellLineParses returns nil if the line is a syntactically valid
// shell command line. We do NOT execute shell; we only check quote
// pairing and trailing-operator dangling.
func shellLineParses(line string) error {
	s := line
	inSingle, inDouble := false, false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !inSingle && !inDouble && c == '#' {
			break
		}
		if !inDouble && c == '\'' && (i == 0 || s[i-1] != '\\') {
			inSingle = !inSingle
		} else if !inSingle && c == '"' && (i == 0 || s[i-1] != '\\') {
			inDouble = !inDouble
		}
	}
	if inSingle {
		return fmt.Errorf("unterminated single quote")
	}
	if inDouble {
		return fmt.Errorf("unterminated double quote")
	}
	stripped := strings.TrimRight(s, " \t")
	if strings.HasSuffix(stripped, "|") || strings.HasSuffix(stripped, "&&") || strings.HasSuffix(stripped, "||") {
		return fmt.Errorf("dangling operator")
	}
	return nil
}

// verifyDryRunOrdering checks:
//   - make -n hulk-uvb76-artifact-secret-gate succeeds and reaches
//     producer before secret banners;
//   - make -n gate reaches producer before secret banners.
func verifyDryRunOrdering(repoRoot string) error {
	secret, err := makeDryRun(repoRoot, secretTarget)
	if err != nil {
		return fmt.Errorf("make -n %s failed: %v", secretTarget, err)
	}
	aggregate, err := makeDryRun(repoRoot, aggregateTarget)
	if err != nil {
		return fmt.Errorf("make -n %s failed: %v", aggregateTarget, err)
	}
	return verifyDryRunOrder(secret, aggregate)
}

// verifyDryRunOrder is the pure-function form of the ordering check.
// It is exported within the package so mutation tests can call it
// directly with synthetic dry-run output without spawning `make`.
func verifyDryRunOrder(secretDryRun, aggregateDryRun string) error {
	if err := bannerOrder(secretTarget, secretDryRun); err != nil {
		return err
	}
	return bannerOrder(aggregateTarget, aggregateDryRun)
}

func bannerOrder(target, dryRun string) error {
	pidx := indexOf(dryRun, "Artifact Producer Enforcement")
	sidx := indexOf(dryRun, "Artifact Secret Hygiene")
	if pidx < 0 || sidx < 0 {
		return fmt.Errorf("make -n %s missing gate banners (producer=%d secret=%d)",
			target, pidx, sidx)
	}
	if pidx > sidx {
		return fmt.Errorf("make -n %s: producer gate appears AFTER secret gate (producer=%d secret=%d)",
			target, pidx, sidx)
	}
	return nil
}

func makeDryRun(repoRoot, target string) (string, error) {
	cmd := exec.Command("make", "-n", "-f", "Makefile", target)
	cmd.Dir = repoRoot
	var out strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return out.String(), err
	}
	return out.String(), nil
}

func indexOf(haystack, needle string) int {
	return strings.Index(haystack, needle)
}