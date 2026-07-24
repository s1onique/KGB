// qualified_execution.go — Canonical evidence schema, converter,
// and serialized verifier for the P0-10 qualified execution path.
//
// CORRECTION22 (this file):
//   - The verifier is split into THREE independent functions with
//     distinct responsibilities:
//       1. verifyUnderlyingObservations(ev) — validates raw
//          observations ONLY. It does not read or modify the
//          supplied claim fields (image_exact_id_match,
//          network_exact_id_match, cleanup_complete, pass,
//          verifier_errors). It does not call SetDerivedFields.
//       2. deriveClaims(ev, underlying) — pure function that
//          computes the four implied claim values from the raw
//          observations. It does NOT mutate ev.
//       3. VerifyQualifiedExecution(ev) — the COMPLETE verifier.
//          It calls (1), then (2), then compares every supplied
//          claim to the derived value and rejects disagreements.
//   - PersistQualifiedExecutionEvidence uses the new sequence:
//     underlying -> derived -> stamp -> marshal -> atomic write
//     -> read back -> VerifyQualifiedExecutionBytes -> physical
//     claim checks. The physical claim check requires the
//     persisted artifact to literally contain pass:true plus the
//     three derived claim values.
//   - Nested JSON verification uses an explicit allowlist per
//     object; unknown fields are rejected.

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

const (
	gitObjectFormatSHA1   = "sha1"
	gitObjectFormatSHA256 = "sha256"
)

// canonicalImageIDPattern is the only accepted exact image ID form.
var canonicalImageIDPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// canonicalNetworkIDPattern matches Docker network IDs.
var canonicalNetworkIDPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// sha1Hex40 matches 40 lowercase hex characters.
var sha1Hex40 = regexp.MustCompile(`^[0-9a-f]{40}$`)

// sha256Hex64 matches 64 lowercase hex characters.
var sha256Hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

// sha256HexPattern matches 64 lowercase hex characters (no algorithm prefix).
var sha256HexPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ImageObservations mirrors dockerlab.ImageObservations.
type ImageObservations struct {
	RequestedReference    string   `json:"requested_reference"`
	InspectedBeforeCreate string   `json:"inspected_id_before_create"`
	InspectedRepoDigests  []string `json:"repo_digests"`
	CreateRequestImage    string   `json:"create_request_image"`
	ContainerInspectImage string   `json:"container_inspect_image_id"`
	ContainerConfigImage  string   `json:"container_inspect_config_image"`
}

// NetworkObservations mirrors dockerlab.NetworkObservations.
type NetworkObservations struct {
	RequestedName       string `json:"requested_name"`
	CreateResponseID    string `json:"create_response_id"`
	InspectResponseID   string `json:"inspected_network_id"`
	ContainerEndpointID string `json:"container_endpoint_network_id"`
	Removed             bool   `json:"removed"`
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
	VCSModified         bool   `json:"vcs_modified"`
	DockerServerVersion string `json:"docker_server_version"`
	ProducerVersion     string `json:"producer_version"`
	ExecutableSHA256    string `json:"executable_sha256"`
}

// QualifiedExecutionEvidence is the canonical persisted evidence.
type QualifiedExecutionEvidence struct {
	SchemaVersion string                `json:"schema_version"`
	GeneratedAt   time.Time             `json:"generated_at"`
	Image         ImageObservations     `json:"image"`
	Network       NetworkObservations   `json:"network"`
	Pull          PullObservations      `json:"pull"`
	Container     ContainerObservations `json:"container"`
	Provenance    ProvenanceBinding     `json:"provenance"`

	// Supplied claims (the producer stamps these after the
	// verifier authorizes pass=true). The verifier compares them
	// against deriveClaims.
	ImageExactIDMatch   bool     `json:"image_exact_id_match"`
	NetworkExactIDMatch bool     `json:"network_exact_id_match"`
	CleanupComplete     bool     `json:"cleanup_complete"`
	Pass                bool     `json:"pass"`
	VerifierErrors      []string `json:"verifier_errors,omitempty"`
}

