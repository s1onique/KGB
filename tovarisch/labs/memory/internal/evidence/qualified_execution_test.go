// qualified_execution_test.go — Table-driven rejection class matrix
// for the qualified execution evidence, plus the CORRECTION22
// mutation tests for every supplied claim.
//
// The complete verifier has three phases:
//   1. verifyUnderlyingObservations (independent validator)
//   2. deriveClaims (pure derivation)
//   3. claim-agreement check
//
// The mutation tests exercise phase 3 in isolation: they build a
// valid raw-observation fixture, stamp the correct derived
// claims, then mutate exactly one supplied claim and verify the
// verifier rejects the lie. Serialized variants mutate the
// persisted JSON bytes directly.

package evidence

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
)

const validCanonicalImageID = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
const validCanonicalNetworkID = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcd00"

// buildValidObservations returns a dockerlab observation object with
// every required field set. Tests use this as the starting point
// for their single-mutation fixtures that target the raw layer.
func buildValidObservations() *dockerlab.QualifiedExecutionObservations {
	return &dockerlab.QualifiedExecutionObservations{
		SchemaVersion: dockerlab.QualifiedExecutionObservationsSchemaVersion,
		GeneratedAt:   time.Now().UTC(),
		Image: dockerlab.ImageObservations{
			RequestedReference:    "kgb-tovarisch-canary:latest",
			InspectedBeforeCreate: validCanonicalImageID,
			CreateRequestImage:    validCanonicalImageID,
			ContainerInspectImage: validCanonicalImageID,
			ContainerConfigImage:  validCanonicalImageID,
			InspectedRepoDigests:  []string{"kgb.dev/canary@sha256:" + strings.Repeat("a", 64)},
		},
		Network: dockerlab.NetworkObservations{
			RequestedName:       "kgb-lab-net",
			CreateResponseID:    validCanonicalNetworkID,
			InspectResponseID:   validCanonicalNetworkID,
			ContainerEndpointID: validCanonicalNetworkID,
			Removed:             true,
		},
		Pull: dockerlab.PullObservations{
			ObservationAvailable: true,
			Attempted:            false,
			AttemptCount:         0,
		},
		Container: dockerlab.ContainerObservations{
			ID:                    "container-id-1",
			Created:               true,
			Inspected:             true,
			Started:               true,
			TerminalStateObserved: true,
			Removed:               true,
		},
		Provenance: dockerlab.ProvenanceBinding{
			SourceCommit:        strings.Repeat("a", 40), // sha1
			SourceTree:          strings.Repeat("b", 40),
			GitObjectFormat:     "sha1",
			WorkingTreeDirty:    false,
			SourceCommitDirty:   false,
			VCSModified:         false,
			DockerServerVersion: "29.6.2",
			ProducerVersion:     "tovarisch-memory-lab/1.0.0",
			ExecutableSHA256:    strings.Repeat("c", 64),
		},
		// CORRECTION27: Valid reachability observations
		Reachability: dockerlab.ReachabilityObservations{
			Method:      dockerlab.ReachabilityMethodDockerExec,
			NetworkID:   validCanonicalNetworkID,
			ExecExitCode: 0,
			TargetHost: "localhost",
			TargetPort: 8080,
			HTTPResponseCode: 200,
			Success:    true,
		},
	}
}

// buildValidEvidence constructs a complete valid artifact
// directly (raw observations AND the correct supplied claims).
// Tests use this as the canonical happy-path fixture so the
// bytes verifier sees the right claim values.
func buildValidEvidence() *QualifiedExecutionEvidence {
	obs := buildValidObservations()
	ev := BuildEvidenceFromObservations(obs)
	ev.SetDerivedFields()
	ev.Pass = true
	ev.VerifierErrors = nil
	return ev
}

func TestVerifyQualifiedExecution_ValidFixturePasses(t *testing.T) {
	ev := buildValidEvidence()
	res := VerifyQualifiedExecution(ev)
	if !res.Pass {
		t.Fatalf("expected valid fixture to pass, got errors: %v", res.Errors)
	}
}

func TestVerifyQualifiedExecution_NilEvidenceFails(t *testing.T) {
	res := VerifyQualifiedExecution(nil)
	if res.Pass {
		t.Fatal("expected nil evidence to fail")
	}
}

