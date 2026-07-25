package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/buildmetadata"
)

const LiveQualificationBundleSchema = "live-qualification-bundle/v1"
const LiveQualificationVerifierVersion = "correction46/1.0.0"

type LiveBundleSource struct {
	Commit string `json:"commit"`
	Tree   string `json:"tree"`
}
type LiveBundleImage struct {
	MetadataPath           string `json:"metadata_path"`
	EngineImageID          string `json:"engine_image_id"`
	BuildKitManifestDigest string `json:"buildkit_manifest_digest"`
	BuildKitIndexDigest    string `json:"buildkit_index_digest"`
	CanaryBinarySHA256     string `json:"canary_binary_sha256"`
	CanaryVCSRevision      string `json:"canary_vcs_revision"`
}
type LiveBundleProducer struct {
	Role           string `json:"role"`
	BinaryPath     string `json:"binary_path"`
	BuildInfoPath  string `json:"build_info_path"`
	BinarySHA256   string `json:"binary_sha256"`
	VCSRevision    string `json:"vcs_revision"`
	EvidencePath   string `json:"evidence_path"`
	EvidenceSHA256 string `json:"evidence_sha256"`
	Pass           bool   `json:"pass"`
}
type LiveQualificationBundle struct {
	SchemaVersion   string             `json:"schema_version"`
	VerifierVersion string             `json:"verifier_version"`
	Source          LiveBundleSource   `json:"source"`
	Image           LiveBundleImage    `json:"image"`
	Helper          LiveBundleProducer `json:"helper"`
	Production      LiveBundleProducer `json:"production"`
}

func VerifyLiveQualificationBundle(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	var bundle LiveQualificationBundle
	if err := dec.Decode(&bundle); err != nil {
		return fmt.Errorf("decode live bundle: %w", err)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("second JSON document")
		}
		return fmt.Errorf("trailing malformed bytes: %w", err)
	}
	return verifyLiveQualificationBundle(filepath.Dir(path), &bundle)
}

func verifyLiveQualificationBundle(base string, bundle *LiveQualificationBundle) error {
	if bundle.SchemaVersion != LiveQualificationBundleSchema || bundle.VerifierVersion != LiveQualificationVerifierVersion {
		return errors.New("unsupported live bundle schema/verifier")
	}
	if err := validateObjectIDPair(bundle.Source.Commit, bundle.Source.Tree); err != nil {
		return err
	}
	metadataPath := resolveBundlePath(base, bundle.Image.MetadataPath)
	metadata, err := buildmetadata.Read(metadataPath)
	if err != nil {
		return fmt.Errorf("build metadata: %w", err)
	}
	if metadata.SourceCommit != bundle.Source.Commit || metadata.SourceTree != bundle.Source.Tree {
		return errors.New("stale build metadata or wrong source commit/tree")
	}
	if bundle.Image.EngineImageID != metadata.EngineImageID {
		return errors.New("Engine image ID mismatch")
	}
	if strings.Contains(bundle.Image.EngineImageID, ":") && !strings.HasPrefix(bundle.Image.EngineImageID, "sha256:") {
		return errors.New("tag substituted for Engine image ID")
	}
	if bundle.Image.BuildKitManifestDigest != metadata.BuildKitManifestDigest || bundle.Image.BuildKitIndexDigest != metadata.BuildKitIndexDigest {
		return errors.New("BuildKit digest class mismatch")
	}
	if bundle.Image.CanaryBinarySHA256 != metadata.CanaryBinarySHA256 {
		return errors.New("canary binary SHA mismatch")
	}
	if bundle.Image.CanaryVCSRevision != metadata.CanaryVCSRevision || bundle.Image.CanaryVCSRevision != bundle.Source.Commit {
		return errors.New("canary VCS revision mismatch")
	}
	if err := verifyLiveProducer(base, bundle.Source, bundle.Image, bundle.Helper, "helper"); err != nil {
		return err
	}
	if err := verifyLiveProducer(base, bundle.Source, bundle.Image, bundle.Production, "production"); err != nil {
		return err
	}
	return compareLiveProducerAuthorities(base, bundle.Helper, bundle.Production)
}