// BuildEvidenceFromObservations translates a dockerlab observation
// object into the canonical evidence schema. The derived claim
// fields are NOT populated by this constructor; the producer must
// call SetDerivedFields or PersistQualifiedExecutionEvidence.
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
			Removed:             cp.Network.Removed,
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
			VCSModified:         cp.Provenance.VCSModified,
			DockerServerVersion: cp.Provenance.DockerServerVersion,
			ProducerVersion:     cp.Provenance.ProducerVersion,
			ExecutableSHA256:    cp.Provenance.ExecutableSHA256,
		},
	}
}

// VerifyQualifiedExecutionResult describes the outcome of verification.
type VerifyQualifiedExecutionResult struct {
	Pass   bool
	Errors []string
}

// VerificationError carries a list of verifier errors and renders as
// the joined message. The canonical persistence path uses this
// type so callers can distinguish "verified OK" from "verifier
// rejected" from "structural parse error".
type VerificationError struct {
	Errors []string
}

func (e *VerificationError) Error() string {
	return fmt.Sprintf("qualified execution evidence rejected: %v", e.Errors)
}

// VerifyQualifiedExecution is the COMPLETE verifier. It performs
// three phases:
//
//  1. verifyUnderlyingObservations(ev) — independent validator
//     over raw observations only.
//  2. deriveClaims(ev, underlying) — pure derivation of the
//     four implied claim values.
//  3. Compare every supplied claim against the derived value;
//     reject every disagreement.
//
// The complete verifier never calls SetDerivedFields and never
// reads the supplied claims during phase 1.
func VerifyQualifiedExecution(ev *QualifiedExecutionEvidence) VerifyQualifiedExecutionResult {
	if ev == nil {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{"evidence is nil"}}
	}
	if ev.SchemaVersion == "" {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{"schema_version is empty"}}
	}
	if ev.SchemaVersion != QualifiedExecutionSchemaVersion {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{
			fmt.Sprintf("unsupported schema_version=%q (expected %q)",
				ev.SchemaVersion, QualifiedExecutionSchemaVersion),
		}}
	}

	// Phase 1: independent validator over raw observations.
	underlying := verifyUnderlyingObservations(ev)
	if !underlying.Pass {
		return underlying
	}

	// Phase 2: derive the implied claim values from the raw
	// observations. The function is pure and does not mutate ev.
	derived := deriveClaims(ev, underlying)

	// Phase 3: compare every supplied claim against the derived
	// value. Any disagreement is a fail-closed signal.
	result := VerifyQualifiedExecutionResult{Pass: true}
	appendErr := func(msg string) {
		result.Pass = false
		result.Errors = append(result.Errors, msg)
	}
	if ev.ImageExactIDMatch != derived.ImageExactIDMatch {
		appendErr(fmt.Sprintf("image_exact_id_match mismatch: claimed=%v derived=%v",
			ev.ImageExactIDMatch, derived.ImageExactIDMatch))
	}
	if ev.NetworkExactIDMatch != derived.NetworkExactIDMatch {
		appendErr(fmt.Sprintf("network_exact_id_match mismatch: claimed=%v derived=%v",
			ev.NetworkExactIDMatch, derived.NetworkExactIDMatch))
	}
	if ev.CleanupComplete != derived.CleanupComplete {
		appendErr(fmt.Sprintf("cleanup_complete mismatch: claimed=%v derived=%v",
			ev.CleanupComplete, derived.CleanupComplete))
	}
	if ev.Pass != derived.Pass {
		appendErr(fmt.Sprintf("pass mismatch: claimed=%v derived=%v",
			ev.Pass, derived.Pass))
	}
	// VerifierErrors must be empty for a passing artifact.
	if len(ev.VerifierErrors) > 0 {
		appendErr(fmt.Sprintf("verifier_errors is non-empty on a passing artifact: %v", ev.VerifierErrors))
	}
	return result
}

