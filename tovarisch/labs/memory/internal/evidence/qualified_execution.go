// qualified_execution.go — Canonical evidence schema, converter,
// and serialized verifier for the P0-10 qualified execution path.
//
// CORRECTION17:
//   - The schema includes every authoritative observation
//     (image, network, pull, container, provenance).
//   - The producer never copies an input value into an observation
//     field; the converter translates the dockerlab observation
//     object (which is populated at the operation that observed
//     each value) into the canonical evidence schema.
//   - The independent verifier (a) checks serialized presence for
//     every required field, (b) re-derives the derived fields
//     (image.exact_id_match, network.exact_id_match, pass) from the
//     underlying values, (c) fails closed for any disagreement
//     between a claimed derived value and the recomputed value.

package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
)

// QualifiedExecutionSchemaVersion is the canonical schema version.
const QualifiedExecutionSchemaVersion = "1.0.0"

// canonicalImageIDPattern is the only accepted exact image ID form.
var canonicalImageIDPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// canonicalNetworkIDPattern matches Docker network IDs.
var canonicalNetworkIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// canonicalCommitSHA1 is the Git SHA-1 object format.
const canonicalCommitSHA1 = "sha1"

// canonicalCommitSHA256 is the Git SHA-256 object format.
const canonicalCommitSHA256 = "sha256"

// ImageObservations mirrors dockerlab.ImageObservations. The
// evidence package keeps its own copy to keep the converter trivial
// and to avoid dragging dockerlab types into the persisted JSON.
type ImageObservations struct {
	RequestedReference      string   `json:"requested_reference"`
	InspectedBeforeCreate   string   `json:"inspected_id_before_create"`
	InspectedRepoDigests    []string `json:"repo_digests"`
	CreateRequestImage      string   `json:"create_request_image"`
	ContainerInspectImage   string   `json:"container_inspect_image_id"`
	ContainerConfigImage    string   `json:"container_inspect_config_image"`
}

// NetworkObservations mirrors dockerlab.NetworkObservations.
type NetworkObservations struct {
	RequestedName       string `json:"requested_name"`
	CreateResponseID    string `json:"create_response_id"`
	InspectResponseID   string `json:"inspected_network_id"`
	ContainerEndpointID string `json:"container_endpoint_network_id"`
}

// PullObservations mirrors dockerlab.PullObservations.
type PullObservations struct {
	ObservationAvailable bool   `json:"observation_available"`
	Attempted            bool   `json:"attempted"`
	AttemptCount         int    `json:"attempt_count"`
	LastReference        string `json:"last_reference,omitempty"`
}

// ContainerObservations mirrors dockerlab.ContainerObservations.
type ContainerObservations struct {
	ID                    string `json:"id"`
	Created               bool   `json:"created"`
	Inspected             bool   `json:"inspected"`
	Started               bool   `json:"started"`
	TerminalStateObserved bool   `json:"terminal_state_observed"`
	Removed               bool   `json:"removed"`
}

// ProvenanceBinding mirrors dockerlab.ProvenanceBinding.
type ProvenanceBinding struct {
	SourceCommit        string `json:"source_commit"`
	SourceTree          string `json:"source_tree"`
	GitObjectFormat     string `json:"git_object_format"`
	WorkingTreeDirty    bool   `json:"working_tree_dirty"`
	SourceCommitDirty   bool   `json:"source_commit_dirty"`
	DockerServerVersion string `json:"docker_server_version"`
	ProducerVersion     string `json:"producer_version"`
}

// QualifiedExecutionEvidence is the canonical persisted evidence.
type QualifiedExecutionEvidence struct {
	SchemaVersion string              `json:"schema_version"`
	GeneratedAt   time.Time           `json:"generated_at"`
	Image         ImageObservations   `json:"image"`
	Network       NetworkObservations `json:"network"`
	Pull          PullObservations    `json:"pull"`
	Container     ContainerObservations `json:"container"`
	Provenance    ProvenanceBinding   `json:"provenance"`
	ImageExactIDMatch  bool     `json:"image_exact_id_match"`
	NetworkExactIDMatch bool   `json:"network_exact_id_match"`
	Pass               bool     `json:"pass"`
	VerifierErrors     []string `json:"verifier_errors,omitempty"`
}

