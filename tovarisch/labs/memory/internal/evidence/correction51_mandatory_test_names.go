// correction51_mandatory_test_names.go
//
// correction51Correction09MandatoryTestNames contains the authoritative list of
// mandatory test names for the production finalizer test suite.
//
// NOTE: This file contains only the name declarations. The actual verification
// is performed by TestCorrection51Correction09_MandatoryTestInventory.
//
// This is NOT a test file - it is a data file that can be imported by tests.
package evidence

// correction51Correction09MandatoryTestNames holds mandatory test names by category.
// These are used by the mandatory test inventory verification test.
var correction51Correction09MandatoryTestNames = map[string][]string{
	"nil_dependency": {
		"TestProductionFinalize_NilExecuteLifecycleRejected",
		"TestProductionFinalize_NilCollectProvenanceRejected",
		"TestProductionFinalize_NilPersistFinalEvidenceRejected",
		"TestProductionFinalize_NilVerifyEvidenceBytesRejected",
		"TestProductionFinalize_NilWriteManifestRejected",
		"TestProductionFinalize_NilWriteChecksumsRejected",
	},
	"path_authority": {
		"TestProductionFinalize_ManifestWriterReceivesExactPath",
		"TestProductionFinalize_ChecksumWriterReceivesExactPath",
		"TestProductionFinalize_ManifestWriterReceivesRunDirectoryPath",
		"TestProductionFinalize_ChecksumWriterReceivesRunDirectoryPath",
	},
	"evidence_binding": {
		"TestProductionFinalize_ReturnedPersistedEvidenceMatch",
		"TestProductionFinalize_ReturnedPersistedSchemaMismatchRejected",
		"TestProductionFinalize_ReturnedPersistedSourceCommitMismatchRejected",
		"TestProductionFinalize_ReturnedPersistedImageMismatchRejected",
		"TestProductionFinalize_ReturnedPersistedReachabilityMismatchRejected",
		"TestProductionFinalize_ReturnedPersistedPullMismatchRejected",
		"TestProductionFinalize_ReturnedPersistedNetworkMismatchRejected",
		"TestProductionFinalize_ReturnedPersistedCleanupMismatchRejected",
		"TestProductionFinalize_ReturnedPersistedSourceTreeMismatchRejected",
		"TestProductionFinalize_PersistedUnknownFieldRejected",
		"TestProductionFinalize_PersistedSecondDocumentRejected",
		"TestProductionFinalize_PersistedTrailingDataRejected",
	},
	"manifest_verification": {
		"TestProductionFinalize_PhysicalManifestContainsEvidenceExactlyOnce",
		"TestProductionFinalize_PhysicalManifestMissingEvidenceRejected",
		"TestProductionFinalize_PhysicalManifestDuplicateEvidenceRejected",
		"TestProductionFinalize_PhysicalManifestWrongRunIDRejected",
		"TestProductionFinalize_PhysicalManifestWrongScenarioRejected",
		"TestProductionFinalize_PhysicalManifestInventorySubstitutionRejected",
		"TestProductionFinalize_PhysicalManifestWrongSchemaRejected",
		"TestProductionFinalize_PhysicalManifestUnknownFieldRejected",
		"TestProductionFinalize_PhysicalManifestSecondDocumentRejected",
		"TestProductionFinalize_PhysicalManifestTrailingDataRejected",
	},
	"checksum_authority": {
		"TestProductionFinalize_PhysicalChecksumsContainEvidence",
		"TestProductionFinalize_EvidenceChecksumMatchesBytes",
		"TestProductionFinalize_EvidenceChecksumMismatchRejected",
		"TestProductionFinalize_EvidenceChecksumMissingRejected",
		"TestProductionFinalize_EvidenceChecksumDuplicateRejected",
		"TestProductionFinalize_ChecksumExtraPathRejected",
		"TestProductionFinalize_ChecksumInventorySubstitutionRejected",
		"TestProductionFinalize_ChecksumUppercaseDigestRejected",
		"TestProductionFinalize_ChecksumMalformedLineRejected",
		"TestProductionFinalize_MalformedChecksumsRejected",
	},
	"error_cause": {
		"TestProductionFinalize_ManifestWriteFailurePreservesCause",
		"TestProductionFinalize_ChecksumWriteFailurePreservesCause",
	},
	"exact_one_decoder": {
		"TestDecodeQualifiedExecutionEvidenceExactlyOne_EmptyRejected",
		"TestDecodeQualifiedExecutionEvidenceExactlyOne_UnknownFieldRejected",
		"TestDecodeQualifiedExecutionEvidenceExactlyOne_SecondObjectRejected",
		"TestDecodeQualifiedExecutionEvidenceExactlyOne_SecondScalarRejected",
		"TestDecodeQualifiedExecutionEvidenceExactlyOne_TrailingGarbageRejected",
		"TestDecodeQualifiedExecutionEvidenceExactlyOne_TrailingWhitespaceAccepted",
	},
	"path_resolver": {
		"TestArtifactResolver_EmptyRootRejected",
		"TestArtifactResolver_MissingRootRejected",
		"TestArtifactResolver_RootFileRejected",
		"TestArtifactResolver_SymlinkRootRejected",
		"TestArtifactResolver_IntermediateSymlinkRejected",
		"TestArtifactResolver_FinalSymlinkRejected",
		"TestArtifactResolver_ValidNestedFile",
		"TestArtifactResolver_MissingFileRejected",
	},
	"checksum_error_chain": {
		"TestVerifyPhysicalChecksums_ParseFailurePreservesMalformedChecksums",
		"TestVerifyPhysicalChecksums_InvalidLexicalPathPreservesFullParseChain",
		"TestVerifyPhysicalChecksums_ChildSymlinkPreservesChecksumMismatch",
		"TestVerifyPhysicalChecksums_ChildSymlinkPreservesInvalidArtifactPath",
		"TestVerifyPhysicalChecksums_ValidChecksumsAndInventorySucceeds",
		"TestVerifyPhysicalChecksums_InvalidRootPreservesChecksumMismatch",
		"TestVerifyPhysicalChecksums_InvalidRootPreservesInvalidArtifactRoot",
		"TestVerifyPhysicalChecksums_InvalidRootPreservesInvalidArtifactPath",
		"TestVerifyPhysicalChecksums_MissingArtifactPreservesNotExist",
		"TestVerifyPhysicalChecksums_DigestMismatchPreservesChecksumMismatch",
	},
	"error_identity": {
		"TestErrorIdentity_ErrMalformedChecksumLine",
		"TestErrorIdentity_ErrMalformedChecksumLine_InvalidPath",
		"TestErrorIdentity_ErrInvalidArtifactPath",
		"TestErrorIdentity_ResolveRegularArtifactPath_ErrInvalidArtifactPath",
		"TestErrorIdentity_ErrInvalidArtifactRoot",
		"TestErrorIdentity_ResolveRegularArtifactPath_RootSymlinkRejected",
		"TestErrorIdentity_ResolveRegularArtifactPath_RootFileRejected",
		"TestErrorIdentity_ErrMalformedChecksums",
		"TestErrorIdentity_ErrChecksumMismatch",
		"TestErrorIdentity_ErrProductionEvidenceMismatch",
		"TestErrorIdentity_ErrDuplicateInventoryEntry",
		"TestErrorIdentity_ErrNilDependency",
		"TestErrorIdentity_ErrorSentinelsAreDistinct",
		"TestResolveRegularArtifactPath_ChildNotExist",
		"TestResolveRegularArtifactPath_ChildSymlink",
		"TestResolveRegularArtifactPath_ParentNotExist",
	},
	"checksum_parser": {
		"TestParseChecksumsCanonical_ValidEntry",
		"TestParseChecksumsCanonical_OneSeparatorSpaceRejected",
		"TestParseChecksumsCanonical_ThreeSeparatorSpacesRejected",
		"TestParseChecksumsCanonical_TabSeparatorRejected",
		"TestParseChecksumsCanonical_CRLFRejected",
		"TestParseChecksumsCanonical_MissingFinalLFRejected",
		"TestParseChecksumsCanonical_BlankLineRejected",
		"TestParseChecksumsCanonical_CommentRejected",
		"TestParseChecksumsCanonical_LeadingWhitespaceRejected",
		"TestParseChecksumsCanonical_TrailingWhitespaceRejected",
		"TestParseChecksumsCanonical_EmptyPathRejected",
		"TestParseChecksumsCanonical_DuplicatePathRejected",
		"TestParseChecksumsCanonical_UppercaseDigestRejected",
		"TestParseChecksumsCanonical_EmptyInputRejected",
	},
	"path_validator": {
		"TestValidateArtifactPath_ValidNestedPath",
		"TestValidateArtifactPath_DotDotComponentRejected",
		"TestValidateArtifactPath_DoubleDotInFilenameAccepted",
		"TestValidateArtifactPath_DotComponentRejected",
		"TestValidateArtifactPath_AbsoluteRejected",
		"TestValidateArtifactPath_BackslashRejected",
		"TestValidateArtifactPath_IntermediateSymlinkRejected",
		"TestValidateArtifactPath_FinalSymlinkRejected",
		"TestValidateArtifactPath_EscapeRejected",
		"TestValidateArtifactPath_DirectoryRejected",
		"TestValidateArtifactPath_MissingRejected",
	},
}

// GetAllMandatoryTestNames returns a flat slice of all mandatory test names.
func GetAllMandatoryTestNames() []string {
	var all []string
	for _, names := range correction51Correction09MandatoryTestNames {
		all = append(all, names...)
	}
	return all
}