// verifyUnderlyingObservations validates the raw observations
// only. It MUST NOT read or modify any of the supplied claim
// fields:
//
//	image_exact_id_match
//	network_exact_id_match
//	cleanup_complete
//	pass
//	verifier_errors
//
// The function returns a VerifyQualifiedExecutionResult that
// reflects ONLY the consistency of the raw observations.
//
// Required semantics (CORRECTION22 P0-1):
//
//   - Image: requested_reference non-empty; inspected_id_before_create,
//     create_request_image, container_inspect_image_id, and
//     container_inspect_config_image are all canonical; precreate ==
//     create_request == container_inspect == container_config.
//   - Network: requested_name non-empty; create_response_id,
//     inspected_network_id, and container_endpoint_network_id
//     are all canonical; create == inspect == endpoint.
//   - Pull: observation_available==true; attempted==false;
//     attempt_count==0; last_reference=="".
//   - Container & cleanup: container.id non-empty; created,
//     inspected, started, terminal_state_observed, container.removed,
//     network.removed all true.
//   - Provenance: source_commit and source_tree canonical for the
//     declared git_object_format; vcs_modified, working_tree_dirty,
//     source_commit_dirty all false; docker_server_version and
//     producer_version non-empty; executable_sha256 is exactly
//     64 lowercase hex characters.
func verifyUnderlyingObservations(ev *QualifiedExecutionEvidence) VerifyQualifiedExecutionResult {
	result := VerifyQualifiedExecutionResult{Pass: true}
	appendErr := func(msg string) {
		result.Pass = false
		result.Errors = append(result.Errors, msg)
	}
	if ev == nil {
		appendErr("evidence is nil")
		return result
	}

	// -- Image observations ----------------------------------------
	if strings.TrimSpace(ev.Image.RequestedReference) == "" {
		appendErr("image.requested_reference is empty")
	}
	if strings.TrimSpace(ev.Image.InspectedBeforeCreate) == "" {
		appendErr("image.inspected_id_before_create is empty")
	} else if err := ValidateCanonicalImageID(ev.Image.InspectedBeforeCreate); err != nil {
		appendErr(fmt.Sprintf("image.inspected_id_before_create invalid: %v", err))
	}
	if strings.TrimSpace(ev.Image.CreateRequestImage) == "" {
		appendErr("image.create_request_image is empty")
	} else if err := ValidateCanonicalImageID(ev.Image.CreateRequestImage); err != nil {
		appendErr(fmt.Sprintf("image.create_request_image invalid: %v", err))
	}
	if ev.Image.CreateRequestImage != "" && ev.Image.RequestedReference != "" {
		if ev.Image.CreateRequestImage == ev.Image.RequestedReference {
			appendErr("create request image is the original mutable tag/reference")
		}
	}
	if ev.Image.InspectedBeforeCreate != "" && ev.Image.CreateRequestImage != "" {
		if ev.Image.InspectedBeforeCreate != ev.Image.CreateRequestImage {
			appendErr(fmt.Sprintf("image precreate!=create_request: inspected=%q create=%q",
				ev.Image.InspectedBeforeCreate, ev.Image.CreateRequestImage))
		}
	}
	if ev.Image.ContainerInspectImage == "" {
		appendErr("image.container_inspect_image_id is empty")
	} else {
		if err := ValidateCanonicalImageID(ev.Image.ContainerInspectImage); err != nil {
			appendErr(fmt.Sprintf("image.container_inspect_image_id invalid: %v", err))
		}
		if ev.Image.CreateRequestImage != "" && ev.Image.ContainerInspectImage != ev.Image.CreateRequestImage {
			appendErr(fmt.Sprintf("image container_inspect!=create_request: container=%q create=%q",
				ev.Image.ContainerInspectImage, ev.Image.CreateRequestImage))
		}
	}
	if ev.Image.ContainerConfigImage == "" {
		appendErr("image.container_inspect_config_image is empty")
	} else {
		if err := ValidateCanonicalImageID(ev.Image.ContainerConfigImage); err != nil {
			appendErr(fmt.Sprintf("image.container_inspect_config_image invalid: %v", err))
		}
		if ev.Image.CreateRequestImage != "" && ev.Image.ContainerConfigImage != ev.Image.CreateRequestImage {
			appendErr(fmt.Sprintf("image container_config!=create_request: config=%q create=%q",
				ev.Image.ContainerConfigImage, ev.Image.CreateRequestImage))
		}
	}

	// -- Network observations --------------------------------------
	if strings.TrimSpace(ev.Network.RequestedName) == "" {
		appendErr("network.requested_name is empty")
	}
	if strings.TrimSpace(ev.Network.CreateResponseID) == "" {
		appendErr("network.create_response_id is empty")
	} else if err := ValidateCanonicalNetworkID(ev.Network.CreateResponseID); err != nil {
		appendErr(fmt.Sprintf("network.create_response_id invalid: %v", err))
	}
	if strings.TrimSpace(ev.Network.InspectResponseID) == "" {
		appendErr("network.inspected_network_id is empty")
	} else {
		if err := ValidateCanonicalNetworkID(ev.Network.InspectResponseID); err != nil {
			appendErr(fmt.Sprintf("network.inspected_network_id invalid: %v", err))
		}
		if ev.Network.CreateResponseID != "" && ev.Network.InspectResponseID != ev.Network.CreateResponseID {
			appendErr(fmt.Sprintf("network create!=inspect: create=%q inspect=%q",
				ev.Network.CreateResponseID, ev.Network.InspectResponseID))
		}
	}
	if strings.TrimSpace(ev.Network.ContainerEndpointID) == "" {
		appendErr("network.container_endpoint_network_id is empty")
	} else {
		if err := ValidateCanonicalNetworkID(ev.Network.ContainerEndpointID); err != nil {
			appendErr(fmt.Sprintf("network.container_endpoint_network_id invalid: %v", err))
		}
		if ev.Network.InspectResponseID != "" && ev.Network.ContainerEndpointID != ev.Network.InspectResponseID {
			appendErr(fmt.Sprintf("network endpoint!=inspect: endpoint=%q inspect=%q",
				ev.Network.ContainerEndpointID, ev.Network.InspectResponseID))
		}
	}

	// -- Pull observations -----------------------------------------
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
	if ev.Pull.LastReference != "" {
		appendErr(fmt.Sprintf("pull.last_reference=%q must be empty when no pull is attempted",
			ev.Pull.LastReference))
	}

	// -- Container observations ------------------------------------
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
		appendErr("container.removed=false: container cleanup unproven")
	}
	if !ev.Network.Removed {
		appendErr("network.removed=false: network cleanup unproven")
	}

	// -- Provenance observations ------------------------------------
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
		case gitObjectFormatSHA1:
			if !sha1Hex40.MatchString(ev.Provenance.SourceCommit) {
				appendErr(fmt.Sprintf("provenance.source_commit=%q is not 40 lowercase hex characters",
					ev.Provenance.SourceCommit))
			}
			if !sha1Hex40.MatchString(ev.Provenance.SourceTree) {
				appendErr(fmt.Sprintf("provenance.source_tree=%q is not 40 lowercase hex characters",
					ev.Provenance.SourceTree))
			}
		case gitObjectFormatSHA256:
			if !sha256Hex64.MatchString(ev.Provenance.SourceCommit) {
				appendErr(fmt.Sprintf("provenance.source_commit=%q is not 64 lowercase hex characters",
					ev.Provenance.SourceCommit))
			}
			if !sha256Hex64.MatchString(ev.Provenance.SourceTree) {
				appendErr(fmt.Sprintf("provenance.source_tree=%q is not 64 lowercase hex characters",
					ev.Provenance.SourceTree))
			}
		default:
			appendErr(fmt.Sprintf("provenance.git_object_format=%q invalid (expected sha1 or sha256)",
				ev.Provenance.GitObjectFormat))
		}
	}
	if ev.Provenance.VCSModified {
		appendErr("provenance.vcs_modified=true: VCS reports the build is dirty")
	}
	if ev.Provenance.WorkingTreeDirty {
		appendErr("provenance.working_tree_dirty=true: working tree has uncommitted changes")
	}
	if ev.Provenance.SourceCommitDirty {
		appendErr("provenance.source_commit_dirty=true: source commit is dirty")
	}
	if strings.TrimSpace(ev.Provenance.DockerServerVersion) == "" {
		appendErr("provenance.docker_server_version is empty")
	}
	if strings.TrimSpace(ev.Provenance.ProducerVersion) == "" {
		appendErr("provenance.producer_version is empty")
	}
	if ev.Provenance.ExecutableSHA256 == "" {
		appendErr("provenance.executable_sha256 is empty")
	} else if err := ValidateSHA256Hex(ev.Provenance.ExecutableSHA256); err != nil {
		appendErr(fmt.Sprintf("provenance.executable_sha256 invalid: %v", err))
	}

	return result
}

