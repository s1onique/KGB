// qualified_execution_test.go — Table-driven rejection class matrix
// for the qualified execution evidence.
//
// CORRECTION17: every rejection class listed in the production
// requirement is exercised with a fixture that mutates exactly one
// observation. The valid fixture covers the happy path. The bytes
// verifier exercises the serialized presence / unknown-field /
// trailing-JSON checks.

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
// every required field set. Tests use this as the starting point for
// their single-mutation fixtures.
func buildValidObservations() *dockerlab.QualifiedExecutionObservations {
	obs := &dockerlab.QualifiedExecutionObservations{
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
	}
	return obs
}

func TestVerifyQualifiedExecution_ValidFixturePasses(t *testing.T) {
	obs := buildValidObservations()
	ev := BuildEvidenceFromObservations(obs)
	ev.SetDerivedFields()
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
	ev := BuildEvidenceFromObservations(buildValidObservations())
	ev.SchemaVersion = ""
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected empty schema version to fail")
	}
}

func TestVerifyQualifiedExecution_UnsupportedSchemaVersionFails(t *testing.T) {
	ev := BuildEvidenceFromObservations(buildValidObservations())
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

func TestVerifyQualifiedExecution_PassTrueWithErrorsFails(t *testing.T) {
	obs := buildValidObservations()
	obs.Pull.AttemptCount = 1
	ev := BuildEvidenceFromObservations(obs)
	// Re-run the verifier; it should set Pass=false.
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected pull.attempt_count=1 to fail the verifier")
	}
	// Now force Pass=true; the verifier must reject the lie.
	ev.Pass = true
	res = VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected pass=true with errors to fail closed")
	}
}

func TestVerifyQualifiedExecution_ExactIDMatchWithoutBackingFails(t *testing.T) {
	obs := buildValidObservations()
	// Mutate one image observation; the implied match becomes false.
	obs.Image.ContainerInspectImage = "sha256:" + strings.Repeat("3", 64)
	ev := BuildEvidenceFromObservations(obs)
	// Manually set the derived flag to true (the lie).
	ev.ImageExactIDMatch = true
	res := VerifyQualifiedExecution(ev)
	if res.Pass {
		t.Fatal("expected image_exact_id_match=true without backing to fail")
	}
}

// VerifyQualifiedExecutionBytes presence + structural tests
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
	ev := BuildEvidenceFromObservations(buildValidObservations())
	ev.SetDerivedFields()
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Append a second JSON value.
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
	ev := BuildEvidenceFromObservations(buildValidObservations())
	ev.SetDerivedFields()
	data, err := json.Marshal(ev)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	// Inject an unknown top-level field by targeting the outermost `}`.
	s := string(data)
	idx := strings.LastIndex(s, `}`)
	s = s[:idx] + `,"rogue_top":42}` + s[idx:]
	data = []byte(s)
	res, err := VerifyQualifiedExecutionBytes(data)
	if res.Pass {
		t.Fatalf("expected unknown top-level field to fail, got: %v", res.Errors)
	}
	_ = err // the unknown-field diagnostic may live in res.Errors
}

func TestVerifyQualifiedExecutionBytes_MissingImageObjectFails(t *testing.T) {
	ev := BuildEvidenceFromObservations(buildValidObservations())
	// Manually unmarshal, drop image, re-marshal.
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

func TestVerifyQualifiedExecutionBytes_RoundTripPersisted(t *testing.T) {
	obs := buildValidObservations()
	ev := BuildEvidenceFromObservations(obs)
	// Persist and re-verify; the persisted bytes must pass.
	if err := writeFileAtomic("/tmp/q-exec-evidence-test.json", mustMarshal(ev)); err != nil {
		t.Fatalf("persist: %v", err)
	}
	defer func() { _ = osRemove("/tmp/q-exec-evidence-test.json") }()
	persisted, err := readFile("/tmp/q-exec-evidence-test.json")
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	res, err := VerifyQualifiedExecutionBytes(persisted)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !res.Pass {
		t.Fatalf("expected persisted fixture to pass, got errors: %v", res.Errors)
	}
}

func mustMarshal(v interface{}) []byte {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		panic(err)
	}
	return b
}

// osRemove and readFile are small helpers used by the persisted
// round-trip test. They are package-private to avoid adding a new
// file just for the helpers.
func osRemove(path string) error { return removeFile(path) }
func readFile(path string) ([]byte, error) { return readAll(path) }
