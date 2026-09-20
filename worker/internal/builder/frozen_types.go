package builder

import "github.com/RimuruChan/Vertex/worker/internal/artifact"

// Builder and judge consume the same internal immutable artifact contract.
const CheckProtocol = artifact.CheckProtocol

type BlobRef = artifact.BlobRef
type FrozenFile = artifact.FrozenFile
type ProgramDefinition = artifact.ProgramDefinition
type FrozenProgram = artifact.FrozenProgram
type TestDefinition = artifact.TestDefinition
type FrozenTest = artifact.FrozenTest
type FrozenGroup = artifact.FrozenGroup
type FrozenMetadata = artifact.FrozenMetadata
type FrozenSnapshot = artifact.FrozenSnapshot
type ArtifactManifest = artifact.ArtifactManifest
type ArtifactTest = artifact.ArtifactTest
type ArtifactStatement = artifact.ArtifactStatement
type FrozenValidation = artifact.FrozenValidation