// DerivedClaims holds the implied claim values computed from the
// raw observations. The derivation is pure: it does not mutate ev.
//
//   - ImageExactIDMatch: every one of the four exact image IDs
//     (precreate, create_request, container_inspect_image,
//     container_inspect_config_image) is non-empty and equal.
//   - NetworkExactIDMatch: the three network IDs (create, inspect,
//     endpoint) are non-empty and equal.
//   - CleanupComplete: container.removed && network.removed.
//   - Pass: underlying.Pass && ImageExactIDMatch && NetworkExactIDMatch
//     && CleanupComplete.
type DerivedClaims struct {
	ImageExactIDMatch   bool
	NetworkExactIDMatch bool
	CleanupComplete     bool
	Pass                bool
}

// deriveClaims is a pure function. It reads ev to compute the
// implied claim values and never mutates ev or the underlying
// result. The caller MUST treat the underlying result as
// authoritative for the raw-observation layer.
func deriveClaims(ev *QualifiedExecutionEvidence, underlying VerifyQualifiedExecutionResult) DerivedClaims {
	impliedImage := ev != nil &&
		ev.Image.InspectedBeforeCreate != "" &&
		ev.Image.InspectedBeforeCreate == ev.Image.CreateRequestImage &&
		ev.Image.CreateRequestImage == ev.Image.ContainerInspectImage &&
		ev.Image.CreateRequestImage == ev.Image.ContainerConfigImage

	impliedNet := ev != nil &&
		ev.Network.CreateResponseID != "" &&
		ev.Network.CreateResponseID == ev.Network.InspectResponseID &&
		ev.Network.InspectResponseID == ev.Network.ContainerEndpointID

	impliedCleanup := ev != nil &&
		ev.Container.Removed && ev.Network.Removed

	impliedPass := underlying.Pass && impliedImage && impliedNet && impliedCleanup
	return DerivedClaims{
		ImageExactIDMatch:   impliedImage,
		NetworkExactIDMatch: impliedNet,
		CleanupComplete:     impliedCleanup,
		Pass:                impliedPass,
	}
}

