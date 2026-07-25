// qualified_observations.go — Canonical observations for qualified execution.
package dockerlab

import (
	"time"

	"github.com/s1onique/KGB/tovarisch/labs/memory/internal/canarycontrol"
)

type ReachabilityMethod string

const ReachabilityMethodDockerExec ReachabilityMethod = "docker_exec"

type ReachabilityOperationObservation struct {
	Operation         canarycontrol.Operation `json:"operation"`
	ExecExitCode      int                     `json:"exec_exit_code"`
	HTTPStatus        int                     `json:"http_status"`
	ResponseValidated bool                    `json:"response_validated"`
	Mode              string                  `json:"mode,omitempty"`
}

type ReachabilityOperateObservation struct {
	Operation         canarycontrol.Operation `json:"operation"`
	ExecExitCode      int                     `json:"exec_exit_code"`
	HTTPStatus        int                     `json:"http_status"`
	Requested         int                     `json:"requested"`
	Attempted         int                     `json:"attempted"`
	Completed         int                     `json:"completed"`
	ResponseValidated bool                    `json:"response_validated"`
}

type ReachabilityObservations struct {
	Method     ReachabilityMethod `json:"method"`
	NetworkID  string             `json:"network_id"`
	TargetHost string             `json:"target_host"`
	TargetPort int                `json:"target_port"`

	Health       ReachabilityOperationObservation `json:"health"`
	InitialState ReachabilityOperationObservation `json:"initial_state"`
	Operate      ReachabilityOperateObservation   `json:"operate"`
	FinalState   ReachabilityOperationObservation `json:"final_state"`

	Success bool `json:"success"`
}

type ProvenanceBinding struct {
	SourceCommit        string `json:"source_commit"`
	SourceTree          string `json:"source_tree"`
	GitObjectFormat     string `json:"git_object_format"`
	WorkingTreeDirty    bool   `json:"working_tree_dirty"`
	SourceCommitDirty   bool   `json:"source_commit_dirty"`
	VCSModified         bool   `json:"vcs_modified"`
	DockerServerVersion string `json:"docker_server_version"`
	ProducerVersion     string `json:"producer_version"`
	ExecutableSHA256    string `json:"executable_sha256,omitempty"`
}

type ImageObservations struct {
	RequestedReference    string   `json:"requested_reference"`
	InspectedBeforeCreate string   `json:"inspected_id_before_create"`
	InspectedRepoDigests  []string `json:"repo_digests"`
	CreateRequestImage    string   `json:"create_request_image"`
	ContainerInspectImage string   `json:"container_inspect_image_id"`
	ContainerConfigImage  string   `json:"container_inspect_config_image"`
}

type NetworkObservations struct {
	RequestedName       string `json:"requested_name"`
	CreateResponseID    string `json:"create_response_id"`
	InspectResponseID   string `json:"inspected_network_id"`
	ContainerEndpointID string `json:"container_endpoint_network_id"`
	Removed             bool   `json:"removed"`
}

type PullObservations struct {
	ObservationAvailable bool   `json:"observation_available"`
	Attempted            bool   `json:"attempted"`
	AttemptCount         int    `json:"attempt_count"`
	LastReference        string `json:"last_reference,omitempty"`
}

type ContainerObservations struct {
	ID                    string `json:"id"`
	Created               bool   `json:"created"`
	Inspected             bool   `json:"inspected"`
	Started               bool   `json:"started"`
	TerminalStateObserved bool   `json:"terminal_state_observed"`
	Removed               bool   `json:"removed"`
}

type QualifiedExecutionObservations struct {
	SchemaVersion string                   `json:"schema_version"`
	GeneratedAt   time.Time                `json:"generated_at"`
	Image         ImageObservations        `json:"image"`
	Network       NetworkObservations      `json:"network"`
	Pull          PullObservations         `json:"pull"`
	Container     ContainerObservations    `json:"container"`
	Provenance    ProvenanceBinding        `json:"provenance"`
	Reachability  ReachabilityObservations `json:"reachability"`
}

func (o *QualifiedExecutionObservations) SetInspectedImage(id string, repoDigests []string) {
	o.Image.InspectedBeforeCreate = id
	o.Image.InspectedRepoDigests = append([]string(nil), repoDigests...)
}
func (o *QualifiedExecutionObservations) SetCreateRequestImage(id string) {
	o.Image.CreateRequestImage = id
}
func (o *QualifiedExecutionObservations) SetContainerInspect(id, image, configImage, endpoint string) {
	o.Container.ID = id
	o.Container.Inspected = true
	o.Image.ContainerInspectImage = image
	o.Image.ContainerConfigImage = configImage
	o.Network.ContainerEndpointID = endpoint
}
func (o *QualifiedExecutionObservations) SetContainerCreated(id string) {
	o.Container.ID = id
	o.Container.Created = true
}
func (o *QualifiedExecutionObservations) SetContainerStarted() { o.Container.Started = true }
func (o *QualifiedExecutionObservations) SetContainerTerminalState() {
	o.Container.TerminalStateObserved = true
}
func (o *QualifiedExecutionObservations) SetContainerRemoved() { o.Container.Removed = true }
func (o *QualifiedExecutionObservations) SetNetworkCreated(name, createID, inspectID string) {
	o.Network.RequestedName = name
	o.Network.CreateResponseID = createID
	o.Network.InspectResponseID = inspectID
}
func (o *QualifiedExecutionObservations) SetNetworkRemoved() { o.Network.Removed = true }
func (o *QualifiedExecutionObservations) SetPullAudit(attempted bool, count int, ref string) {
	o.Pull.ObservationAvailable = true
	o.Pull.Attempted = attempted
	o.Pull.AttemptCount = count
	o.Pull.LastReference = ref
}
func (o *QualifiedExecutionObservations) SetProvenance(commit, tree, format, docker, producer, executable string) {
	o.Provenance.SourceCommit = commit
	o.Provenance.SourceTree = tree
	o.Provenance.GitObjectFormat = format
	o.Provenance.DockerServerVersion = docker
	o.Provenance.ProducerVersion = producer
	o.Provenance.ExecutableSHA256 = executable
}
func (o *QualifiedExecutionObservations) SetProvenanceDirty(working, commit bool) {
	o.Provenance.WorkingTreeDirty = working
	o.Provenance.SourceCommitDirty = commit
}
func (o *QualifiedExecutionObservations) SetVCSModified(modified bool) {
	o.Provenance.VCSModified = modified
}

// SetReachabilityUnknown is retained only for lifecycle-only hermetic tests;
// production qualification must populate all four canonical operations.
func (o *QualifiedExecutionObservations) SetReachabilityUnknown(networkID string) {
	o.Reachability.Method = ReachabilityMethod("unknown")
	o.Reachability.NetworkID = networkID
}