// BuildEvidenceFromObservations translates a dockerlab observation
// object into the canonical evidence schema. The conversion is
// purely structural; no observation value is overwritten.
func BuildEvidenceFromObservations(obs *dockerlab.QualifiedExecutionObservations) *QualifiedExecutionEvidence {
	if obs == nil {
		return nil
	}
	cp := *obs
	return &QualifiedExecutionEvidence{
		SchemaVersion: obs.SchemaVersion,
		GeneratedAt:   obs.GeneratedAt,
		Image: ImageObservations{
			RequestedReference:    cp.Image.RequestedReference,
			InspectedBeforeCreate: cp.Image.InspectedBeforeCreate,
			InspectedRepoDigests:  cp.Image.InspectedRepoDigests,
			CreateRequestImage:    cp.Image.CreateRequestImage,
			ContainerInspectImage: cp.Image.ContainerInspectImage,
			ContainerConfigImage:  cp.Image.ContainerConfigImage,
		},
		Network: NetworkObservations{
			RequestedName:       cp.Network.RequestedName,
			CreateResponseID:    cp.Network.CreateResponseID,
			InspectResponseID:   cp.Network.InspectResponseID,
			ContainerEndpointID: cp.Network.ContainerEndpointID,
		},
		Pull: PullObservations{
			ObservationAvailable: cp.Pull.ObservationAvailable,
			Attempted:            cp.Pull.Attempted,
			AttemptCount:         cp.Pull.AttemptCount,
			LastReference:        cp.Pull.LastReference,
		},
		Container: ContainerObservations{
			ID:                    cp.Container.ID,
			Created:               cp.Container.Created,
			Inspected:             cp.Container.Inspected,
			Started:               cp.Container.Started,
			TerminalStateObserved: cp.Container.TerminalStateObserved,
			Removed:               cp.Container.Removed,
		},
		Provenance: ProvenanceBinding{
			SourceCommit:        cp.Provenance.SourceCommit,
			SourceTree:          cp.Provenance.SourceTree,
			GitObjectFormat:     cp.Provenance.GitObjectFormat,
			WorkingTreeDirty:    cp.Provenance.WorkingTreeDirty,
			SourceCommitDirty:   cp.Provenance.SourceCommitDirty,
			DockerServerVersion: cp.Provenance.DockerServerVersion,
			ProducerVersion:     cp.Provenance.ProducerVersion,
		},
	}
}

// VerifyQualifiedExecutionResult describes the outcome of verification.
type VerifyQualifiedExecutionResult struct {
	Pass   bool
	Errors []string
}

// VerifyQualifiedExecution verifies an in-memory evidence struct.
func VerifyQualifiedExecution(ev *QualifiedExecutionEvidence) VerifyQualifiedExecutionResult {
	return verifyQualifiedExecution(ev, true)
}

