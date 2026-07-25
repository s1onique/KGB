// final_qualified_evidence.go — One post-lifecycle evidence producer.
package evidence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/dockerlab"
)

var ErrFinalQualifiedOutcomeIncomplete = errors.New("final qualified lifecycle outcome is incomplete")
var ErrFinalQualifiedProvenanceIncomplete = errors.New("final qualified controller provenance is incomplete")

// BuildAndPersistFinalQualifiedEvidence is the only accepted producer for
// helper and production CLI qualified-execution evidence.
func BuildAndPersistFinalQualifiedEvidence(
	ctx context.Context,
	outcome *dockerlab.QualifiedLifecycleOutcome,
	provenance ControllerProvenance,
	artifactDir string,
) (*QualifiedExecutionEvidence, error) {
	if ctx == nil {
		return nil, errors.New("context is nil")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if outcome == nil || outcome.Observations == nil {
		return nil, fmt.Errorf("%w: outcome or observations is nil", ErrFinalQualifiedOutcomeIncomplete)
	}
	if !outcome.Terminal || !outcome.Observations.Container.TerminalStateObserved {
		return nil, fmt.Errorf("%w: terminal state is unproven", ErrFinalQualifiedOutcomeIncomplete)
	}
	if !outcome.ContainerRemoved || !outcome.Observations.Container.Removed {
		return nil, fmt.Errorf("%w: container removal/absence is unproven", ErrFinalQualifiedOutcomeIncomplete)
	}
	if !outcome.NetworkRemoved || !outcome.Observations.Network.Removed {
		return nil, fmt.Errorf("%w: network removal/absence is unproven", ErrFinalQualifiedOutcomeIncomplete)
	}
	if err := validateFinalControllerProvenance(provenance); err != nil {
		return nil, err
	}

	final := dockerlab.CloneQualifiedExecutionObservations(outcome.Observations)
	final.SetProvenance(provenance.VCSRevision, provenance.VCSTree, provenance.GitObjectFormat,
		provenance.DockerServerVersion, provenance.ProducerVersion, provenance.ExecutableSHA256)
	final.SetProvenanceDirty(provenance.WorkingTreeDirty, provenance.SourceCommitDirty)
	final.SetVCSModified(provenance.VCSModified)
	outcome.RecordPhase(dockerlab.PhaseProvenanceStamped)

	ev := BuildEvidenceFromObservations(final)
	if ev == nil {
		return nil, errors.New("build evidence returned nil")
	}
	ev.SetDerivedFields()
	outcome.RecordPhase(dockerlab.PhaseEvidenceBuilt)
	if err := PersistQualifiedExecutionEvidence(artifactDir, ev); err != nil {
		return nil, err
	}
	outcome.RecordPhase(dockerlab.PhaseEvidencePersisted)

	data, err := os.ReadFile(artifactDir + "/qualified-execution-evidence.json")
	if err != nil {
		return nil, fmt.Errorf("read persisted qualified evidence: %w", err)
	}
	verified, err := VerifyQualifiedExecutionBytes(data)
	if err != nil || !verified.Pass {
		return nil, fmt.Errorf("verify persisted qualified evidence: result=%v err=%w", verified.Errors, err)
	}
	var persisted QualifiedExecutionEvidence
	if err := json.Unmarshal(data, &persisted); err != nil {
		return nil, fmt.Errorf("decode persisted qualified evidence: %w", err)
	}
	return &persisted, nil
}

func validateFinalControllerProvenance(p ControllerProvenance) error {
	invalid := func(field string) error {
		return fmt.Errorf("%w: %s", ErrFinalQualifiedProvenanceIncomplete, field)
	}
	if strings.TrimSpace(p.VCSRevision) == "" || strings.TrimSpace(p.VCSTree) == "" {
		return invalid("source commit/tree is empty")
	}
	if p.GitObjectFormat != gitObjectFormatSHA1 && p.GitObjectFormat != gitObjectFormatSHA256 {
		return invalid("git object format is not sha1 or sha256")
	}
	if p.VCSModified || p.WorkingTreeDirty || p.SourceCommitDirty {
		return invalid("controller source is dirty")
	}
	if strings.TrimSpace(p.ProducerVersion) == "" {
		return invalid("producer version is empty")
	}
	if err := ValidateSHA256Hex(p.ExecutableSHA256); err != nil {
		return invalid("executable SHA-256 is invalid")
	}
	if strings.TrimSpace(p.DockerServerVersion) == "" {
		return invalid("Docker server version is empty")
	}
	return nil
}