func verifyLiveProducer(base string, source LiveBundleSource, image LiveBundleImage, producer LiveBundleProducer, role string) error {
	if producer.Role != role {
		return fmt.Errorf("%s producer role mismatch", role)
	}
	binaryHash, err := hashLiveFile(resolveBundlePath(base, producer.BinaryPath))
	if err != nil {
		return fmt.Errorf("%s binary: %w", role, err)
	}
	if binaryHash != producer.BinarySHA256 {
		return fmt.Errorf("%s binary SHA mismatch", role)
	}
	revision, modified, err := parseCapturedBuildInfo(resolveBundlePath(base, producer.BuildInfoPath))
	if err != nil {
		return fmt.Errorf("%s build info: %w", role, err)
	}
	if modified || revision != source.Commit || producer.VCSRevision != revision {
		return fmt.Errorf("%s VCS mismatch", role)
	}
	evidencePath := resolveBundlePath(base, producer.EvidencePath)
	evidenceBytes, err := os.ReadFile(evidencePath)
	if err != nil {
		return err
	}
	evidenceHash := sha256.Sum256(evidenceBytes)
	if hex.EncodeToString(evidenceHash[:]) != producer.EvidenceSHA256 {
		return fmt.Errorf("%s evidence digest mismatch", role)
	}
	verified, verifyErr := VerifyQualifiedExecutionBytes(evidenceBytes)
	if verifyErr != nil || !verified.Pass {
		return fmt.Errorf("%s evidence rejected: %v: %w", role, verified.Errors, verifyErr)
	}
	var ev QualifiedExecutionEvidence
	if err := json.Unmarshal(evidenceBytes, &ev); err != nil {
		return err
	}
	if ev.Provenance.SourceCommit != source.Commit || ev.Provenance.SourceTree != source.Tree {
		return fmt.Errorf("%s evidence source mismatch", role)
	}
	if ev.Provenance.ExecutableSHA256 != producer.BinarySHA256 {
		return fmt.Errorf("%s evidence executable mismatch", role)
	}
	if ev.Image.InspectedBeforeCreate != image.EngineImageID {
		return fmt.Errorf("%s evidence Engine image mismatch", role)
	}
	if producer.Pass != verified.Pass {
		return fmt.Errorf("%s pass claim mismatch", role)
	}
	return nil
}

func compareLiveProducerAuthorities(base string, helper, production LiveBundleProducer) error {
	load := func(p LiveBundleProducer) (QualifiedExecutionEvidence, error) {
		data, err := os.ReadFile(resolveBundlePath(base, p.EvidencePath))
		if err != nil {
			return QualifiedExecutionEvidence{}, err
		}
		var ev QualifiedExecutionEvidence
		err = json.Unmarshal(data, &ev)
		return ev, err
	}
	h, err := load(helper)
	if err != nil {
		return err
	}
	p, err := load(production)
	if err != nil {
		return err
	}
	if h.SchemaVersion != p.SchemaVersion || h.Provenance.SourceCommit != p.Provenance.SourceCommit || h.Provenance.SourceTree != p.Provenance.SourceTree || h.Provenance.GitObjectFormat != p.Provenance.GitObjectFormat || h.Image.InspectedBeforeCreate != p.Image.InspectedBeforeCreate {
		return errors.New("helper and production authority mismatch")
	}
	if h.Reachability.Method != p.Reachability.Method || h.Reachability.Health.Operation != p.Reachability.Health.Operation || h.Reachability.InitialState.Operation != p.Reachability.InitialState.Operation || h.Reachability.Operate.Operation != p.Reachability.Operate.Operation || h.Reachability.FinalState.Operation != p.Reachability.FinalState.Operation {
		return errors.New("helper and production reachability schema/order mismatch")
	}
	if h.Pull.Attempted != p.Pull.Attempted || h.Pull.AttemptCount != p.Pull.AttemptCount || h.Container.Removed != p.Container.Removed || h.Network.Removed != p.Network.Removed {
		return errors.New("helper and production pull/cleanup policy mismatch")
	}
	return nil
}

func parseCapturedBuildInfo(path string) (string, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false, err
	}
	var revision, modified string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "build\tvcs.revision=") {
			revision = strings.TrimPrefix(line, "build\t")
		} else if strings.HasPrefix(line, "vcs.revision=") {
			revision = line
		}
		if strings.HasPrefix(revision, "vcs.revision=") {
			revision = strings.TrimPrefix(revision, "vcs.revision=")
		}
		if strings.Contains(line, "vcs.modified=") {
			modified = line[strings.Index(line, "vcs.modified=")+len("vcs.modified="):]
		}
	}
	if revision == "" || modified == "" {
		return "", false, errors.New("vcs.revision/vcs.modified missing")
	}
	return revision, modified == "true", nil
}
func hashLiveFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}
func resolveBundlePath(base, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(base, path)
}
func validateObjectIDPair(commit, tree string) error {
	if (len(commit) != 40 && len(commit) != 64) || len(tree) != len(commit) {
		return errors.New("wrong source commit/tree")
	}
	for _, v := range []string{commit, tree} {
		for _, c := range v {
			if !(c >= '0' && c <= '9' || c >= 'a' && c <= 'f') {
				return errors.New("wrong source commit/tree")
			}
		}
	}
	return nil
}

func WriteLiveQualificationBundle(path string, bundle LiveQualificationBundle) error {
	data, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return os.WriteFile(path, data, 0o644)
}
