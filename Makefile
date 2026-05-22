.PHONY: gate digest

gate:
	./scripts/quality_gate.sh

digest:
	./scripts/make_targeted_digest.sh
