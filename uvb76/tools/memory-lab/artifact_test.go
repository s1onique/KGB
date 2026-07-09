package memlab

import (
	"encoding/json"
	"testing"
	"time"
)

func TestClassificationIsValid(t *testing.T) {
	valid := []Classification{
		ClassificationGoHeapRetention,
		ClassificationRSSGrowthStable,
		ClassificationGoroutineGrowth,
		ClassificationNoMaterialGrowth,
		ClassificationInconclusive,
	}

	for _, c := range valid {
		if !c.IsValid() {
			t.Errorf("Expected %q to be valid", c)
		}
	}

	invalid := Classification("invalid_classification")
	if invalid.IsValid() {
		t.Error("Expected invalid classification to return false")
	}
}

func TestClassifyInconclusiveNoRSS(t *testing.T) {
	in := ClassifierInput{
		RSSStartBytes:       0,
		RSSEndBytes:         0,
		HeapInuseStartBytes: 0,
		HeapInuseEndBytes:   0,
		GoroutinesStart:     10,
		GoroutinesEnd:       10,
		HasHeapData:        true,
		HasRSSData:         false,
	}

	result := Classify(in)
	if result != ClassificationInconclusive {
		t.Errorf("Expected inconclusive, got %s", result)
	}
}

func TestClassifyGoroutineGrowth(t *testing.T) {
	in := ClassifierInput{
		RSSStartBytes:       50 * 1024 * 1024, // 50 MB
		RSSEndBytes:         52 * 1024 * 1024, // 52 MB
		HeapInuseStartBytes: 10 * 1024 * 1024,
		HeapInuseEndBytes:   10 * 1024 * 1024,
		GoroutinesStart:     10,
		GoroutinesEnd:       25, // +15 goroutines
		HasHeapData:        true,
		HasRSSData:         true,
	}

	result := Classify(in)
	if result != ClassificationGoroutineGrowth {
		t.Errorf("Expected goroutine_growth, got %s", result)
	}
}

func TestClassifyGoHeapRetention(t *testing.T) {
	in := ClassifierInput{
		RSSStartBytes:       50 * 1024 * 1024,
		RSSEndBytes:         60 * 1024 * 1024, // +10 MB RSS
		HeapInuseStartBytes: 10 * 1024 * 1024,
		HeapInuseEndBytes:   15 * 1024 * 1024, // +5 MB heap
		GoroutinesStart:     10,
		GoroutinesEnd:       12,
		HasHeapData:        true,
		HasRSSData:         true,
	}

	result := Classify(in)
	if result != ClassificationGoHeapRetention {
		t.Errorf("Expected suspected_go_heap_retention, got %s", result)
	}
}

func TestClassifyRSSGrowthHeapStable(t *testing.T) {
	in := ClassifierInput{
		RSSStartBytes:       50 * 1024 * 1024,
		RSSEndBytes:         60 * 1024 * 1024, // +10 MB RSS
		HeapInuseStartBytes: 10 * 1024 * 1024,
		HeapInuseEndBytes:   10 * 1024 * 1024, // stable heap
		GoroutinesStart:     10,
		GoroutinesEnd:       12,
		HasHeapData:        true,
		HasRSSData:         true,
	}

	result := Classify(in)
	if result != ClassificationRSSGrowthStable {
		t.Errorf("Expected rss_growth_heap_stable, got %s", result)
	}
}

func TestClassifyNoMaterialGrowth(t *testing.T) {
	in := ClassifierInput{
		RSSStartBytes:       50 * 1024 * 1024,
		RSSEndBytes:         50*1024*1024 + 512*1024, // +512KB RSS (below 1MB threshold)
		HeapInuseStartBytes: 10 * 1024 * 1024,
		HeapInuseEndBytes:   10 * 1024 * 1024,
		GoroutinesStart:     10,
		GoroutinesEnd:       12,
		HasHeapData:        true,
		HasRSSData:         true,
	}

	result := Classify(in)
	if result != ClassificationNoMaterialGrowth {
		t.Errorf("Expected no_material_growth, got %s", result)
	}
}

func TestClassifyInconclusiveRSSGrowthNoHeapData(t *testing.T) {
	in := ClassifierInput{
		RSSStartBytes:       50 * 1024 * 1024,
		RSSEndBytes:         60 * 1024 * 1024, // +10 MB RSS
		HeapInuseStartBytes: 0,
		HeapInuseEndBytes:   0,
		GoroutinesStart:     10,
		GoroutinesEnd:       12,
		HasHeapData:        false,
		HasRSSData:         true,
	}

	result := Classify(in)
	if result != ClassificationInconclusive {
		t.Errorf("Expected inconclusive, got %s", result)
	}
}