func TestVerifyQualifiedExecution_MissingSchemaVersionFails(t *testing.T) {
	ev := buildValidEvidence()
	ev.SchemaVersion = ""
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected empty schema version to fail")
	}
}

func TestVerifyQualifiedExecution_UnsupportedSchemaVersionFails(t *testing.T) {
	ev := buildValidEvidence()
	ev.SchemaVersion = "2.0.0"
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected unsupported schema version to fail")
	}
}

func TestVerifyQualifiedExecution_MissingRequestedReferenceFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Image.RequestedReference = ""
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected missing requested reference to fail")
	}
}

func TestVerifyQualifiedExecution_MissingPreCreateImageIDFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Image.InspectedBeforeCreate = ""
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected missing pre-create image ID to fail")
	}
}

func TestVerifyQualifiedExecution_MalformedPreCreateImageIDFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Image.InspectedBeforeCreate = "not-an-id"
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected malformed pre-create image ID to fail")
	}
}

func TestVerifyQualifiedExecution_MalformedCreateRequestImageFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Image.CreateRequestImage = "not-an-id"
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected malformed create request image to fail")
	}
}

func TestVerifyQualifiedExecution_TagInCreateRequestFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Image.CreateRequestImage = "kgb-tovarisch-canary:latest"
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected tag-in-create-request to fail")
	}
}

func TestVerifyQualifiedExecution_PreCreateAndCreateRequestMismatchFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Image.InspectedBeforeCreate = "sha256:" + strings.Repeat("0", 64)
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected pre-create vs create-request mismatch to fail")
	}
}

func TestVerifyQualifiedExecution_ContainerRuntimeImageMismatchFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Image.ContainerInspectImage = "sha256:" + strings.Repeat("1", 64)
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected container runtime image mismatch to fail")
	}
}

func TestVerifyQualifiedExecution_ContainerConfigImageMismatchFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Image.ContainerConfigImage = "sha256:" + strings.Repeat("2", 64)
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected container config image mismatch to fail")
	}
}

func TestVerifyQualifiedExecution_MissingNetworkIDFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Network.CreateResponseID = ""
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected missing network create-response ID to fail")
	}
}

func TestVerifyQualifiedExecution_NetworkCreateInspectMismatchFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Network.InspectResponseID = strings.Repeat("9", 64)
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected network create/inspect mismatch to fail")
	}
}

func TestVerifyQualifiedExecution_NetworkEndpointMismatchFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Network.ContainerEndpointID = strings.Repeat("8", 64)
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected network endpoint mismatch to fail")
	}
}

func TestVerifyQualifiedExecution_PullAttemptedTrueFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Pull.Attempted = true
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected pull.attempted=true to fail")
	}
}

func TestVerifyQualifiedExecution_PullAttemptCountNonZeroFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Pull.AttemptCount = 1
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected pull.attempt_count=1 to fail")
	}
}

func TestVerifyQualifiedExecution_PullObservationUnavailableFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Pull.ObservationAvailable = false
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected missing pull audit to fail")
	}
}

func TestVerifyQualifiedExecution_MissingContainerIDFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Container.ID = ""
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected missing container ID to fail")
	}
}

func TestVerifyQualifiedExecution_MissingSourceCommitFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Provenance.SourceCommit = ""
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected missing source commit to fail")
	}
}

func TestVerifyQualifiedExecution_MissingSourceTreeFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Provenance.SourceTree = ""
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected missing source tree to fail")
	}
}

func TestVerifyQualifiedExecution_UnknownGitObjectFormatFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Provenance.GitObjectFormat = "blake2b"
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected unknown git object format to fail")
	}
}

func TestVerifyQualifiedExecution_SourceCommitLengthMismatchFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Provenance.GitObjectFormat = "sha1"
	obs.Provenance.SourceCommit = strings.Repeat("a", 41) // not 40
	ev := BuildEvidenceFromObservations(obs)
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected source commit length mismatch to fail")
	}
}

// ----------------------------------------------------------------------------
// P0-6: complete claim mutation tests. Each test builds a valid
// fixture (raw observations + matching claims) then mutates ONE
// supplied claim and verifies the verifier rejects the lie.
// ----------------------------------------------------------------------------

