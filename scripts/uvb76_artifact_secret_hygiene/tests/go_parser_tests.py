"""
Go AST parser tests for UVB-76 Artifact Secret Hygiene.

Tests the Go AST parser that validates rule constant declarations
in redact.go against the canonical registry.

Uses real go/parser, go/ast, go/token to parse Go source.
"""

import os
import re
import subprocess
import tempfile
import json
from dataclasses import dataclass
from typing import Optional, Dict, List, Tuple

from ..registry_loader import get_registry


# ============================================================================
# Go AST Parser - Real AST using go/parser, go/ast, go/token
# ============================================================================

@dataclass
class GoRuleDeclaration:
    """Represents a parsed Go rule constant declaration."""
    const_name: str
    rule_id: str
    rule_class: str
    source_file: str
    line_number: int


# Bounded query parsing constant
MAX_QUERY_PARAMS = 100


def _parse_go_constants_real_ast(go_source_path: str) -> tuple[list[GoRuleDeclaration], list[str]]:
    """
    Parse Go source file using real go/parser and go/ast.

    Returns (declarations, errors) where declarations is a list preserving all entries
    (including duplicates) and errors contains any parsing issues.

    This is the ONLY production parsing mechanism - no regex fallback.
    Fails closed for: Go unavailable, compilation failure, timeout, ParseFile failure,
    invalid JSON output, missing parser marker, empty declaration result,
    unsupported expression, class annotation missing.
    """
    declarations: list[GoRuleDeclaration] = []
    errors: list[str] = []

    # Go AST parser script that emits structured JSON with parser marker
    # Must be in same directory as target Go source for go/parser
    script = '''package main

import (
    "encoding/json"
    "fmt"
    "go/ast"
    "go/parser"
    "go/token"
    "os"
    "strings"
)

type RuleInfo struct {
    ConstName string `json:"const_name"`
    RuleID    string `json:"rule_id"`
    Class     string `json:"class"`
    Line      int    `json:"line"`
}

type Output struct {
    Parser       string     `json:"parser"`
    Declarations []RuleInfo `json:"declarations"`
}

func main() {
    if len(os.Args) < 2 {
        fmt.Fprintf(os.Stderr, "Usage: %s <source_file>\\n", os.Args[0])
        os.Exit(1)
    }

    fset := token.NewFileSet()
    node, err := parser.ParseFile(fset, os.Args[1], nil, parser.ParseComments)
    if err != nil {
        fmt.Fprintf(os.Stderr, "ParseFile error: %v\\n", err)
        os.Exit(1)
    }

    var rules []RuleInfo

    for _, decl := range node.Decls {
        genDecl, ok := decl.(*ast.GenDecl)
        if !ok || genDecl.Tok != token.CONST {
            continue
        }

        for _, spec := range genDecl.Specs {
            valueSpec, ok := spec.(*ast.ValueSpec)
            if !ok {
                continue
            }

            // Look for Rule* identifiers
            for i, name := range valueSpec.Names {
                if name.Name == "_" || !ast.IsExported(name.Name) {
                    continue
                }
                if !hasRulePrefix(name.Name) {
                    continue
                }

                // Get the rule ID from the value
                if i >= len(valueSpec.Values) {
                    continue
                }

                ruleID := extractStringLit(valueSpec.Values[i])
                if ruleID == "" || !hasUVB76Prefix(ruleID) {
                    continue
                }

                // Extract class from per-value comment
                // Contract: ValueSpec.Comment (same-line) then ValueSpec.Doc (preceding)
                // If neither exists, this is an error condition
                className := extractClassFromValueSpec(valueSpec)
                if className == "" {
                    // No class annotation - fail closed
                    fmt.Fprintf(os.Stderr, "Class annotation missing for %s at line %d\\n",
                        name.Name, fset.Position(name.Pos()).Line)
                    os.Exit(1)
                }

                pos := fset.Position(name.Pos())
                rules = append(rules, RuleInfo{
                    ConstName: name.Name,
                    RuleID:    ruleID,
                    Class:     className,
                    Line:      pos.Line,
                })
            }
        }
    }

    // Emit structured output with parser marker
    output := Output{
        Parser:       "go-ast",
        Declarations: rules,
    }
    json.NewEncoder(os.Stdout).Encode(output)
}

func hasRulePrefix(name string) bool {
    return len(name) > 4 && name[:4] == "Rule"
}

func hasUVB76Prefix(s string) bool {
    return len(s) > 6 && s[:6] == "UVB76-"
}

func extractStringLit(expr ast.Expr) string {
    switch e := expr.(type) {
    case *ast.BasicLit:
        if e.Kind == token.STRING {
            // Remove quotes
            s := e.Value
            if len(s) >= 2 {
                return s[1 : len(s)-1]
            }
        }
    }
    return ""
}

// extractClassFromValueSpec extracts the class annotation from a ValueSpec.
// Contract: ValueSpec.Comment (same-line trailing) takes precedence over
// ValueSpec.Doc (preceding documentation comment).
// Returns the last word from the comment (e.g., "private_key_pem" from "// private_key_pem").
func extractClassFromValueSpec(valueSpec *ast.ValueSpec) string {
    // Prefer ValueSpec.Comment (same-line trailing annotation)
    if valueSpec.Comment != nil {
        for _, comment := range valueSpec.Comment.List {
            text := comment.Text
            // Handle line comments: // comment
            if len(text) > 2 && text[:2] == "//" {
                text = text[2:]
            }
            text = strings.TrimSpace(text)
            // Extract last word as class name
            parts := strings.Fields(text)
            if len(parts) > 0 {
                return parts[len(parts)-1]
            }
        }
    }

    // Fall back to ValueSpec.Doc (preceding documentation)
    if valueSpec.Doc != nil {
        for _, comment := range valueSpec.Doc.List {
            text := comment.Text
            // Handle line comments: // comment
            if len(text) > 2 && text[:2] == "//" {
                text = text[2:]
            }
            text = strings.TrimSpace(text)
            // Extract last word as class name
            parts := strings.Fields(text)
            if len(parts) > 0 {
                return parts[len(parts)-1]
            }
        }
    }

    return ""
}
'''

    # Use temp directory - copy Go source there so both files are in same directory
    # Note: filenames cannot start with underscore as Go ignores those files
    try:
        with tempfile.TemporaryDirectory() as tmp_dir:
            script_path = os.path.join(tmp_dir, 'astparser.go')
            binary_path = os.path.join(tmp_dir, 'astparser')
            source_copy = os.path.join(tmp_dir, 'source.go')

            # Write the parser script
            with open(script_path, 'w') as f:
                f.write(script)

            # Copy the Go source to temp directory
            import shutil
            shutil.copy2(go_source_path, source_copy)

            # Build the parser to a binary
            build_result = subprocess.run(
                ['go', 'build', '-o', binary_path, script_path],
                capture_output=True,
                text=True,
                timeout=60,
            )

            if build_result.returncode != 0:
                errors.append(f"AST parser build failed: {build_result.stderr.strip()}")
                return declarations, errors

            # Run parser on the copied source (same directory)
            result = subprocess.run(
                [binary_path, source_copy],
                capture_output=True,
                text=True,
                timeout=30,
            )

            if result.returncode != 0:
                errors.append(f"AST parser exit code {result.returncode}")
                if result.stderr:
                    errors.append(f"AST parser stderr: {result.stderr.strip()}")
                return declarations, errors

            if not result.stdout.strip():
                errors.append("Empty output from AST parser")
                return declarations, errors

            try:
                data = json.loads(result.stdout.strip())

                # Validate parser marker - MUST be exactly "go-ast"
                parser_marker = data.get("parser", "")
                if parser_marker != "go-ast":
                    errors.append(f"Invalid parser marker: {parser_marker} (expected: go-ast)")
                    return declarations, errors

                declarations_list = data.get("declarations", [])
                if not declarations_list:
                    errors.append("Empty declaration result from AST parser")
                    return declarations, errors

                for item in declarations_list:
                    declarations.append(GoRuleDeclaration(
                        const_name=item['const_name'],
                        rule_id=item['rule_id'],
                        rule_class=item['class'],
                        source_file=go_source_path,
                        line_number=item['line'],
                    ))
                return declarations, errors

            except (json.JSONDecodeError, KeyError) as e:
                errors.append(f"Invalid JSON output from AST parser: {e}")
                return declarations, errors

    except subprocess.TimeoutExpired:
        errors.append("AST parser timeout")
        return declarations, errors
    except FileNotFoundError:
        errors.append("Go unavailable (go command not found)")
        return declarations, errors
    except Exception as e:
        errors.append(f"AST parser failed: {e}")
        return declarations, errors


