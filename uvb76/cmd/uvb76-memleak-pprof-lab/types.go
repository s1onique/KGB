package main

// LabResult captures the lab outcome.
type LabResult struct {
	OK                 bool   `json:"ok"`
	DurationSeconds    int    `json:"duration_seconds"`
	ArtifactDir        string `json:"artifact_dir"`
	UVB76Started       bool   `json:"uvb76_started"`
	PProfReachable     bool   `json:"pprof_reachable"`
	TovarischReachable bool   `json:"tovarisch_reachable"`
	CollectorSucceeded bool   `json:"collector_succeeded"`
	PProfDiffSucceeded bool   `json:"pprof_diff_succeeded"`
	ManifestValid      bool   `json:"manifest_valid"`
	VerdictValid       bool   `json:"verdict_valid"`
}
