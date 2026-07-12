// ACT-UVB76-HULK05R4R1A-R2: mutation tests for the recipe-aware
// Makefile composition verifier.
//
// Each test constructs a Makefile fragment containing both producer
// and secret targets, exercises one mutation, and asserts that the
// checker accepts (GNU Make-legal) or rejects (illegal) the fragment
// as expected. The mutation set is the minimum that proves every
// assertion in main.go is wired through.
//
// Mutations covered:
//   1. duplicate recipe rule            → FAIL
//   2. prerequisite-only duplicate       → PASS (GNU Make legal)
//   3. missing producer dependency      → FAIL
//   4. unterminated recipe double quote → FAIL
//   5. unterminated recipe single quote → FAIL
//   6. reversed banner ordering         → FAIL
//   7. missing banner in dry-run        → FAIL
//   8. happy path                       → PASS
package main

import (
	"path/filepath"
	"strings"
	"testing"
)

// Canonical fragments used by the mutation tests. Each must produce a
// valid rule for both the producer and secret gate targets.
const (
	producerRecipeOK = "hulk-uvb76-artifact-producer-gate:\n" +
		"\t@echo producer\n"

	secretRecipeOK = "hulk-uvb76-artifact-secret-gate: hulk-uvb76-artifact-producer-gate\n" +
		"\t@echo secret\n"
)

// checkFragment runs the static (non-make-spawning) portion of the
// verifier against a Makefile body and returns the assembled errors.
// It does NOT call verifyDryRunOrdering (which spawns make); for the
// banner-ordering mutations the test calls verifyDryRunOrder directly
// with synthetic dry-run strings.
func checkFragment(t *testing.T, body string) []string {
	t.Helper()
	c := body
	var errs []string

	producerRules := parseRules(c, producerTarget)
	secretRules := parseRules(c, secretTarget)

	errs = append(errs, checkRecipeRuleCount(producerTarget, producerRules)...)
	errs = append(errs, checkRecipeRuleCount(secretTarget, secretRules)...)

	if !hasWord(strings.Join(effectivePrereqs(secretRules), " "), producerTarget) {
		errs = append(errs, "secret target missing dependency on producer")
	}
	errs = append(errs, checkRecipeShellLines(producerRules)...)
	errs = append(errs, checkRecipeShellLines(secretRules)...)
	return errs
}

func anyContains(errs []string, needles ...string) bool {
	for _, e := range errs {
		for _, n := range needles {
			if strings.Contains(e, n) {
				return true
			}
		}
	}
	return false
}

// 1. Happy path: a canonical fragment passes every static check.
func TestCheck_HappyPath_Passes(t *testing.T) {
	body := producerRecipeOK + "\n" + secretRecipeOK
	errs := checkFragment(t, body)
	if len(errs) > 0 {
		t.Fatalf("expected pass on canonical fragment; got errors=%v", errs)
	}
}

// 2. Duplicate recipe rules: both producer rules supply a recipe.
// GNU Make resolves this by using the second, which is almost always
// wrong. The checker must reject.
func TestCheck_DuplicateRecipe_Fails(t *testing.T) {
	body := producerRecipeOK +
		"hulk-uvb76-artifact-producer-gate:\n" +
		"\t@echo second-recipe\n" +
		secretRecipeOK
	errs := checkFragment(t, body)
	if len(errs) == 0 {
		t.Fatalf("expected FAIL on duplicate recipes; got PASS")
	}
	if !anyContains(errs, "recipe-bearing rule(s)") {
		t.Fatalf("expected recipe-count error; got %v", errs)
	}
}

// 3. Prerequisite-only duplicate: a second rule that ONLY adds a
// dependency. This is legal GNU Make (prereqs merge). The checker
// must accept.
func TestCheck_PrerequisiteOnlyDuplicate_Accepted(t *testing.T) {
	body := producerRecipeOK +
		"hulk-uvb76-artifact-producer-gate: verify-script-doctrine\n" +
		secretRecipeOK
	errs := checkFragment(t, body)
	if len(errs) > 0 {
		t.Fatalf("expected PASS on prerequisite-only duplicate; got errors=%v", errs)
	}
}

// 4. Missing dependency: secret target has no producer prereq.
func TestCheck_MissingDependency_Fails(t *testing.T) {
	body := producerRecipeOK +
		"hulk-uvb76-artifact-secret-gate:\n" +
		"\t@echo secret-without-dep\n"
	errs := checkFragment(t, body)
	if len(errs) == 0 {
		t.Fatalf("expected FAIL when secret does not depend on producer")
	}
	if !anyContains(errs, "missing dependency") {
		t.Fatalf("expected missing-dep error; got %v", errs)
	}
}

// 5. Unterminated double quote inside a recipe.
func TestCheck_UnterminatedDoubleQuote_Fails(t *testing.T) {
	body := producerRecipeOK +
		"hulk-uvb76-artifact-secret-gate: hulk-uvb76-artifact-producer-gate\n" +
		"\t@echo \"unterminated-string\n"
	errs := checkFragment(t, body)
	if len(errs) == 0 {
		t.Fatalf("expected FAIL on unterminated double quote")
	}
	if !anyContains(errs, "unterminated double quote") {
		t.Fatalf("expected unterminated-quote error; got %v", errs)
	}
}