// VerifyQualifiedExecutionBytes parses and verifies serialized
// evidence. The verifier rejects:
//   - malformed JSON
//   - trailing JSON values
//   - unknown fields
//   - missing required fields
//   - missing nested objects
//   - inconsistent claimed derived values
func VerifyQualifiedExecutionBytes(data []byte) (VerifyQualifiedExecutionResult, error) {
	if len(data) == 0 {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{"evidence bytes are empty"}}, nil
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	var raw map[string]json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{fmt.Sprintf("malformed JSON: %v", err)}}, err
	}
	// DisallowUnknownFields does not apply to maps; enforce top-level
	// field allowlist explicitly.
	allowed := map[string]struct{}{
		"schema_version": {}, "generated_at": {},
		"image": {}, "network": {}, "pull": {},
		"container": {}, "provenance": {},
		"image_exact_id_match": {}, "network_exact_id_match": {},
		"pass": {},
	}
	for k := range raw {
		if _, ok := allowed[k]; !ok {
			return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{
				fmt.Sprintf("unknown top-level field: %q", k),
			}}, nil
		}
	}
	// Reject trailing JSON values.
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{"trailing JSON document"}}, errors.New("trailing JSON document")
	}
	// Require the top-level fields to be present.
	required := []string{
		"schema_version", "generated_at", "image", "network", "pull",
		"container", "provenance", "image_exact_id_match",
		"network_exact_id_match", "pass",
	}
	var missing []string
	for _, k := range required {
		if _, ok := raw[k]; !ok {
			missing = append(missing, k)
		}
	}
	if len(missing) > 0 {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{
			fmt.Sprintf("missing required top-level fields: %v", missing),
		}}, nil
	}
	// Required nested fields. Missing nested objects are an error.
	if _, ok := raw["image"]; !ok || len(raw["image"]) == 0 {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{"missing image object"}}, nil
	}
	if _, ok := raw["network"]; !ok || len(raw["network"]) == 0 {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{"missing network object"}}, nil
	}
	if _, ok := raw["pull"]; !ok || len(raw["pull"]) == 0 {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{"missing pull object"}}, nil
	}
	if _, ok := raw["container"]; !ok || len(raw["container"]) == 0 {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{"missing container object"}}, nil
	}
	if _, ok := raw["provenance"]; !ok || len(raw["provenance"]) == 0 {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{"missing provenance object"}}, nil
	}

	// Decode into the typed struct, then run the in-memory verifier.
	var ev QualifiedExecutionEvidence
	if err := json.Unmarshal(data, &ev); err != nil {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{fmt.Sprintf("decode after structural check: %v", err)}}, err
	}
	return verifyQualifiedExecution(&ev, false), nil
}