// RequiredTopLevelFields is the canonical allowlist of top-level
// keys for the serialized verifier. Missing any of these fails the
// verifier.
var RequiredTopLevelFields = []string{
	"schema_version",
	"generated_at",
	"image",
	"network",
	"pull",
	"container",
	"provenance",
	"image_exact_id_match",
	"network_exact_id_match",
	"cleanup_complete",
	"pass",
}

// RequiredImageFields is the canonical allowlist of fields that
// must be physically present in the image object.
var RequiredImageFields = []string{
	"requested_reference",
	"inspected_id_before_create",
	"create_request_image",
	"container_inspect_image_id",
	"container_inspect_config_image",
	"repo_digests",
}

// RequiredNetworkFields is the canonical allowlist of fields that
// must be physically present in the network object.
var RequiredNetworkFields = []string{
	"requested_name",
	"create_response_id",
	"inspected_network_id",
	"container_endpoint_network_id",
	"removed",
}

// RequiredPullFields is the canonical allowlist of fields that
// must be physically present in the pull object.
var RequiredPullFields = []string{
	"observation_available",
	"attempted",
	"attempt_count",
}

// RequiredContainerFields is the canonical allowlist of fields
// that must be physically present in the container object.
var RequiredContainerFields = []string{
	"id",
	"created",
	"inspected",
	"started",
	"terminal_state_observed",
	"removed",
}

// RequiredProvenanceFields is the canonical allowlist of fields
// that must be physically present in the provenance object.
var RequiredProvenanceFields = []string{
	"source_commit",
	"source_tree",
	"git_object_format",
	"vcs_modified",
	"working_tree_dirty",
	"source_commit_dirty",
	"docker_server_version",
	"producer_version",
	"executable_sha256",
}