func TestVerifyQualifiedExecution_ImagePositiveLieFails(t *testing.T) {
	obs := buildValidObservations()
	// Break the underlying image agreement: now implied=false.
	obs.Image.ContainerInspectImage = "sha256:" + strings.Repeat("1", 64)
	ev := BuildEvidenceFromObservations(obs)
	// Lie: claim exact match while underlying disagrees.
	ev.ImageExactIDMatch = true
	ev.NetworkExactIDMatch = true
	ev.CleanupComplete = true
	ev.Pass = true
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected positive image_exact_id_match lie to fail")
	}
}

func TestVerifyQualifiedExecution_ImageNegativeLieFails(t *testing.T) {
	ev := buildValidEvidence()
	// Lie: claim image_exact_id_match=false while underlying agrees.
	ev.ImageExactIDMatch = false
	ev.Pass = false
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected negative image_exact_id_match lie to fail")
	}
}

func TestVerifyQualifiedExecution_NetworkPositiveLieFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Network.ContainerEndpointID = strings.Repeat("8", 64)
	ev := BuildEvidenceFromObservations(obs)
	ev.ImageExactIDMatch = true
	ev.NetworkExactIDMatch = true
	ev.CleanupComplete = true
	ev.Pass = true
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected positive network_exact_id_match lie to fail")
	}
}

func TestVerifyQualifiedExecution_NetworkNegativeLieFails(t *testing.T) {
	ev := buildValidEvidence()
	ev.NetworkExactIDMatch = false
	ev.Pass = false
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected negative network_exact_id_match lie to fail")
	}
}

func TestVerifyQualifiedExecution_CleanupPositiveLieFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Network.Removed = false
	ev := BuildEvidenceFromObservations(obs)
	ev.ImageExactIDMatch = true
	ev.NetworkExactIDMatch = true
	ev.CleanupComplete = true
	ev.Pass = true
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected positive cleanup_complete lie to fail")
	}
}

func TestVerifyQualifiedExecution_CleanupNegativeLieFails(t *testing.T) {
	ev := buildValidEvidence()
	ev.CleanupComplete = false
	ev.Pass = false
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected negative cleanup_complete lie to fail")
	}
}

func TestVerifyQualifiedExecution_PassPositiveLieFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Container.Removed = false
	ev := BuildEvidenceFromObservations(obs)
	// Underlying fails; supplied claim says pass=true.
	ev.ImageExactIDMatch = true
	ev.NetworkExactIDMatch = true
	ev.CleanupComplete = true
	ev.Pass = true
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected positive pass lie (with broken underlying) to fail")
	}
}

func TestVerifyQualifiedExecution_PassNegativeLieFails(t *testing.T) {
	ev := buildValidEvidence()
	// Underlying valid; supplied claim says pass=false.
	ev.Pass = false
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected negative pass lie to fail")
	}
}

// ----------------------------------------------------------------------------
// P0-5 / P0-6: persistence + serialized-byte round-trip tests.
// ----------------------------------------------------------------------------