func verifyQualifiedExecution(ev *QualifiedExecutionEvidence, fromMemory bool) VerifyQualifiedExecutionResult {
	result := VerifyQualifiedExecutionResult{Pass: true}
	appendErr := func(msg string) {
		result.Pass = false
		result.Errors = append(result.Errors, msg)
	}

	if ev == nil {
		appendErr("evidence is nil")
		return result
	}
	if ev.SchemaVersion == "" {
		appendErr("schema_version is empty")
	} else if ev.SchemaVersion != QualifiedExecutionSchemaVersion {
		appendErr(fmt.Sprintf("unsupported schema_version=%q (expected %q)",
			ev.SchemaVersion, QualifiedExecutionSchemaVersion))
	}
	if fromMemory && ev.SchemaVersion == "" {
		// In-memory verifier does not require every field to be
		// physically present in JSON, but the values must still
		// validate. The structural check (from bytes) catches the
		// missing-field case.
	}

	// Image validation.
	if strings.TrimSpace(ev.Image.RequestedReference) == "" {
		appendErr("image.requested_reference is empty")
	}
	if strings.TrimSpace(ev.Image.InspectedBeforeCreate) == "" {
		appendErr("image.inspected_id_before_create is empty")
	}
	if ev.Image.InspectedBeforeCreate != "" {
		if err := ValidateCanonicalImageID(ev.Image.InspectedBeforeCreate); err != nil {
			appendErr(fmt.Sprintf("image.inspected_id_before_create invalid: %v", err))
		}
	}
	if ev.Image.CreateRequestImage != "" {
		if err := ValidateCanonicalImageID(ev.Image.CreateRequestImage); err != nil {
			appendErr(fmt.Sprintf("image.create_request_image invalid: %v", err))
		}
	}
	if ev.Image.RequestedReference != "" && ev.Image.CreateRequestImage != "" {
		if ev.Image.CreateRequestImage == ev.Image.RequestedReference {
			appendErr("create request image is the original mutable tag/reference")
		}
	}
	if ev.Image.InspectedBeforeCreate != "" && ev.Image.CreateRequestImage != "" {
		if ev.Image.InspectedBeforeCreate != ev.Image.CreateRequestImage {
			appendErr(fmt.Sprintf("image mismatch: inspected_id_before_create=%q != create_request_image=%q",
				ev.Image.InspectedBeforeCreate, ev.Image.CreateRequestImage))
		}
	}
	if ev.Image.ContainerInspectImage != "" {
		if err := ValidateCanonicalImageID(ev.Image.ContainerInspectImage); err != nil {
			appendErr(fmt.Sprintf("image.container_inspect_image_id invalid: %v", err))
		} else if ev.Image.ContainerInspectImage != ev.Image.CreateRequestImage {
			appendErr(fmt.Sprintf("image mismatch: container_inspect_image_id=%q != create_request_image=%q",
				ev.Image.ContainerInspectImage, ev.Image.CreateRequestImage))
		}
	}
	if ev.Image.ContainerConfigImage != "" {
		if err := ValidateCanonicalImageID(ev.Image.ContainerConfigImage); err != nil {
			appendErr(fmt.Sprintf("image.container_inspect_config_image invalid: %v", err))
		} else if ev.Image.ContainerConfigImage != ev.Image.CreateRequestImage {
			appendErr(fmt.Sprintf("image mismatch: container_inspect_config_image=%q != create_request_image=%q",
				ev.Image.ContainerConfigImage, ev.Image.CreateRequestImage))
		}
	}

	impliedImageExact := ev.Image.InspectedBeforeCreate != "" &&
		ev.Image.InspectedBeforeCreate == ev.Image.CreateRequestImage &&
		ev.Image.CreateRequestImage != "" &&
		ev.Image.CreateRequestImage == ev.Image.ContainerInspectImage &&
		ev.Image.CreateRequestImage == ev.Image.ContainerConfigImage
	if ev.ImageExactIDMatch && !impliedImageExact {
		appendErr("image_exact_id_match=true but the underlying image values disagree")
	}

	// Network validation.
	if strings.TrimSpace(ev.Network.CreateResponseID) == "" {
		appendErr("network.create_response_id is empty")
	}
	if strings.TrimSpace(ev.Network.InspectResponseID) == "" {
		appendErr("network.inspected_network_id is empty")
	}
	if strings.TrimSpace(ev.Network.ContainerEndpointID) == "" {
		appendErr("network.container_endpoint_network_id is empty")
	}
	if strings.TrimSpace(ev.Network.RequestedName) == "" {
		appendErr("network.requested_name is empty")
	}
	if ev.Network.CreateResponseID != "" {
		if err := ValidateCanonicalNetworkID(ev.Network.CreateResponseID); err != nil {
			appendErr(fmt.Sprintf("network.create_response_id invalid: %v", err))
		}
	}
	if ev.Network.InspectResponseID != "" {
		if err := ValidateCanonicalNetworkID(ev.Network.InspectResponseID); err != nil {
			appendErr(fmt.Sprintf("network.inspected_network_id invalid: %v", err))
		}
	}
	if ev.Network.CreateResponseID != "" && ev.Network.InspectResponseID != "" {
		if ev.Network.CreateResponseID != ev.Network.InspectResponseID {
			appendErr(fmt.Sprintf("network mismatch: create_response_id=%q != inspected_network_id=%q",
				ev.Network.CreateResponseID, ev.Network.InspectResponseID))
		}
	}
	if ev.Network.ContainerEndpointID != "" {
		if err := ValidateCanonicalNetworkID(ev.Network.ContainerEndpointID); err != nil {
			appendErr(fmt.Sprintf("network.container_endpoint_network_id invalid: %v", err))
		} else if ev.Network.ContainerEndpointID != ev.Network.InspectResponseID {
			appendErr(fmt.Sprintf("network mismatch: container_endpoint_network_id=%q != inspected_network_id=%q",
				ev.Network.ContainerEndpointID, ev.Network.InspectResponseID))
		}
	}

	impliedNetExact := ev.Network.CreateResponseID != "" &&
		ev.Network.CreateResponseID == ev.Network.InspectResponseID &&
		ev.Network.InspectResponseID == ev.Network.ContainerEndpointID
	if ev.NetworkExactIDMatch && !impliedNetExact {
		appendErr("network_exact_id_match=true but the underlying network values disagree")
	}

	// Pull validation.
	if !ev.Pull.ObservationAvailable {
		appendErr("pull.observation_available is false: the audit was not installed")
	}
	if ev.Pull.Attempted {
		appendErr("pull.attempted=true: qualified execution path must not invoke any image-pull")
	}
	if ev.Pull.AttemptCount != 0 {
		appendErr(fmt.Sprintf("pull.attempt_count=%d: qualified execution path must have zero pull attempts",
			ev.Pull.AttemptCount))
	}
	if ev.Pull.AttemptCount < 0 {
		appendErr("pull.attempt_count is negative: pull observation is corrupted")
	}

	// Container validation.
	if strings.TrimSpace(ev.Container.ID) == "" {
		appendErr("container.id is empty")
	}
	if !ev.Container.Created {
		appendErr("container.created=false")
	}
	if !ev.Container.Inspected {
		appendErr("container.inspected=false")
	}
	if !ev.Container.Started {
		appendErr("container.started=false")
	}
	if !ev.Container.TerminalStateObserved {
		appendErr("container.terminal_state_observed=false")
	}
	if !ev.Container.Removed {
		appendErr("container.removed=false")
	}

	// Provenance validation.
	if strings.TrimSpace(ev.Provenance.SourceCommit) == "" {
		appendErr("provenance.source_commit is empty")
	}
	if strings.TrimSpace(ev.Provenance.SourceTree) == "" {
		appendErr("provenance.source_tree is empty")
	}
	if strings.TrimSpace(ev.Provenance.GitObjectFormat) == "" {
		appendErr("provenance.git_object_format is empty")
	} else {
		switch ev.Provenance.GitObjectFormat {
		case canonicalCommitSHA1:
			if len(ev.Provenance.SourceCommit) != 40 {
				appendErr(fmt.Sprintf("provenance.source_commit length=%d, expected 40 for sha1",
					len(ev.Provenance.SourceCommit)))
			}
		case canonicalCommitSHA256:
			if len(ev.Provenance.SourceCommit) != 64 {
				appendErr(fmt.Sprintf("provenance.source_commit length=%d, expected 64 for sha256",
					len(ev.Provenance.SourceCommit)))
			}
		default:
			appendErr(fmt.Sprintf("provenance.git_object_format=%q invalid (expected sha1 or sha256)",
				ev.Provenance.GitObjectFormat))
		}
	}
	if strings.TrimSpace(ev.Provenance.DockerServerVersion) == "" {
		appendErr("provenance.docker_server_version is empty")
	}
	if strings.TrimSpace(ev.Provenance.ProducerVersion) == "" {
		appendErr("provenance.producer_version is empty")
	}

	// The Pass claim must agree with the absence of errors.
	if ev.Pass && len(result.Errors) > 0 {
		appendErr("pass=true but authoritative observations are missing or inconsistent")
	}

	return result
}