// 6. Unterminated single quote inside a recipe.
func TestCheck_UnterminatedSingleQuote_Fails(t *testing.T) {
	body := producerRecipeOK +
		"hulk-uvb76-artifact-secret-gate: hulk-uvb76-artifact-producer-gate\n" +
		"\t@echo 'unterminated\n"
	errs := checkFragment(t, body)
	if len(errs) == 0 {
		t.Fatalf("expected FAIL on unterminated single quote")
	}
	if !anyContains(errs, "unterminated single quote") {
		t.Fatalf("expected unterminated-quote error; got %v", errs)
	}
}

// 7. Dangling operator at end of recipe line.
func TestCheck_DanglingOperator_Fails(t *testing.T) {
	body := producerRecipeOK +
		"hulk-uvb76-artifact-secret-gate: hulk-uvb76-artifact-producer-gate\n" +
		"\t@echo step1 &&\n"
	errs := checkFragment(t, body)
	if len(errs) == 0 {
		t.Fatalf("expected FAIL on dangling operator")
	}
	if !anyContains(errs, "dangling operator") {
		t.Fatalf("expected dangling-operator error; got %v", errs)
	}
}

// 8. Reversed banner ordering: secret banner appears BEFORE producer.
func TestVerifyDryRunOrder_ReversedFails(t *testing.T) {
	reversed := strings.Repeat("x", 5) + "Artifact Secret Hygiene\n" +
		strings.Repeat("y", 25) + "Artifact Producer Enforcement\n"
	err := verifyDryRunOrder(reversed, reversed)
	if err == nil {
		t.Fatalf("expected FAIL when secret banner precedes producer banner")
	}
	if !anyContains([]string{err.Error()}, "AFTER secret gate") {
		t.Fatalf("expected AFTER-secret error; got %v", err)
	}
}

// 9. Missing banner in dry-run.
func TestVerifyDryRunOrder_MissingBanner_Fails(t *testing.T) {
	noBanner := "Some other output\nbut no gate banners\n"
	err := verifyDryRunOrder(noBanner, noBanner)
	if err == nil {
		t.Fatalf("expected FAIL when banners are missing")
	}
	if !anyContains([]string{err.Error()}, "missing gate banners") {
		t.Fatalf("expected missing-banner error; got %v", err)
	}
}

// 10. Correct ordering passes.
func TestVerifyDryRunOrder_CorrectPasses(t *testing.T) {
	correct := "Artifact Producer Enforcement\n" +
		strings.Repeat("y", 25) + "Artifact Secret Hygiene\n"
	if err := verifyDryRunOrder(correct, correct); err != nil {
		t.Fatalf("expected PASS on correct ordering; got %v", err)
	}
}

// 11. Recipe lines attached to the rule: verifies collectRecipeLines
// correctly gathers TAB-indented lines and excludes non-recipe lines.
func TestParseRules_CollectsRecipeLines(t *testing.T) {
	body := "hulk-uvb76-artifact-producer-gate:\n" +
		"\t@echo line-one\n" +
		"\t@echo line-two\n" +
		"\n" +
		"unrelated-target:\n" +
		"\t@echo unrelated\n"
	rules := parseRules(body, producerTarget)
	if len(rules) != 1 {
		t.Fatalf("expected exactly 1 rule for producer; got %d", len(rules))
	}
	got := rules[0].RecipeLines
	if len(got) != 2 {
		t.Fatalf("expected 2 recipe lines; got %d (%v)", len(got), got)
	}
	if got[0] != "@echo line-one" || got[1] != "@echo line-two" {
		t.Fatalf("unexpected recipe content: %v", got)
	}
}

// 12. parseRules skips comments and directives.
func TestParseRules_SkipsCommentsAndDirectives(t *testing.T) {
	body := "# leading comment\n" +
		".PHONY: something\n" +
		"hulk-uvb76-artifact-producer-gate:\n" +
		"\t@echo only-rule\n"
	rules := parseRules(body, producerTarget)
	if len(rules) != 1 {
		t.Fatalf("expected exactly 1 producer rule; got %d", len(rules))
	}
	if len(rules[0].RecipeLines) != 1 {
		t.Fatalf("expected 1 recipe line; got %d", len(rules[0].RecipeLines))
	}
}

// 13. effectivePrereqs merges prerequisites across multiple rules,
// preserves first-seen order, and deduplicates.
func TestEffectivePrereqs_MergesAndDeduplicates(t *testing.T) {
	rules := []RuleDefinition{
		{Prerequisites: []string{"a", "b"}},
		{Prerequisites: []string{"c", "a"}}, // 'a' duplicated
		{Prerequisites: []string{"b", "d"}}, // 'b' duplicated
	}
	got := effectivePrereqs(rules)
	want := []string{"a", "b", "c", "d"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Fatalf("effectivePrereqs = %v; want %v", got, want)
	}
}

// 14. shellLineParses accepts balanced quotes and rejects dangling.
func TestShellLineParses(t *testing.T) {
	good := []string{
		"@echo hello",
		"@echo \"hello world\"",
		"@echo 'hello world'",
		"@echo \"escaped \\\"quote\\\"\"",
		"@cd path && go test",
		"@cmd | tee out.log",
	}
	for _, line := range good {
		if err := shellLineParses(line); err != nil {
			t.Errorf("expected pass for %q; got %v", line, err)
		}
	}
	bad := []string{
		"@echo \"unterminated",
		"@echo 'unterminated",
		"@cmd &&",
		"@cmd ||",
		"@cmd |",
	}
	for _, line := range bad {
		if err := shellLineParses(line); err == nil {
			t.Errorf("expected FAIL for %q; got pass", line)
		}
	}
}

// Helper: ensure filepath is referenced so the package compiles even
// when a future refactor moves path-handling into this file.
var _ = filepath.Join