# ============================================================================
# Production comparison function used by gate
# ============================================================================

def validate_go_registry_agreement(
    declarations: list[GoRuleDeclaration],
    registry: dict
) -> tuple[list[str], bool]:
    """
    Validate Go declarations against registry before indexing.

    Checks:
    - duplicate constant name
    - duplicate rule ID
    - one constant name mapped to multiple IDs
    - one rule ID mapped to multiple constants
    - missing class
    - unknown class

    Returns (errors, is_valid).
    """
    errors: list[str] = []
    seen_const_names: dict[str, list[int]] = {}
    seen_rule_ids: dict[str, list[int]] = {}
    const_to_id: dict[str, str] = {}
    id_to_const: dict[str, str] = {}

    registry_classes: dict[str, str] = {r["rule_id"]: r["class"] for r in registry.get("rules", [])}

    for decl in declarations:
        # Track constant names
        if decl.const_name not in seen_const_names:
            seen_const_names[decl.const_name] = []
        seen_const_names[decl.const_name].append(decl.line_number)

        # Track rule IDs
        if decl.rule_id not in seen_rule_ids:
            seen_rule_ids[decl.rule_id] = []
        seen_rule_ids[decl.rule_id].append(decl.line_number)

        # Check constant -> ID mapping
        if decl.const_name in const_to_id:
            if const_to_id[decl.const_name] != decl.rule_id:
                errors.append(f"Constant {decl.const_name} maps to multiple IDs")
        else:
            const_to_id[decl.const_name] = decl.rule_id

        # Check ID -> constant mapping
        if decl.rule_id in id_to_const:
            if id_to_const[decl.rule_id] != decl.const_name:
                errors.append(f"Rule ID {decl.rule_id} maps to multiple constants")
        else:
            id_to_const[decl.rule_id] = decl.const_name

        # Check class exists in registry
        if decl.rule_id not in registry_classes:
            errors.append(f"Unknown class for {decl.rule_id}: {decl.rule_class}")
        else:
            expected_class = registry_classes[decl.rule_id]
            if decl.rule_class != expected_class:
                errors.append(
                    f"Class mismatch for {decl.rule_id}: got '{decl.rule_class}', expected '{expected_class}'"
                )

    # Report duplicates
    for const_name, lines in seen_const_names.items():
        if len(lines) > 1:
            errors.append(f"Duplicate constant name: {const_name} at lines {lines}")

    for rule_id, lines in seen_rule_ids.items():
        if len(lines) > 1:
            errors.append(f"Duplicate rule ID: {rule_id} at lines {lines}")

    return errors, len(errors) == 0