// VerifyQualifiedExecutionBytes parses and verifies serialized
// evidence. The verifier rejects malformed JSON, trailing JSON,
// unknown top-level fields, missing required fields, and any
// semantic failure. Unknown fields at any nested level are
// rejected by the strict decoder + per-object allowlist.
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
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{"trailing JSON document"}}, errors.New("trailing JSON document")
	}
	// Top-level field allowlist.
	for k := range raw {
		allowed := false
		for _, want := range RequiredTopLevelFields {
			if k == want || k == "verifier_errors" {
				allowed = true
				break
			}
		}
		if !allowed {
			return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{
				fmt.Sprintf("unknown top-level field: %q", k),
			}}, nil
		}
	}
	// Top-level presence.
	for _, k := range RequiredTopLevelFields {
		if _, ok := raw[k]; !ok {
			return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{
				fmt.Sprintf("missing required top-level field: %q", k),
			}}, nil
		}
	}
	// Nested object presence + per-object allowlist.
	if err := verifyNestedObject(raw["image"], "image", RequiredImageFields); err != nil {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{err.Error()}}, nil
	}
	if err := verifyNestedObject(raw["network"], "network", RequiredNetworkFields); err != nil {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{err.Error()}}, nil
	}
	if err := verifyNestedObject(raw["pull"], "pull", RequiredPullFields); err != nil {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{err.Error()}}, nil
	}
	if err := verifyNestedObject(raw["container"], "container", RequiredContainerFields); err != nil {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{err.Error()}}, nil
	}
	if err := verifyNestedObject(raw["provenance"], "provenance", RequiredProvenanceFields); err != nil {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{err.Error()}}, nil
	}

	// Decode into the typed struct, then run the in-memory verifier.
	var ev QualifiedExecutionEvidence
	if err := json.Unmarshal(data, &ev); err != nil {
		return VerifyQualifiedExecutionResult{Pass: false, Errors: []string{fmt.Sprintf("decode after structural check: %v", err)}}, err
	}
	return VerifyQualifiedExecution(&ev), nil
}