func TestBuildVerdict(t *testing.T) {
	in := ClassifierInput{
		RSSStartBytes:       50 * 1024 * 1024,
		RSSEndBytes:         60 * 1024 * 1024,
		HeapInuseStartBytes: 10 * 1024 * 1024,
		HeapInuseEndBytes:   15 * 1024 * 1024,
		GoroutinesStart:     10,
		GoroutinesEnd:       12,
		HasHeapData:        true,
		HasRSSData:         true,
	}

	verdict := BuildVerdict(ClassificationGoHeapRetention, in)

	if verdict.Summary == "" {
		t.Error("Expected non-empty summary")
	}
	if verdict.RSSGrowthBytes != 10*1024*1024 {
		t.Errorf("Expected RSSGrowthBytes=10MB, got %d", verdict.RSSGrowthBytes)
	}
	if verdict.HeapGrowthBytes != 5*1024*1024 {
		t.Errorf("Expected HeapGrowthBytes=5MB, got %d", verdict.HeapGrowthBytes)
	}
	if len(verdict.Reasons) == 0 {
		t.Error("Expected non-empty reasons")
	}
}

func TestManifestJSONSerialization(t *testing.T) {
	now := time.Now().UTC()
	m := Manifest{
		LabID:         "test-lab-001",
		StartedAt:     now,
		EndedAt:       now.Add(60 * time.Second),
		TargetID:      "tovarisch-01",
		DurationSeconds: 60.0,
		Classification: ClassificationNoMaterialGrowth,
		Verdict: Verdict{
			Summary:         "no material memory growth detected",
			RSSGrowthBytes:  1024 * 1024,
			HeapGrowthBytes: 0,
			GoroutineDelta:  2,
			Reasons:         []string{"rss_delta_below_threshold"},
		},
		Samples: Samples{
			StartRSS: RSSSample{
				Timestamp: now,
				Bytes:    50 * 1024 * 1024,
			},
			EndRSS: RSSSample{
				Timestamp: now.Add(60 * time.Second),
				Bytes:    51 * 1024 * 1024,
			},
			GoroutineSamples: []GoroutineSample{
				{Timestamp: now, Count: 10},
				{Timestamp: now.Add(60 * time.Second), Count: 12},
			},
		},
	}

	// Serialize
	data, err := m.ToJSON()
	if err != nil {
		t.Fatalf("ToJSON failed: %v", err)
	}

	// Deserialize
	parsed, err := FromJSON(data)
	if err != nil {
		t.Fatalf("FromJSON failed: %v", err)
	}

	if parsed.LabID != m.LabID {
		t.Errorf("LabID mismatch: got %q, want %q", parsed.LabID, m.LabID)
	}
	if parsed.Classification != m.Classification {
		t.Errorf("Classification mismatch: got %s, want %s", parsed.Classification, m.Classification)
	}
	if parsed.Samples.StartRSS.Bytes != m.Samples.StartRSS.Bytes {
		t.Errorf("StartRSS.Bytes mismatch: got %d, want %d", parsed.Samples.StartRSS.Bytes, m.Samples.StartRSS.Bytes)
	}
}

func TestManifestJSONStableFieldNames(t *testing.T) {
	// Verify field names are stable (not changed by struct tags)
	type manifestRaw struct {
		LabID           string `json:"lab_id"`
		StartedAt       string `json:"started_at"`
		Classification string `json:"classification"`
	}

	now := time.Now().UTC()
	m := Manifest{
		LabID:         "test",
		StartedAt:     now,
		Classification: ClassificationInconclusive,
	}

	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("Marshal failed: %v", err)
	}

	// Check that JSON contains the expected field names (not field names from struct)
	var rawMap map[string]json.RawMessage
	if err := json.Unmarshal(data, &rawMap); err != nil {
		t.Fatalf("Unmarshal to map failed: %v", err)
	}

	if _, ok := rawMap["lab_id"]; !ok {
		t.Error("Expected 'lab_id' key in JSON output")
	}
	if _, ok := rawMap["classification"]; !ok {
		t.Error("Expected 'classification' key in JSON output")
	}
	if _, ok := rawMap["started_at"]; !ok {
		t.Error("Expected 'started_at' key in JSON output")
	}
}

func TestThresholdConstants(t *testing.T) {
	// Verify thresholds are sensible
	if MinRSSGrowthBytes < 1024*1024 {
		t.Error("MinRSSGrowthBytes should be at least 1 MB")
	}
	if MinHeapGrowthBytes < 256*1024 {
		t.Error("MinHeapGrowthBytes should be at least 256 KB")
	}
	if MinGoroutineGrowth < 5 {
		t.Error("MinGoroutineGrowth should be at least 5")
	}
}