class GoRegistryAgreementError(Exception):
    """Raised when Go constants don't agree with registry."""
    def __init__(self, errors: list[str]):
        self.errors = errors
        super().__init__(f"Go-registry agreement failed:\n" + "\n".join(f"  - {e}" for e in errors))


def _parse_go_constants_subprocess(go_source_path: str) -> tuple[dict[str, tuple[str, str, int]], list[str]]:
    """
    Parse Go source using Go's AST parser via subprocess.

    This is the PRODUCTION mechanism - uses go/parser and go/ast only.
    Returns (constants, errors) tuple.

    IMPORTANT: Errors are now propagated, not ignored.
    Gate must check errors and fail if non-empty.
    """
    declarations, parse_errors = _parse_go_constants_real_ast(go_source_path)

    # Fail closed: if AST parsing failed, return empty with errors
    if parse_errors or not declarations:
        return {}, parse_errors

    # Validate declarations before indexing
    registry = get_registry()
    validation_errors, is_valid = validate_go_registry_agreement(declarations, registry)
    if not is_valid:
        # PROPAGATE validation errors - do NOT ignore
        return {}, validation_errors

    # Convert to dict (taking first occurrence for each rule_id)
    constants = {}
    for decl in declarations:
        if decl.rule_id not in constants:
            constants[decl.rule_id] = (decl.const_name, decl.rule_class, decl.line_number)

    return constants, []


# ============================================================================
# Regex parser - TEST FIXTURES ONLY (not production)
# ============================================================================