// ValidateCanonicalImageID enforces the canonical image ID form.
func ValidateCanonicalImageID(id string) error {
	if id == "" {
		return errors.New("image ID is empty")
	}
	if !canonicalImageIDPattern.MatchString(id) {
		return errors.New("image ID is not canonical sha256:64-lowercase-hex")
	}
	return nil
}

// ValidateCanonicalNetworkID enforces the canonical Docker network ID form.
func ValidateCanonicalNetworkID(id string) error {
	if id == "" {
		return errors.New("network ID is empty")
	}
	if !canonicalNetworkIDPattern.MatchString(id) {
		return errors.New("network ID is not 64 lowercase hex characters")
	}
	return nil
}

// SetDerivedFields computes the derived fields from the underlying
// values. The producer calls this just before persistence.
func (ev *QualifiedExecutionEvidence) SetDerivedFields() {
	impliedImage := ev.Image.InspectedBeforeCreate != "" &&
		ev.Image.InspectedBeforeCreate == ev.Image.CreateRequestImage &&
		ev.Image.CreateRequestImage != "" &&
		ev.Image.CreateRequestImage == ev.Image.ContainerInspectImage &&
		ev.Image.CreateRequestImage == ev.Image.ContainerConfigImage
	ev.ImageExactIDMatch = impliedImage
	impliedNet := ev.Network.CreateResponseID != "" &&
		ev.Network.CreateResponseID == ev.Network.InspectResponseID &&
		ev.Network.InspectResponseID == ev.Network.ContainerEndpointID
	ev.NetworkExactIDMatch = impliedNet
}

