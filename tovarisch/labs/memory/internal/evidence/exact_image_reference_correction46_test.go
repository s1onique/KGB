package evidence

import "testing"

func TestQualifiedEvidence_ExactEngineImageIDMayBeRequestedReference(t *testing.T) {
	ev := buildValidEvidence()
	ev.Image.RequestedReference = ev.Image.InspectedBeforeCreate
	if result := VerifyQualifiedExecution(ev); !result.Pass {
		t.Fatalf("exact Engine image ID request rejected: %v", result.Errors)
	}
}