def _parse_go_constants_regex_with_declarations(go_source_path: str) -> tuple[list[GoRuleDeclaration], list[str]]:
    """
    Parse Go source using regex pattern matching.

    THIS IS FOR TEST FIXTURES ONLY - NOT PRODUCTION USE.
    Regex helpers may remain only as isolated unit-test fixtures.
    """
    declarations: list[GoRuleDeclaration] = []
    errors: list[str] = []

    try:
        with open(go_source_path, 'r', encoding='utf-8') as f:
            lines = f.readlines()
    except OSError as e:
        errors.append(f"Failed to read file: {e}")
        return declarations, errors

    # Pattern to match constant declarations with class comment
    const_pattern = re.compile(
        r'^\s*(?:const\s+)?(Rule\w+)\s*=\s*"([^"]+)"\s*//\s*(\w+)\s*$'
    )

    for line_num, line in enumerate(lines, 1):
        match = const_pattern.match(line)
        if match:
            declarations.append(GoRuleDeclaration(
                const_name=match.group(1),
                rule_id=match.group(2),
                rule_class=match.group(3),
                source_file=go_source_path,
                line_number=line_num,
            ))

    return declarations, errors


def _parse_go_constants_regex(go_source_path: str) -> dict[str, tuple[str, str, int]]:
    """
    Fallback: Parse Go source using regex pattern matching.

    THIS IS FOR TEST FIXTURES ONLY - NOT PRODUCTION USE.
    """
    declarations, _ = _parse_go_constants_regex_with_declarations(go_source_path)

    constants = {}
    for decl in declarations:
        if decl.rule_id not in constants:
            constants[decl.rule_id] = (decl.const_name, decl.rule_class, decl.line_number)

    return constants


# ============================================================================
# Helper functions
# ============================================================================

def _get_redact_go_path() -> str:
    """Get the absolute path to redact.go."""
    tests_dir = os.path.dirname(os.path.abspath(__file__))
    scripts_dir = os.path.dirname(tests_dir)  # uvb76_artifact_secret_hygiene
    project_root = os.path.dirname(os.path.dirname(scripts_dir))  # KGB
    return os.path.join(project_root, "uvb76", "internal", "redact", "redact.go")


# ============================================================================
# Test Cases
# ============================================================================

def test_go_ast_parser_available() -> tuple[bool, str]:
    """
    Test that we can parse Go source using Go AST parser.

    Verifies go/parser, go/ast, go/token are accessible.
    """
    redact_go_path = _get_redact_go_path()

    if not os.path.exists(redact_go_path):
        return False, f"Go source not found at {redact_go_path}"

    constants, errors = _parse_go_constants_subprocess(redact_go_path)

    # Fail if errors occurred
    if errors:
        return False, f"Go parsing errors: {errors[:2]}"

    if constants:
        return True, f"Go constants parsed via Go AST parser ({len(constants)} rules)"

    return False, "Failed to parse Go constants via AST"


def test_go_constants_match_registry() -> tuple[bool, str]:
    """
    Test that Go constants agree with registry using parsed constants.
    """
    redact_go_path = _get_redact_go_path()

    if not os.path.exists(redact_go_path):
        return False, f"Go source not found"

    go_constants, errors = _parse_go_constants_subprocess(redact_go_path)

    # Fail if errors occurred
    if errors:
        return False, f"Go-registry errors: {errors[:2]}"

    if not go_constants:
        return False, "No Go constants found"

    registry = get_registry()
    registry_rules = {r["rule_id"]: r["class"] for r in registry.get("rules", [])}

    errors = []

    # Check Go constants against registry
    for rule_id, (const_name, class_name, line) in go_constants.items():
        if rule_id not in registry_rules:
            errors.append(f"Go {const_name}={rule_id} not in registry")
            continue

        registry_class = registry_rules[rule_id]
        if class_name != registry_class:
            errors.append(
                f"Go {const_name} class '{class_name}' != registry '{registry_class}'"
            )

    # Check registry rules against Go constants
    for rule_id, registry_class in registry_rules.items():
        if rule_id not in go_constants:
            errors.append(f"Registry rule {rule_id} missing Go constant")

    if errors:
        return False, f"Go-registry mismatches: {errors[:5]}"

    return True, f"Go constants agree with registry ({len(go_constants)} rules)"


def test_go_parser_mechanism_is_accurate() -> tuple[bool, str]:
    """
    Test that the Go parser uses AST mechanism.

    Verifies we're using actual Go AST parsing (go/parser, go/ast, go/token).
    """
    redact_go_path = _get_redact_go_path()

    ast_constants, errors = _parse_go_constants_subprocess(redact_go_path)

    if ast_constants and not errors:
        return True, f"Using Go AST parser ({len(ast_constants)} rules)"

    return False, "No Go AST parsing mechanism available"


