package producer

// SurfaceRecord mirrors the inventory surface shape needed by the validator.
// Mirroring avoids importing the Python inventory module from Go.
type SurfaceRecord struct {
	ID               string
	Path             string
	Producer         string
	CommittedAllowed bool
	Sensitivity      string
	Sanitizer        string
}

// Registry is the canonical catalog of producer contracts.
//
// It mirrors the inventory surfaces. The validator rejects any surface
// in inventory that lacks a corresponding contract here, and vice versa.
type Registry struct {
	surfaces []SurfaceRecord
	contracts []*ProducerContract
	byID map[string]*ProducerContract
	surfacesByID map[string]SurfaceRecord
}

// NewRegistry constructs a registry from inventory surfaces and contracts.
// The caller MUST ensure lists are deduplicated.
func NewRegistry(surfaces []SurfaceRecord, contracts []*ProducerContract) *Registry {
	r := &Registry{
		surfaces: surfaces,
		contracts: contracts,
		byID: make(map[string]*ProducerContract),
		surfacesByID: make(map[string]SurfaceRecord),
	}
	for _, s := range surfaces {
		r.surfacesByID[s.ID] = s
	}
	for _, c := range contracts {
		r.byID[c.SurfaceID] = c
	}
	return r
}

// ContractByID returns the contract for the given surface ID, or nil.
func (r *Registry) ContractByID(id string) *ProducerContract {
	return r.byID[id]
}

// SurfaceByID returns the inventory surface for the given ID, or zero value.
func (r *Registry) SurfaceByID(id string) (SurfaceRecord, bool) {
	s, ok := r.surfacesByID[id]
	return s, ok
}

// AllContractIDs returns the set of contract surface IDs.
func (r *Registry) AllContractIDs() []string {
	out := make([]string, 0, len(r.contracts))
	for _, c := range r.contracts {
		out = append(out, c.SurfaceID)
	}
	return out
}

// AllSurfaceIDs returns the set of inventory surface IDs.
func (r *Registry) AllSurfaceIDs() []string {
	out := make([]string, 0, len(r.surfaces))
	for _, s := range r.surfaces {
		out = append(out, s.ID)
	}
	return out
}

// Contracts returns all registered producer contracts.
func (r *Registry) Contracts() []*ProducerContract {
	return r.contracts
}

// Surfaces returns all inventory surfaces.
func (r *Registry) Surfaces() []SurfaceRecord {
	return r.surfaces
}

// CountByStatus returns the number of contracts with the given status.
func (r *Registry) CountByStatus(status ProducerStatus) int {
	n := 0
	for _, c := range r.contracts {
		if c.Status == status {
			n++
		}
	}
	return n
}

// CountActiveHighSensitivity returns the number of ACTIVE HIGH-sensitivity
// contracts. Sensitivity comes from the inventory surface.
func (r *Registry) CountActiveHighSensitivity() int {
	n := 0
	for _, c := range r.contracts {
		if c.Status != StatusActive {
			continue
		}
		s, ok := r.SurfaceByID(c.SurfaceID)
		if !ok {
			continue
		}
		if s.Sensitivity == "high" {
			n++
		}
	}
	return n
}
