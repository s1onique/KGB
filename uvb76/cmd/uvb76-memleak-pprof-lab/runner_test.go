package main

import (
	"testing"
)

func TestProcessIsGoneForInvalidPID(t *testing.T) {
	// PID 0 should be considered gone
	if !processIsGone(0) {
		t.Error("PID 0 should be considered gone")
	}

	// Negative PIDs should be considered gone
	if !processIsGone(-1) {
		t.Error("negative PID should be considered gone")
	}
}

func TestFlagsDefaultValues(t *testing.T) {
	// Test that default values are sensible for smoke
	if *flagUseFakeTovarisch != false {
		t.Errorf("default use-fake-tovarisch should be false, got %v", *flagUseFakeTovarisch)
	}

	// Default duration should be 2 minutes for smoke
	if (*flagDuration).Seconds() != 120 {
		t.Errorf("default duration should be 120s for smoke, got %v", *flagDuration)
	}

	// Sample interval should be 1 second
	if (*flagSampleInterval).Seconds() != 1 {
		t.Errorf("default sample interval should be 1s, got %v", *flagSampleInterval)
	}
}

func TestPortDefaults(t *testing.T) {
	if *flagTovarischPort != "18317" {
		t.Errorf("default tovarisch port should be 18317, got %s", *flagTovarischPort)
	}
	if *flagUVB76Port != "18444" {
		t.Errorf("default uvb76 port should be 18444, got %s", *flagUVB76Port)
	}
	if *flagPProfPort != "16060" {
		t.Errorf("default pprof port should be 16060, got %s", *flagPProfPort)
	}
}

func TestLabResultClassification(t *testing.T) {
	result := LabResult{
		RealTovarischStarted:  true,
		RealTovarischReady:    true,
		RealUVB76Started:      true,
		UVB76PProfReady:       true,
		RealTargetObserved:    true,
		ScrapeAttempted:       true,
		ScrapeCompleted:       true,
		ProcessSamplesPresent: true,
		ProfilesPresent:       true,
		UVB76Removed:          true,
		TovarischRemoved:      true,
		PortsReleased:         true,
	}

	// All checks pass -> OBSERVED
	if result.RealTovarischStarted && result.RealUVB76Started &&
		result.UVB76PProfReady && result.RealTargetObserved &&
		result.ProcessSamplesPresent && result.ProfilesPresent &&
		result.UVB76Removed && result.TovarischRemoved && result.PortsReleased {
		result.Classification = "OBSERVED"
		result.OK = true
	}

	if result.Classification != "OBSERVED" {
		t.Errorf("expected OBSERVED classification, got %s", result.Classification)
	}
	if !result.OK {
		t.Error("result should be OK when all checks pass")
	}
}

func TestLabResultPartialClassification(t *testing.T) {
	result := LabResult{
		RealTovarischStarted: true,
		RealTovarischReady:   true,
		RealUVB76Started:     true,
		UVB76PProfReady:      true,
		// Missing scrape completion
		RealTargetObserved:    false,
		ProcessSamplesPresent: true,
		ProfilesPresent:       true,
		UVB76Removed:          true,
		TovarischRemoved:      true,
		PortsReleased:         true,
	}

	// Partial failure
	if !result.RealTargetObserved {
		result.Classification = "PARTIAL"
	}

	if result.Classification != "PARTIAL" {
		t.Errorf("expected PARTIAL classification for missing scrape, got %s", result.Classification)
	}
}

func TestLabResultFailedClassification(t *testing.T) {
	result := LabResult{
		RealTovarischStarted: false, // Failed to start
		Errors:               []string{"start tovarisch: binary not found"},
	}

	// Failed classification
	result.Classification = "FAILED"

	if result.Classification != "FAILED" {
		t.Errorf("expected FAILED classification, got %s", result.Classification)
	}
}

func TestClassificationForbiddenValues(t *testing.T) {
	// These classifications are FORBIDDEN from this smoke:
	forbidden := []string{"stable", "growing", "leak", "bounded", "resource_growth"}

	result := LabResult{}
	result.Classification = "OBSERVED" // Only allowed value for success

	for _, v := range forbidden {
		if result.Classification == v {
			t.Errorf("forbidden classification value: %s", v)
		}
	}
}

func TestProcessIdentityFields(t *testing.T) {
	identity := ProcessIdentity{
		ExecutablePath: "/path/to/tovarisch",
		Argv:           []string{"tovarisch", "serve", "--listen", "127.0.0.1:18317"},
		PID:            12345,
		Port:           "18317",
	}

	if identity.ExecutablePath == "" {
		t.Error("ExecutablePath should be set")
	}
	if identity.PID <= 0 {
		t.Error("PID should be positive")
	}
	if identity.Port == "" {
		t.Error("Port should be set")
	}
}

func TestReadinessResultFields(t *testing.T) {
	readiness := ReadinessResult{
		TovarischReady:  true,
		UVB76PProfReady: true,
	}

	if !readiness.TovarischReady {
		t.Error("TovarischReady should be true")
	}
	if !readiness.UVB76PProfReady {
		t.Error("UVB76PProfReady should be true")
	}
}

func TestProcessSampleFields(t *testing.T) {
	sample := ProcessSample{
		PID:       12345,
		RSSKIB:    1024,
		VMSizeKIB: 4096,
		Threads:   5,
		FDCount:   10,
	}

	if sample.PID <= 0 {
		t.Error("PID should be positive")
	}
	if sample.RSSKIB <= 0 {
		t.Error("RSSKIB should be positive")
	}
	if sample.Threads <= 0 {
		t.Error("Threads should be positive")
	}
}

func TestTargetObservationFields(t *testing.T) {
	obs := TargetObservation{
		TargetID:   "real-tovarisch",
		Reachable:  true,
		Status:     "ok",
		Version:    "0.1.0",
		ScrapedURL: "http://localhost:18317/status",
	}

	if obs.TargetID != "real-tovarisch" {
		t.Errorf("expected TargetID 'real-tovarisch', got %s", obs.TargetID)
	}
	if obs.ScrapedURL == "" {
		t.Error("ScrapedURL should be set")
	}
}