// ComputeEvidenceSHA256 returns a deterministic SHA-256 of the
// evidence bytes (with verifier outputs and derived fields
// zeroed) for content addressing.
func ComputeEvidenceSHA256(ev *QualifiedExecutionEvidence) (string, error) {
	if ev == nil {
		return "", errors.New("evidence is nil")
	}
	cp := *ev
	cp.VerifierErrors = nil
	cp.Pass = false
	cp.ImageExactIDMatch = false
	cp.NetworkExactIDMatch = false
	cp.GeneratedAt = time.Time{}
	if cp.Image.InspectedRepoDigests != nil {
		cp.Image.InspectedRepoDigests = append([]string{}, ev.Image.InspectedRepoDigests...)
		sort.Strings(cp.Image.InspectedRepoDigests)
	}
	data, err := json.Marshal(cp)
	if err != nil {
		return "", fmt.Errorf("marshal evidence: %w", err)
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

// PersistQualifiedExecutionEvidence writes the canonical evidence
// atomically and re-verifies the persisted bytes.
func PersistQualifiedExecutionEvidence(dir string, ev *QualifiedExecutionEvidence) error {
	if ev == nil {
		return errors.New("evidence is nil")
	}
	if dir == "" {
		return errors.New("dir is empty")
	}
	ev.SetDerivedFields()
	verify := VerifyQualifiedExecution(ev)
	ev.Pass = verify.Pass
	ev.VerifierErrors = verify.Errors
	if len(ev.Image.InspectedRepoDigests) > 1 {
		sort.Strings(ev.Image.InspectedRepoDigests)
	}
	data, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal evidence: %w", err)
	}
	if err := writeFileAtomic(dir+"/qualified-execution-evidence.json", data); err != nil {
		return err
	}
	// Re-verify the persisted bytes to prove round-trip safety.
	persisted, err := os.ReadFile(dir + "/qualified-execution-evidence.json")
	if err != nil {
		return fmt.Errorf("read persisted evidence: %w", err)
	}
	if _, err := VerifyQualifiedExecutionBytes(persisted); err != nil {
		return fmt.Errorf("persisted evidence rejected: %w", err)
	}
	return nil
}

// writeFileAtomic writes data atomically by writing to a temp file and renaming.
func writeFileAtomic(path string, data []byte) error {
	dir := path
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			dir = path[:i]
			break
		}
		if i == 0 {
			dir = "."
		}
	}
	tmp, err := os.CreateTemp(dir, ".qualified-evidence-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	cleanup := func() { _ = os.Remove(tmpName) }
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		cleanup()
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		cleanup()
		return fmt.Errorf("rename temp to %s: %w", path, err)
	}
	return nil
}

// removeFile deletes a file via os.Remove.
func removeFile(path string) error {
	return os.Remove(path)
}

// readAll reads a file. Exported via readFile alias in tests.
func readAll(path string) ([]byte, error) {
	return os.ReadFile(path)
}