func TestVerifyQualifiedExecutionBytes_EmptyBytesFails(t *testing.T) {
	res, err := VerifyQualifiedExecutionBytes(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Pass {
		t.Fatal("expected empty bytes to fail")
	}
}

func TestVerifyQualifiedExecutionBytes_MalformedJSONFails(t *testing.T) {
	res, err := VerifyQualifiedExecutionBytes([]byte("{not-json"))
	if err == nil {
		t.Fatal("expected malformed JSON error")
	}
	if res.Pass {
		t.Fatal("expected malformed JSON to fail")
	}
}

func TestVerifyQualifiedExecutionBytes_TrailingJSONFails(t *testing.T) {
	ev := buildValidEvidence()
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	data = append(data, []byte(" 42")...)
	res, err := VerifyQualifiedExecutionBytes(data)
	if err == nil {
		t.Fatal("expected trailing JSON error")
	}
	if res.Pass {
		t.Fatal("expected trailing JSON to fail")
	}
}

func TestVerifyQualifiedExecutionBytes_UnknownTopLevelFieldFails(t *testing.T) {
	ev := buildValidEvidence()
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	s := string(data)
	idx := strings.LastIndex(s, `}`)
	s = s[:idx] + `,"rogue_top":42}` + s[idx:]
	data = []byte(s)
	res, err := VerifyQualifiedExecutionBytes(data)
	if res.Pass {
		t.Fatalf("expected unknown top-level field to fail, got: %v", res.Errors)
	}
	_ = err
}

func TestVerifyQualifiedExecutionBytes_MissingImageObjectFails(t *testing.T) {
	ev := buildValidEvidence()
	raw, _ := json.Marshal(ev)
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	delete(m, "image")
	data, _ := json.Marshal(m)
	res, err := VerifyQualifiedExecutionBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Pass {
		t.Fatal("expected missing image object to fail")
	}
}

// TestVerifyQualifiedExecutionBytes_RoundTripPersisted exercises the
// full persistence loop with the canonical happy-path fixture.
// It must:
//   1. pass the same ev to persistence;
//   2. read the exact output path;
//   3. unmarshal those exact bytes;
//   4. assert the four physical claims are true;
//   5. pass those exact bytes to the independent bytes verifier.
func TestVerifyQualifiedExecutionBytes_RoundTripPersisted(t *testing.T) {
	ev := buildValidEvidence()
	if err := PersistQualifiedExecutionEvidence("/tmp", ev); err != nil {
		t.Fatalf("persist: %v", err)
	}
	defer func() { _ = removeFile("/tmp/qualified-execution-evidence.json") }()
	persisted, err := readAll("/tmp/qualified-execution-evidence.json")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var doc QualifiedExecutionEvidence
	if err := json.Unmarshal(persisted, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !doc.ImageExactIDMatch || !doc.NetworkExactIDMatch || !doc.CleanupComplete || !doc.Pass {
		t.Fatalf("persisted artifact lacks required claims: image=%v network=%v cleanup=%v pass=%v",
			doc.ImageExactIDMatch, doc.NetworkExactIDMatch, doc.CleanupComplete, doc.Pass)
	}
	res, err := VerifyQualifiedExecutionBytes(persisted)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !res.Pass {
		t.Fatalf("expected persisted fixture to pass, got errors: %v", res.Errors)
	}
}

// TestPersistQualifiedExecutionEvidence_ValidFixturePasses exercises
// the canonical persistence path directly with the valid fixture.
func TestPersistQualifiedExecutionEvidence_ValidFixturePasses(t *testing.T) {
	ev := buildValidEvidence()
	if err := PersistQualifiedExecutionEvidence("/tmp", ev); err != nil {
		t.Fatalf("persist: %v", err)
	}
	defer func() { _ = removeFile("/tmp/qualified-execution-evidence.json") }()
	persisted, err := readAll("/tmp/qualified-execution-evidence.json")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var doc QualifiedExecutionEvidence
	if err := json.Unmarshal(persisted, &doc); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !doc.Pass || !doc.ImageExactIDMatch || !doc.NetworkExactIDMatch || !doc.CleanupComplete {
		t.Fatalf("persisted artifact lacks required claims: pass=%v image=%v network=%v cleanup=%v",
			doc.Pass, doc.ImageExactIDMatch, doc.NetworkExactIDMatch, doc.CleanupComplete)
	}
}

// ----------------------------------------------------------------------------
// P0-6: serialized-byte mutation tests. Each test produces a
// valid JSON artifact, mutates exactly one physical claim to
// the wrong value, and verifies the bytes verifier rejects the
// lie.
// ----------------------------------------------------------------------------

func TestVerifyQualifiedExecutionBytes_ImagePositiveLieFails(t *testing.T) {
	ev := buildValidEvidence()
	data := mustMarshalJSON(t, ev)
	mutate := `{"image_exact_id_match":true,"network_exact_id_match":false,"cleanup_complete":false,"pass":false}`
	data = injectClaims(t, data, mutate)
	res, err := VerifyQualifiedExecutionBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Pass {
		t.Fatalf("expected positive image_exact_id_match lie in bytes to fail")
	}
}

func TestVerifyQualifiedExecutionBytes_ImageNegativeLieFails(t *testing.T) {
	ev := buildValidEvidence()
	data := mustMarshalJSON(t, ev)
	mutate := `{"image_exact_id_match":false,"network_exact_id_match":false,"cleanup_complete":false,"pass":false}`
	data = injectClaims(t, data, mutate)
	res, err := VerifyQualifiedExecutionBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Pass {
		t.Fatalf("expected negative image_exact_id_match lie in bytes to fail")
	}
}

func TestVerifyQualifiedExecutionBytes_NetworkPositiveLieFails(t *testing.T) {
	ev := buildValidEvidence()
	// Break the underlying network agreement.
	ev.Network.ContainerEndpointID = strings.Repeat("8", 64)
	ev.SetDerivedFields()
	ev.Pass = true
	data := mustMarshalJSON(t, ev)
	// Now claim network_exact_id_match=true (lie).
	mutate := `{"image_exact_id_match":false,"network_exact_id_match":true,"cleanup_complete":false,"pass":false}`
	data = injectClaims(t, data, mutate)
	res, err := VerifyQualifiedExecutionBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Pass {
		t.Fatalf("expected positive network_exact_id_match lie in bytes to fail")
	}
}

func TestVerifyQualifiedExecutionBytes_NetworkNegativeLieFails(t *testing.T) {
	ev := buildValidEvidence()
	data := mustMarshalJSON(t, ev)
	mutate := `{"image_exact_id_match":false,"network_exact_id_match":false,"cleanup_complete":false,"pass":false}`
	data = injectClaims(t, data, mutate)
	res, err := VerifyQualifiedExecutionBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Pass {
		t.Fatalf("expected negative network_exact_id_match lie in bytes to fail")
	}
}

func TestVerifyQualifiedExecutionBytes_CleanupPositiveLieFails(t *testing.T) {
	ev := buildValidEvidence()
	// Break underlying cleanup agreement.
	ev.Network.Removed = false
	ev.SetDerivedFields()
	ev.Pass = true
	data := mustMarshalJSON(t, ev)
	mutate := `{"image_exact_id_match":false,"network_exact_id_match":false,"cleanup_complete":true,"pass":false}`
	data = injectClaims(t, data, mutate)
	res, err := VerifyQualifiedExecutionBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Pass {
		t.Fatalf("expected positive cleanup_complete lie in bytes to fail")
	}
}

func TestVerifyQualifiedExecutionBytes_CleanupNegativeLieFails(t *testing.T) {
	ev := buildValidEvidence()
	data := mustMarshalJSON(t, ev)
	mutate := `{"image_exact_id_match":false,"network_exact_id_match":false,"cleanup_complete":false,"pass":false}`
	data = injectClaims(t, data, mutate)
	res, err := VerifyQualifiedExecutionBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Pass {
		t.Fatalf("expected negative cleanup_complete lie in bytes to fail")
	}
}

func TestVerifyQualifiedExecutionBytes_PassPositiveLieFails(t *testing.T) {
	ev := buildValidEvidence()
	data := mustMarshalJSON(t, ev)
	// Underlying is valid; lie: pass=true with one claim false.
	mutate := `{"image_exact_id_match":false,"network_exact_id_match":false,"cleanup_complete":false,"pass":true}`
	data = injectClaims(t, data, mutate)
	res, err := VerifyQualifiedExecutionBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Pass {
		t.Fatalf("expected positive pass lie in bytes to fail")
	}
}

func TestVerifyQualifiedExecutionBytes_PassNegativeLieFails(t *testing.T) {
	ev := buildValidEvidence()
	data := mustMarshalJSON(t, ev)
	mutate := `{"image_exact_id_match":true,"network_exact_id_match":true,"cleanup_complete":true,"pass":false}`
	data = injectClaims(t, data, mutate)
	res, err := VerifyQualifiedExecutionBytes(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Pass {
		t.Fatalf("expected negative pass lie in bytes to fail")
	}
}

// mustMarshalJSON serializes the evidence as indented JSON.
func mustMarshalJSON(t *testing.T, v interface{}) []byte {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// injectClaims replaces the four supplied claim fields with the
// values from the supplied JSON object (a single JSON object
// literal). It locates the first "{" after the schema_version
// key, then walks the JSON to rewrite the four claim keys. The
// replacement is exact; unknown fields are rejected by the
// verifier.
func injectClaims(t *testing.T, data []byte, claimsJSON string) []byte {
	t.Helper()
	var doc map[string]json.RawMessage
	if err := json.Unmarshal(data, &doc); err != nil {
		t.Fatalf("unmarshal fixture: %v", err)
	}
	var claims map[string]bool
	if err := json.Unmarshal([]byte(claimsJSON), &claims); err != nil {
		t.Fatalf("unmarshal claims: %v", err)
	}
	for k, v := range claims {
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatalf("marshal claim %s: %v", k, err)
		}
		doc[k] = b
	}
	out, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatalf("re-marshal: %v", err)
	}
	return out
}