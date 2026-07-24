// qualified_observations.go — Canonical observation object for the
// qualified execution path.
//
// CORRECTION18: the producer populates each field at the operation
// that actually observes it. The observation object also carries
// the cleanup-absence and provenance fields required by the
// serialized verifier and the gate.

package dockerlab

import "time"

// ProvenanceBinding distinguishes the canonical repository identities
// for the implementation tree. The verifier enforces the Git object
// format and the required lengths.
type ProvenanceBinding struct {
	SourceCommit        string `json:"source_commit"`
	SourceTree          string `json:"source_tree"`
	GitObjectFormat     string `json:"git_object_format"` // "sha1" or "sha256"
	WorkingTreeDirty    bool   `json:"working_tree_dirty"`
	SourceCommitDirty   bool   `json:"source_commit_dirty"`
	VCSModified         bool   `json:"vcs_modified"`
	DockerServerVersion string `json:"docker_server_version"`
	ProducerVersion     string `json:"producer_version"`
	ExecutableSHA256    string `json:"executable_sha256,omitempty"`
}

// ImageObservations captures the immutable image identity observations.
type ImageObservations struct {
	RequestedReference      string   `json:"requested_reference"`
	InspectedBeforeCreate   string   `json:"inspected_id_before_create"`
	InspectedRepoDigests    []string `json:"repo_digests"`
	CreateRequestImage      string   `json:"create_request_image"`
	ContainerInspectImage   string   `json:"container_inspect_image_id"`
	ContainerConfigImage    string   `json:"container_inspect_config_image"`
}

// NetworkObservations captures the canonical network identity observations.
type NetworkObservations struct {
	RequestedName       string `json:"requested_name"`
	CreateResponseID    string `json:"create_response_id"`
	InspectResponseID   string `json:"inspected_network_id"`
	ContainerEndpointID string `json:"container_endpoint_network_id"`
	Removed             bool   `json:"removed"`
}

// PullObservations captures the pull-audit observations.
type PullObservations struct {
	ObservationAvailable bool   `json:"observation_available"`
	Attempted            bool   `json:"attempted"`
	AttemptCount         int    `json:"attempt_count"`
	LastReference        string `json:"last_reference,omitempty"`
}

// ContainerObservations captures the post-create container lifecycle.
type ContainerObservations struct {
	ID                    string `json:"id"`
	Created               bool   `json:"created"`
	Inspected             bool   `json:"inspected"`
	Started               bool   `json:"started"`
	TerminalStateObserved bool   `json:"terminal_state_observed"`
	Removed               bool   `json:"removed"`
}

// QualifiedExecutionObservations is the canonical observation object.
type QualifiedExecutionObservations struct {
	SchemaVersion string                `json:"schema_version"`
	GeneratedAt   time.Time             `json:"generated_at"`
	Image         ImageObservations     `json:"image"`
	Network       NetworkObservations   `json:"network"`
	Pull          PullObservations      `json:"pull"`
	Container     ContainerObservations `json:"container"`
	Provenance    ProvenanceBinding     `json:"provenance"`
}

// SetInspectedImage records the result of ImageInspectWithRaw.
func (o *QualifiedExecutionObservations) SetInspectedImage(id string, repoDigests []string) {
	o.Image.InspectedBeforeCreate = id
	if repoDigests != nil {
		o.Image.InspectedRepoDigests = append([]string{}, repoDigests...)
	}
}

// SetCreateRequestImage records the immutable value passed to ContainerCreate.
func (o *QualifiedExecutionObservations) SetCreateRequestImage(imageID string) {
	o.Image.CreateRequestImage = imageID
}

// SetContainerInspect records the result of post-create ContainerInspect.
func (o *QualifiedExecutionObservations) SetContainerInspect(containerID, imageID, configImage, endpointID string) {
	o.Container.ID = containerID
	o.Container.Inspected = true
	o.Image.ContainerInspectImage = imageID
	o.Image.ContainerConfigImage = configImage
	o.Network.ContainerEndpointID = endpointID
}

// SetContainerCreated marks the container as created.
func (o *QualifiedExecutionObservations) SetContainerCreated(id string) {
	o.Container.ID = id
	o.Container.Created = true
}

// SetContainerStarted marks the container as started.
func (o *QualifiedExecutionObservations) SetContainerStarted() {
	o.Container.Started = true
}

// SetContainerTerminalState marks the terminal state observation.
func (o *QualifiedExecutionObservations) SetContainerTerminalState() {
	o.Container.TerminalStateObserved = true
}

// SetContainerRemoved marks the container as removed (only after
// proven absence via post-remove inspect).
func (o *QualifiedExecutionObservations) SetContainerRemoved() {
	o.Container.Removed = true
}

// SetNetworkCreated records the create-response and inspect-response IDs.
func (o *QualifiedExecutionObservations) SetNetworkCreated(name, createID, inspectID string) {
	o.Network.RequestedName = name
	o.Network.CreateResponseID = createID
	o.Network.InspectResponseID = inspectID
}

// SetNetworkRemoved marks the network as removed (only after proven
// absence via post-remove inspect).
func (o *QualifiedExecutionObservations) SetNetworkRemoved() {
	o.Network.Removed = true
}

// SetPullAudit records the audit counters.
func (o *QualifiedExecutionObservations) SetPullAudit(attempted bool, count int, lastRef string) {
	o.Pull.ObservationAvailable = true
	o.Pull.Attempted = attempted
	o.Pull.AttemptCount = count
	o.Pull.LastReference = lastRef
}

// SetProvenance records the source-tree binding.
func (o *QualifiedExecutionObservations) SetProvenance(commit, tree, format, dockerVer, producer string) {
	o.Provenance.SourceCommit = commit
	o.Provenance.SourceTree = tree
	o.Provenance.GitObjectFormat = format
	o.Provenance.DockerServerVersion = dockerVer
	o.Provenance.ProducerVersion = producer
}

// SetProvenanceDirty marks both the working tree and the source commit as dirty.
func (o *QualifiedExecutionObservations) SetProvenanceDirty(working, commitDirty bool) {
	o.Provenance.WorkingTreeDirty = working
	o.Provenance.SourceCommitDirty = commitDirty
}

// SetVCSModified marks the VCS-modified flag.
func (o *QualifiedExecutionObservations) SetVCSModified(modified bool) {
	o.Provenance.VCSModified = modified
}
