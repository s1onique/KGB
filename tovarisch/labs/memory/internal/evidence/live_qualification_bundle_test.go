package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/buildmetadata"
)

type liveFixture struct {
	root, path string
	bundle     LiveQualificationBundle
}

func newLiveFixture(t *testing.T) *liveFixture {
	t.Helper()
	root := t.TempDir()
	source := LiveBundleSource{Commit: strings.Repeat("a", 40), Tree: strings.Repeat("b", 40)}
	binary := func(name string) (string, string) {
		p := filepath.Join(root, name)
		os.WriteFile(p, []byte(name+"-binary"), 0o755)
		h, _ := hashLiveFile(p)
		return p, h
	}
	hb, hh := binary("helper")
	pb, ph := binary("production")
	writeInfo := func(name string) string {
		p := filepath.Join(root, name)
		os.WriteFile(p, []byte("\tbuild\tvcs.revision="+source.Commit+"\n\tbuild\tvcs.modified=false\n"), 0o644)
		return p
	}
	hi := writeInfo("helper-buildinfo.txt")
	pi := writeInfo("production-buildinfo.txt")
	base := buildValidEvidence()
	engine := base.Image.InspectedBeforeCreate
	writeEv := func(dir, hash, role string) string {
		d := filepath.Join(root, dir)
		os.Mkdir(d, 0o755)
		ev := *base
		ev.Provenance.SourceCommit = source.Commit
		ev.Provenance.SourceTree = source.Tree
		ev.Provenance.ExecutableSHA256 = hash
		ev.Provenance.ProducerVersion = role + "/1.0.0"
		ev.SetDerivedFields()
		if err := PersistQualifiedExecutionEvidence(d, &ev); err != nil {
			t.Fatal(err)
		}
		return filepath.Join(d, "qualified-execution-evidence.json")
	}
	he := writeEv("helper-evidence", hh, "helper")
	pe := writeEv("production-evidence", ph, "production")
	meta := buildmetadata.CanaryImageBuild{SchemaVersion: buildmetadata.SchemaVersion, SourceCommit: source.Commit, SourceTree: source.Tree, CanarySourceTree: strings.Repeat("c", 40), RequestedReference: "kgb-tovarisch-canary:correction46-S46", EngineImageID: engine, RepoDigests: []string{}, BuildKitManifestDigest: "sha256:" + strings.Repeat("d", 64), BuildKitIndexDigest: "sha256:" + strings.Repeat("e", 64), CanaryBinarySHA256: strings.Repeat("f", 64), CanaryVCSRevision: source.Commit}
	mp := filepath.Join(root, "canary-image-build.json")
	if err := buildmetadata.WriteAtomic(mp, meta); err != nil {
		t.Fatal(err)
	}
	producer := func(role, b, bi, ev, h string) LiveBundleProducer {
		eh, _ := hashLiveFile(ev)
		return LiveBundleProducer{Role: role, BinaryPath: b, BuildInfoPath: bi, BinarySHA256: h, VCSRevision: source.Commit, EvidencePath: ev, EvidenceSHA256: eh, Pass: true}
	}
	bundle := LiveQualificationBundle{SchemaVersion: LiveQualificationBundleSchema, VerifierVersion: LiveQualificationVerifierVersion, Source: source, Image: LiveBundleImage{MetadataPath: mp, EngineImageID: engine, BuildKitManifestDigest: meta.BuildKitManifestDigest, BuildKitIndexDigest: meta.BuildKitIndexDigest, CanaryBinarySHA256: meta.CanaryBinarySHA256, CanaryVCSRevision: source.Commit}, Helper: producer("helper", hb, hi, he, hh), Production: producer("production", pb, pi, pe, ph)}
	path := filepath.Join(root, "live-qualification-bundle.json")
	if err := WriteLiveQualificationBundle(path, bundle); err != nil {
		t.Fatal(err)
	}
	return &liveFixture{root: root, path: path, bundle: bundle}
}
func (f *liveFixture) write(t *testing.T) {
	t.Helper()
	if err := WriteLiveQualificationBundle(f.path, f.bundle); err != nil {
		t.Fatal(err)
	}
}
func (f *liveFixture) mutateEvidence(t *testing.T, role string, fn func(*QualifiedExecutionEvidence)) {
	t.Helper()
	p := &f.bundle.Helper
	if role == "production" {
		p = &f.bundle.Production
	}
	data, err := os.ReadFile(p.EvidencePath)
	if err != nil {
		t.Fatal(err)
	}
	var ev QualifiedExecutionEvidence
	if err := json.Unmarshal(data, &ev); err != nil {
		t.Fatal(err)
	}
	fn(&ev)
	data, err = json.MarshalIndent(ev, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(p.EvidencePath, data, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	p.EvidenceSHA256 = hex.EncodeToString(sum[:])
	f.write(t)
}

func TestLiveQualification_HelperAndCLIShareCanonicalProducer(t *testing.T) {
	f := newLiveFixture(t)
	if err := VerifyLiveQualificationBundle(f.path); err != nil {
		t.Fatal(err)
	}
}

func TestLiveQualificationMutationMatrix(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*testing.T, *liveFixture)
	}{
		{"wrong_source_commit_tree", func(t *testing.T, f *liveFixture) { f.bundle.Source.Commit = strings.Repeat("1", 40); f.write(t) }},
		{"stale_build_metadata", func(t *testing.T, f *liveFixture) {
			m, _ := buildmetadata.Read(f.bundle.Image.MetadataPath)
			m.SourceCommit = strings.Repeat("1", 40)
			m.CanaryVCSRevision = m.SourceCommit
			if err := buildmetadata.WriteAtomic(f.bundle.Image.MetadataPath, m); err != nil {
				t.Fatal(err)
			}
		}},
		{"engine_id_replaced_by_buildkit_index", func(t *testing.T, f *liveFixture) {
			f.bundle.Image.EngineImageID = f.bundle.Image.BuildKitIndexDigest
			f.write(t)
		}},
		{"tag_substituted_for_engine_id", func(t *testing.T, f *liveFixture) {
			f.bundle.Image.EngineImageID = "kgb-tovarisch-canary:latest"
			f.write(t)
		}},
		{"canary_binary_sha_mismatch", func(t *testing.T, f *liveFixture) {
			f.bundle.Image.CanaryBinarySHA256 = strings.Repeat("1", 64)
			f.write(t)
		}},
		{"canary_vcs_revision_mismatch", func(t *testing.T, f *liveFixture) {
			f.bundle.Image.CanaryVCSRevision = strings.Repeat("1", 40)
			f.write(t)
		}},
		{"helper_vcs_mismatch", func(t *testing.T, f *liveFixture) { f.bundle.Helper.VCSRevision = strings.Repeat("1", 40); f.write(t) }},
		{"cli_vcs_mismatch", func(t *testing.T, f *liveFixture) {
			f.bundle.Production.VCSRevision = strings.Repeat("1", 40)
			f.write(t)
		}},
		{"helper_evidence_digest_mismatch", func(t *testing.T, f *liveFixture) {
			f.bundle.Helper.EvidenceSHA256 = strings.Repeat("1", 64)
			f.write(t)
		}},
		{"production_evidence_digest_mismatch", func(t *testing.T, f *liveFixture) {
			f.bundle.Production.EvidenceSHA256 = strings.Repeat("1", 64)
			f.write(t)
		}},
		{"missing_terminal_state", func(t *testing.T, f *liveFixture) {
			f.mutateEvidence(t, "helper", func(ev *QualifiedExecutionEvidence) { ev.Container.TerminalStateObserved = false })
		}},
		{"missing_provenance", func(t *testing.T, f *liveFixture) {
			f.mutateEvidence(t, "helper", func(ev *QualifiedExecutionEvidence) { ev.Provenance.SourceCommit = "" })
		}},
		{"missing_reachability", func(t *testing.T, f *liveFixture) {
			f.mutateEvidence(t, "helper", func(ev *QualifiedExecutionEvidence) { ev.Reachability.Method = "" })
		}},
		{"operation_order_mutation", func(t *testing.T, f *liveFixture) {
			f.mutateEvidence(t, "helper", func(ev *QualifiedExecutionEvidence) { ev.Reachability.Health.Operation = "state" })
		}},
		{"pull_attempt", func(t *testing.T, f *liveFixture) {
			f.mutateEvidence(t, "helper", func(ev *QualifiedExecutionEvidence) { ev.Pull.Attempted = true; ev.Pull.AttemptCount = 1 })
		}},
		{"cleanup_lie", func(t *testing.T, f *liveFixture) {
			f.mutateEvidence(t, "helper", func(ev *QualifiedExecutionEvidence) { ev.Container.Removed = false })
		}},
		{"pass_false_negative", func(t *testing.T, f *liveFixture) { f.bundle.Helper.Pass = false; f.write(t) }},
		{"pass_false_positive", func(t *testing.T, f *liveFixture) {
			f.mutateEvidence(t, "helper", func(ev *QualifiedExecutionEvidence) { ev.Container.TerminalStateObserved = false; ev.Pass = true })
		}},
		{"missing_required_member", func(t *testing.T, f *liveFixture) {
			data, _ := os.ReadFile(f.path)
			data = []byte(strings.Replace(string(data), "\"verifier_version\": \""+LiveQualificationVerifierVersion+"\",\n", "", 1))
			os.WriteFile(f.path, data, 0o644)
		}},
		{"null_required_member", func(t *testing.T, f *liveFixture) {
			data, _ := os.ReadFile(f.path)
			data = []byte(strings.Replace(string(data), "\"source\": {", "\"source\": null", 1))
			os.WriteFile(f.path, data, 0o644)
		}},
		{"wrong_type_required_member", func(t *testing.T, f *liveFixture) {
			data, _ := os.ReadFile(f.path)
			data = []byte(strings.Replace(string(data), "\"pass\": true", "\"pass\": \"true\"", 1))
			os.WriteFile(f.path, data, 0o644)
		}},
		{"unknown_member", func(t *testing.T, f *liveFixture) {
			data, _ := os.ReadFile(f.path)
			data = []byte(strings.Replace(string(data), "{\n", "{\n  \"unknown\": true,\n", 1))
			os.WriteFile(f.path, data, 0o644)
		}},
		{"second_json_document", func(t *testing.T, f *liveFixture) {
			file, _ := os.OpenFile(f.path, os.O_APPEND|os.O_WRONLY, 0)
			file.WriteString("{}\n")
			file.Close()
		}},
		{"trailing_malformed_bytes", func(t *testing.T, f *liveFixture) {
			file, _ := os.OpenFile(f.path, os.O_APPEND|os.O_WRONLY, 0)
			file.WriteString("not-json")
			file.Close()
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newLiveFixture(t)
			tc.mutate(t, f)
			if err := VerifyLiveQualificationBundle(f.path); err == nil {
				t.Fatalf("mutation %s passed", tc.name)
			}
		})
	}
}