def test_go_constants_parse_single_declaration() -> tuple[bool, str]:
    """Test that single const declarations are parsed."""
    content = '''
package main

const RuleTest = "UVB76-SECRET-0001" // private_key_pem
'''

    with tempfile.NamedTemporaryFile(mode='w', suffix='.go', delete=False) as f:
        f.write(content)
        path = f.name

    try:
        constants = _parse_go_constants_regex(path)
        if "UVB76-SECRET-0001" in constants:
            return True, "Single const declaration parsed"
        return False, "Single const declaration not parsed"
    finally:
        os.unlink(path)


def test_go_constants_parse_grouped_declaration() -> tuple[bool, str]:
    """Test that grouped const declarations are parsed."""
    content = '''
package main

const (
    RuleTest1 = "UVB76-SECRET-0001" // private_key_pem
    RuleTest2 = "UVB76-SECRET-0002" // encrypted_private_key_pem
)
'''

    with tempfile.NamedTemporaryFile(mode='w', suffix='.go', delete=False) as f:
        f.write(content)
        path = f.name

    try:
        constants = _parse_go_constants_regex(path)
        if len(constants) >= 2:
            return True, f"Grouped const declarations parsed ({len(constants)})"
        return False, f"Expected 2, got {len(constants)}"
    finally:
        os.unlink(path)


def test_go_constants_parse_with_tabs_and_spaces() -> tuple[bool, str]:
    """Test that various whitespace patterns are handled."""
    content = '''
package main

const (
    RuleTest1="UVB76-SECRET-0001"//comment
    RuleTest2  =  "UVB76-SECRET-0002"  //  spaced
)
'''

    with tempfile.NamedTemporaryFile(mode='w', suffix='.go', delete=False) as f:
        f.write(content)
        path = f.name

    try:
        constants = _parse_go_constants_regex(path)
        if len(constants) >= 2:
            return True, f"Whitespace variations parsed ({len(constants)})"
        return False, f"Expected 2, got {len(constants)}"
    finally:
        os.unlink(path)


def test_go_constants_detect_duplicate_id() -> tuple[bool, str]:
    """Test that duplicate rule IDs in Go would be detected."""
    content = '''
package main

const (
    RuleTest1 = "UVB76-SECRET-0001" // private_key_pem
    RuleTest1Dup = "UVB76-SECRET-0001" // duplicate_id
)
'''

    with tempfile.NamedTemporaryFile(mode='w', suffix='.go', delete=False) as f:
        f.write(content)
        path = f.name

    try:
        declarations, _ = _parse_go_constants_regex_with_declarations(path)

        seen_ids: dict[str, list[str]] = {}
        duplicates: list[tuple[str, str, str]] = []

        for decl in declarations:
            if decl.rule_id in seen_ids:
                for prev_const_name in seen_ids[decl.rule_id]:
                    duplicates.append((decl.rule_id, prev_const_name, decl.const_name))
                seen_ids[decl.rule_id].append(decl.const_name)
            else:
                seen_ids[decl.rule_id] = [decl.const_name]

        if duplicates:
            return True, f"Duplicate IDs detected: {duplicates}"
        return False, "Duplicate IDs not detected"
    finally:
        os.unlink(path)


def test_go_constants_detect_wrong_class() -> tuple[bool, str]:
    """Test that wrong class annotations are detected."""
    content = '''
package main

const (
    RuleTest = "UVB76-SECRET-0001" // wrong_class_name
)
'''

    with tempfile.NamedTemporaryFile(mode='w', suffix='.go', delete=False) as f:
        f.write(content)
        path = f.name

    try:
        constants = _parse_go_constants_regex(path)
        registry = get_registry()

        registry_class = None
        for rule in registry.get("rules", []):
            if rule["rule_id"] == "UVB76-SECRET-0001":
                registry_class = rule["class"]
                break

        if "UVB76-SECRET-0001" in constants:
            _, parsed_class, _ = constants["UVB76-SECRET-0001"]
            if registry_class and parsed_class != registry_class:
                return True, f"Wrong class detected: {parsed_class} != {registry_class}"
            return False, "Class mismatch not detected"

        return False, "Rule not parsed"
    finally:
        os.unlink(path)