// verifyNestedObject checks that a nested JSON object is present
// and that every key is on the allowlist.
func verifyNestedObject(raw json.RawMessage, name string, required []string) error {
	if len(raw) == 0 {
		return fmt.Errorf("missing %s object", name)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return fmt.Errorf("decode %s: %w", name, err)
	}
	if len(m) == 0 {
		return fmt.Errorf("missing %s object", name)
	}
	for k := range m {
		ok := false
		for _, want := range required {
			if k == want {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("unknown field in %s: %q", name, k)
		}
	}
	for _, k := range required {
		if _, ok := m[k]; !ok {
			return fmt.Errorf("missing required field in %s: %q", name, k)
		}
	}
	return nil
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

// ValidateSHA1Hex validates a 40-character lowercase hex string.
func ValidateSHA1Hex(s string) error {
	if !sha1Hex40.MatchString(s) {
		return fmt.Errorf("not 40 lowercase hex characters: %q", s)
	}
	return nil
}

// ValidateSHA256Hex validates a 64-character lowercase hex string.
func ValidateSHA256Hex(s string) error {
	if !sha256HexPattern.MatchString(s) {
		return fmt.Errorf("not 64 lowercase hex characters: %q", s)
	}
	return nil
}

// SetDerivedFields computes the derived fields from the underlying
// values. The producer calls this just before persistence or as
// part of a convenience wrapper; the persistence path itself uses
// deriveClaims to avoid touching the supplied claims directly.
func (ev *QualifiedExecutionEvidence) SetDerivedFields() {
	if ev == nil {
		return
	}
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
	ev.CleanupComplete = ev.Container.Removed && ev.Network.Removed
	// Pass is left to the persistence path: the verifier authorizes
	// pass=true after the underlying validator succeeds and the
	// derived claims agree with the supplied claims.
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
	cp.CleanupComplete = false
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
// atomically and re-verifies the persisted bytes. The function
// fails closed on:
//
//   - any in-memory verifier rejection;
//   - any bytes-verifier rejection (after round-trip);
//   - any write or read error;
//   - the persisted artifact not physically containing pass:true
//     plus the three derived claim values.
//
// Persistence sequence (CORRECTION22 P0-4):
//
//  1. validate underlying observations;
//  2. derive expected claims;
//  3. compare supplied claims to derived;
//  4. reject any disagreement;
//  5. return pass only when the supplied artifact is internally truthful.
func PersistQualifiedExecutionEvidence(dir string, ev *QualifiedExecutionEvidence) error {
	if ev == nil {
		return errors.New("evidence is nil")
	}
	if dir == "" {
		return errors.New("dir is empty")
	}

	// Phase 1: independent validator over raw observations.
	underlying := verifyUnderlyingObservations(ev)
	if !underlying.Pass {
		_ = writeRejectedDiagnostic(dir, ev, underlying.Errors)
		return &VerificationError{Errors: underlying.Errors}
	}

	// Phase 2: pure claim derivation.
	derived := deriveClaims(ev, underlying)

	// Phase 3 + 4 + 5: complete verifier authorizes pass=true.
	if !derived.Pass {
		_ = writeRejectedDiagnostic(dir, ev, []string{
			"derived pass=false; supplied claims disagree with raw observations",
		})
		return &VerificationError{Errors: []string{
			"derived pass=false; supplied claims disagree with raw observations",
		}}
	}
	if ev.ImageExactIDMatch != derived.ImageExactIDMatch ||
		ev.NetworkExactIDMatch != derived.NetworkExactIDMatch ||
		ev.CleanupComplete != derived.CleanupComplete {
		_ = writeRejectedDiagnostic(dir, ev, []string{
			"supplied derived claims do not match the recomputed values",
		})
		return &VerificationError{Errors: []string{
			"supplied derived claims do not match the recomputed values",
		}}
	}

	// Stamp the supplied claims with the derived values and pass=true.
	ev.ImageExactIDMatch = derived.ImageExactIDMatch
	ev.NetworkExactIDMatch = derived.NetworkExactIDMatch
	ev.CleanupComplete = derived.CleanupComplete
	ev.Pass = derived.Pass
	ev.VerifierErrors = nil
	if len(ev.Image.InspectedRepoDigests) > 1 {
		sort.Strings(ev.Image.InspectedRepoDigests)
	}

	memoryResult := VerifyQualifiedExecution(ev)
	if !memoryResult.Pass {
		_ = writeRejectedDiagnostic(dir, ev, memoryResult.Errors)
		return &VerificationError{Errors: memoryResult.Errors}
	}

	data, err := json.MarshalIndent(ev, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal evidence: %w", err)
	}
	if err := writeFileAtomic(dir+"/qualified-execution-evidence.json", data); err != nil {
		return err
	}
	persisted, err := os.ReadFile(dir + "/qualified-execution-evidence.json")
	if err != nil {
		return fmt.Errorf("read persisted evidence: %w", err)
	}
	result, err := VerifyQualifiedExecutionBytes(persisted)
	if err != nil {
		return fmt.Errorf("persisted evidence bytes rejected: %w", err)
	}
	if !result.Pass {
		_ = writeRejectedDiagnostic(dir, ev, result.Errors)
		return &VerificationError{Errors: result.Errors}
	}

	// CORRECTION20 P0-8: require the physical document to contain
	// pass:true plus the derived field values. The bytes verifier is
	// the only authority for the artifact claim.
	var persistedDoc QualifiedExecutionEvidence
	if err := json.Unmarshal(persisted, &persistedDoc); err != nil {
		return fmt.Errorf("unmarshal persisted evidence: %w", err)
	}
	if !persistedDoc.Pass ||
		!persistedDoc.ImageExactIDMatch ||
		!persistedDoc.NetworkExactIDMatch ||
		!persistedDoc.CleanupComplete {
		return &VerificationError{Errors: []string{
			fmt.Sprintf("persisted artifact lacks required pass/derived fields: pass=%v image=%v network=%v cleanup=%v",
				persistedDoc.Pass, persistedDoc.ImageExactIDMatch,
				persistedDoc.NetworkExactIDMatch, persistedDoc.CleanupComplete),
		}}
	}
	return nil
}

// writeRejectedDiagnostic writes the failed evidence to a separate
// non-PASS file for diagnostics.
func writeRejectedDiagnostic(dir string, ev *QualifiedExecutionEvidence, errors []string) error {
	if ev == nil {
		return nil
	}
	cp := *ev
	cp.Pass = false
	cp.VerifierErrors = errors
	data, err := json.MarshalIndent(&cp, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(dir+"/qualified-execution-evidence.rejected.json", data)
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

// removeFile is a small helper used by tests to delete a file.
func removeFile(path string) error { return os.Remove(path) }

// readAll is a small helper used by tests to read a file.
func readAll(path string) ([]byte, error) { return os.ReadFile(path) }