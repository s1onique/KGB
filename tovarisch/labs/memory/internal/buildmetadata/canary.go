// Package buildmetadata owns canonical canary image build metadata.
package buildmetadata

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const SchemaVersion = "canary-image-build/v2"

var imageIDPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
var hex64Pattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
var objectIDPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)

// CanaryImageBuild separates Docker Engine and BuildKit identity classes.
type CanaryImageBuild struct {
	SchemaVersion          string   `json:"schema_version"`
	SourceCommit           string   `json:"source_commit"`
	SourceTree             string   `json:"source_tree"`
	CanarySourceTree       string   `json:"canary_source_tree"`
	RequestedReference     string   `json:"requested_reference"`
	EngineImageID          string   `json:"engine_image_id"`
	RepoDigests            []string `json:"repo_digests"`
	BuildKitManifestDigest string   `json:"buildkit_manifest_digest"`
	BuildKitIndexDigest    string   `json:"buildkit_index_digest"`
	CanaryBinarySHA256     string   `json:"canary_binary_sha256"`
	CanaryVCSRevision      string   `json:"canary_vcs_revision"`
	CanaryVCSModified      bool     `json:"canary_vcs_modified"`
}

func (m CanaryImageBuild) Validate() error {
	if m.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version=%q", m.SchemaVersion)
	}
	if !objectIDPattern.MatchString(m.SourceCommit) || !objectIDPattern.MatchString(m.SourceTree) || !objectIDPattern.MatchString(m.CanarySourceTree) {
		return errors.New("source identity is malformed")
	}
	if m.RequestedReference == "" {
		return errors.New("requested_reference is empty")
	}
	if !imageIDPattern.MatchString(m.EngineImageID) {
		return errors.New("engine_image_id is not canonical")
	}
	if !hex64Pattern.MatchString(m.CanaryBinarySHA256) {
		return errors.New("canary_binary_sha256 is not canonical")
	}
	if m.CanaryVCSRevision != m.SourceCommit {
		return errors.New("canary_vcs_revision does not equal source_commit")
	}
	if m.CanaryVCSModified {
		return errors.New("canary_vcs_modified=true")
	}
	for name, digest := range map[string]string{"buildkit_manifest_digest": m.BuildKitManifestDigest, "buildkit_index_digest": m.BuildKitIndexDigest} {
		if digest != "" && !imageIDPattern.MatchString(digest) {
			return fmt.Errorf("%s is not canonical", name)
		}
	}
	return nil
}

// BinaryAuthority hashes a built canary and reads its embedded VCS authority.
func BinaryAuthority(path string) (hash, revision string, modified bool, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", false, err
	}
	sum := sha256.Sum256(data)
	hash = hex.EncodeToString(sum[:])
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return "", "", false, fmt.Errorf("read Go build info: %w", err)
	}
	var modifiedRaw string
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			revision = setting.Value
		case "vcs.modified":
			modifiedRaw = setting.Value
		}
	}
	if revision == "" {
		return "", "", false, errors.New("canary binary has no embedded vcs.revision")
	}
	return hash, revision, modifiedRaw == "true", nil
}

// BuildKitDigests extracts available digest classes from buildx metadata.
func BuildKitDigests(path string) (manifest, index string, err error) {
	if path == "" {
		return "", "", nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", "", err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return "", "", err
	}
	_ = json.Unmarshal(raw["containerimage.digest"], &manifest)
	var descriptor struct {
		Digest    string `json:"digest"`
		MediaType string `json:"mediaType"`
	}
	if json.Unmarshal(raw["containerimage.descriptor"], &descriptor) == nil && strings.Contains(descriptor.MediaType, "index") {
		index = descriptor.Digest
	}
	return manifest, index, nil
}

// WriteAtomic replaces stale metadata and verifies the resulting current bytes.
func WriteAtomic(path string, metadata CanaryImageBuild) error {
	if err := metadata.Validate(); err != nil {
		return err
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".canary-image-build-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return err
	}
	persisted, err := Read(path)
	if err != nil {
		return err
	}
	if persisted.SourceCommit != metadata.SourceCommit || persisted.EngineImageID != metadata.EngineImageID {
		return errors.New("persisted metadata retained stale source/image identity")
	}
	return nil
}

func Read(path string) (CanaryImageBuild, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return CanaryImageBuild{}, err
	}
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.DisallowUnknownFields()
	var metadata CanaryImageBuild
	if err := dec.Decode(&metadata); err != nil {
		return CanaryImageBuild{}, err
	}
	var extra any
	if err := dec.Decode(&extra); err == nil {
		return CanaryImageBuild{}, errors.New("second JSON document")
	}
	return metadata, metadata.Validate()
}