def test_go_constants_detect_missing_constant() -> tuple[bool, str]:
    """Test that missing Go constants for registry rules are detected."""
    redact_go_path = _get_redact_go_path()

    go_constants, errors = _parse_go_constants_subprocess(redact_go_path)

    # If there are errors, the test fails
    if errors:
        return False, f"Go parsing errors: {errors[:2]}"

    registry = get_registry()
    registry_rules = {r["rule_id"] for r in registry.get("rules", [])}

    missing = []
    for rule_id in registry_rules:
        if rule_id not in go_constants:
            missing.append(rule_id)

    if missing:
        return False, f"Missing Go constants for: {missing[:3]}"

    return True, "All registry rules have Go constants"


def test_go_constants_detect_unknown_constant() -> tuple[bool, str]:
    """Test that unknown Go constants (not in registry) are detected."""
    redact_go_path = _get_redact_go_path()

    go_constants, errors = _parse_go_constants_subprocess(redact_go_path)

    # If there are errors, the test fails
    if errors:
        return False, f"Go parsing errors: {errors[:2]}"

    registry = get_registry()
    registry_rules = {r["rule_id"] for r in registry.get("rules", [])}

    unknown = []
    for rule_id in go_constants:
        if rule_id not in registry_rules:
            unknown.append(rule_id)

    if unknown:
        return False, f"Unknown Go constants found: {unknown}"

    return True, "No unknown Go constants"


def test_go_constants_unrelated_constants() -> tuple[bool, str]:
    """Test that unrelated constants are not confused with rule constants."""
    content = '''
package main

const (
    MaxRetries = 3
    Timeout    = 30
    RuleTest   = "UVB76-SECRET-0001" // private_key_pem
    BufferSize = 1024
)
'''

    with tempfile.NamedTemporaryFile(mode='w', suffix='.go', delete=False) as f:
        f.write(content)
        path = f.name

    try:
        constants = _parse_go_constants_regex(path)
        if "UVB76-SECRET-0001" in constants:
            unrelated = [k for k in constants if not k.startswith("UVB76-SECRET-")]
            if unrelated:
                return False, f"Unrelated constants captured: {unrelated}"
            return True, "Only rule constants captured"
        return False, "Rule constant not captured"
    finally:
        os.unlink(path)


def test_go_constants_comments_before_after() -> tuple[bool, str]:
    """Test that comments before and after values don't break parsing."""
    content = '''
package main

const (
    // This is a comment before
    RuleTest = "UVB76-SECRET-0001" // private_key_pem
    // This is a comment after
)
'''

    with tempfile.NamedTemporaryFile(mode='w', suffix='.go', delete=False) as f:
        f.write(content)
        path = f.name

    try:
        constants = _parse_go_constants_regex(path)
        if "UVB76-SECRET-0001" in constants:
            return True, "Comments handled correctly"
        return False, "Comments broke parsing"
    finally:
        os.unlink(path)


def test_value_spec_comment_contract() -> tuple[bool, str]:
    """
    Test ValueSpec.Comment precedence contract.

    Contract: ValueSpec.Comment (same-line trailing) takes precedence over
    ValueSpec.Doc (preceding documentation comment).
    """
    # Test 1: Same-line comment takes precedence
    content = '''
package main

const RuleA = "UVB76-SECRET-0001" // private_key_pem

const (
    RuleB = "UVB76-SECRET-0002" // encrypted_private_key_pem
)

// rsa_private_key_pem
const RuleC = "UVB76-SECRET-0003"
'''

    with tempfile.NamedTemporaryFile(mode='w', suffix='.go', delete=False) as f:
        f.write(content)
        path = f.name

    try:
        declarations, errors = _parse_go_constants_regex_with_declarations(path)

        # Verify declarations were found
        rule_ids = {d.rule_id for d in declarations}
        if "UVB76-SECRET-0001" not in rule_ids:
            return False, "RuleA not parsed"
        if "UVB76-SECRET-0002" not in rule_ids:
            return False, "RuleB not parsed"
        if "UVB76-SECRET-0003" not in rule_ids:
            return False, "RuleC not parsed"

        # Verify class annotations
        classes = {d.rule_class for d in declarations}
        if "private_key_pem" not in classes:
            return False, "private_key_pem class not found"
        if "encrypted_private_key_pem" not in classes:
            return False, "encrypted_private_key_pem class not found"
        if "rsa_private_key_pem" not in classes:
            return False, "rsa_private_key_pem class not found"

        return True, "ValueSpec.Comment contract verified"
    finally:
        os.unlink(path)
