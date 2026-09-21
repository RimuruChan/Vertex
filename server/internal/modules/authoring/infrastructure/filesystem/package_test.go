package filesystem

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func makePackage(entries map[string]string) []byte {
	manifest := authoringdomain.CheckArtifact{SchemaVersion: 1, ToolchainKey: strings.Repeat("b", 64), Snapshot: authoringdomain.CheckSnapshot{SchemaVersion: 1, PolicyVersion: authoringdomain.CheckPolicyVersion, TreeHash: strings.Repeat("a", 64), Metadata: authoringdomain.PackageMetadata{Comparison: authoringdomain.OutputComparison{Kind: "exact"}}}}
	for i := 1; i <= len(entries); i++ {
		name := fmt.Sprintf("%d", i)
		if _, exists := entries[name+".in"]; !exists {
			continue
		}
		manifest.Snapshot.Tests = append(manifest.Snapshot.Tests, authoringdomain.SnapshotTest{ID: name})
		manifest.Tests = append(manifest.Tests, authoringdomain.ArtifactTest{ID: name, Input: authoringdomain.Reference([]byte(entries[name+".in"])), Answer: authoringdomain.Reference([]byte(entries[name+".out"]))})
	}
	manifest.Snapshot.DataHash, _ = manifest.Snapshot.DataFingerprint()
	encoded, err := json.Marshal(manifest)
	Expect(err).NotTo(HaveOccurred())
	withManifest := map[string]string{"artifact.json": string(encoded)}
	for name, body := range entries {
		withManifest[name] = body
	}
	entries = withManifest

	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, body := range entries {
		file, err := writer.Create(name)
		Expect(err).NotTo(HaveOccurred())
		_, err = file.Write([]byte(body))
		Expect(err).NotTo(HaveOccurred())
	}
	Expect(writer.Close()).To(Succeed())
	return buffer.Bytes()
}

type zeroReader struct{}

func (zeroReader) Read(buffer []byte) (int, error) {
	clear(buffer)
	return len(buffer), nil
}

func makeOversizedAggregatePackage() []byte {
	size := MaxPackageBytes/2 + 1
	hash := sha256.New()
	_, err := io.CopyN(hash, zeroReader{}, size)
	Expect(err).NotTo(HaveOccurred())
	ref := authoringdomain.BlobRef{SHA256: hex.EncodeToString(hash.Sum(nil)), Bytes: size}
	manifest := authoringdomain.CheckArtifact{SchemaVersion: 1, ToolchainKey: strings.Repeat("b", 64), Snapshot: authoringdomain.CheckSnapshot{SchemaVersion: 1, PolicyVersion: authoringdomain.CheckPolicyVersion, TreeHash: strings.Repeat("a", 64), Tests: []authoringdomain.SnapshotTest{{ID: "one"}}}, Tests: []authoringdomain.ArtifactTest{{ID: "one", Input: ref, Answer: ref}}}
	manifest.Snapshot.DataHash, _ = manifest.Snapshot.DataFingerprint()
	encoded, err := json.Marshal(manifest)
	Expect(err).NotTo(HaveOccurred())
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	descriptor, err := writer.Create("artifact.json")
	Expect(err).NotTo(HaveOccurred())
	_, err = descriptor.Write(encoded)
	Expect(err).NotTo(HaveOccurred())
	for _, name := range []string{"1.in", "1.out"} {
		file, err := writer.Create(name)
		Expect(err).NotTo(HaveOccurred())
		_, err = io.CopyN(file, zeroReader{}, MaxPackageBytes/2+1)
		Expect(err).NotTo(HaveOccurred())
	}
	Expect(writer.Close()).To(Succeed())
	return buffer.Bytes()
}

var _ = Describe("TestdataPublisher", func() {
	var root string
	var publisher *TestdataPublisher

	BeforeEach(func() {
		root = GinkgoT().TempDir()
		publisher = NewTestdataPublisher(root)
	})

	It("installs a package under its content hash and reports the case count", func() {
		upload, err := publisher.Publish("problem-1", makePackage(map[string]string{
			"1.in": "1 2\n", "1.out": "3\n", "2.in": "3 4\n", "2.out": "7\n",
		}))
		Expect(err).NotTo(HaveOccurred())
		Expect(upload.CaseCount).To(Equal(2))
		Expect(upload.Checker).To(Equal("exact"))
		Expect(upload.StoragePath).To(HavePrefix("problem-1/"))

		installed := filepath.Join(root, filepath.FromSlash(upload.StoragePath))
		Expect(os.ReadFile(filepath.Join(installed, "2.out"))).To(Equal([]byte("7\n")))
	})

	It("rejects obsolete flat checker uploads", func() {
		var buffer bytes.Buffer
		writer := zip.NewWriter(&buffer)
		for _, name := range []string{"1.in", "1.out", "checker.cpp"} {
			file, err := writer.Create(name)
			Expect(err).NotTo(HaveOccurred())
			_, err = file.Write([]byte("old"))
			Expect(err).NotTo(HaveOccurred())
		}
		Expect(writer.Close()).To(Succeed())
		_, err := publisher.Publish("problem", buffer.Bytes())
		Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
	})

	It("gives identical packages the same content-addressed path", func() {
		entries := map[string]string{"1.in": "1\n", "1.out": "1\n"}
		first, err := publisher.Publish("problem-1", makePackage(entries))
		Expect(err).NotTo(HaveOccurred())
		second, err := publisher.Publish("problem-1", makePackage(entries))
		Expect(err).NotTo(HaveOccurred())
		Expect(second.StoragePath).To(Equal(first.StoragePath))
	})

	It("gives different data different paths so an in-flight job keeps its snapshot", func() {
		first, err := publisher.Publish("problem-1", makePackage(map[string]string{"1.in": "1\n", "1.out": "1\n"}))
		Expect(err).NotTo(HaveOccurred())
		second, err := publisher.Publish("problem-1", makePackage(map[string]string{"1.in": "1\n", "1.out": "2\n"}))
		Expect(err).NotTo(HaveOccurred())
		Expect(second.StoragePath).NotTo(Equal(first.StoragePath))
	})

	It("rejects a test whose answer is missing", func() {
		_, err := publisher.Publish("problem-1", makePackage(map[string]string{"1.in": "1\n"}))
		Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
	})

	It("rejects entries the judge side would not recognize", func() {
		_, err := publisher.Publish("problem-1", makePackage(map[string]string{
			"1.in": "1\n", "1.out": "1\n", "run.sh": "rm -rf /",
		}))
		Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
	})

	It("rejects nested entries so an archive cannot write outside the snapshot", func() {
		_, err := publisher.Publish("problem-1", makePackage(map[string]string{
			"../1.in": "1\n", "../1.out": "1\n",
		}))
		Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
	})

	It("rejects a problem path that could escape the testdata root", func() {
		outside := filepath.Join(filepath.Dir(root), "outside")
		_, err := publisher.Publish("../outside", makePackage(map[string]string{
			"1.in": "1\n", "1.out": "1\n",
		}))
		Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
		_, statErr := os.Stat(outside)
		Expect(os.IsNotExist(statErr)).To(BeTrue())
	})

	It("rejects an empty or unreadable archive", func() {
		_, err := publisher.Publish("problem-1", makePackage(nil))
		Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
		_, err = publisher.Publish("problem-1", []byte("not a zip"))
		Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
		entries, readErr := os.ReadDir(root)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	})

	It("rejects aggregate expansion and removes its staging directory", func() {
		archive := makeOversizedAggregatePackage()
		Expect(int64(len(archive))).To(BeNumerically("<", MaxPackageBytes))

		_, err := publisher.Publish("problem-1", archive)
		Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
		entries, readErr := os.ReadDir(root)
		Expect(readErr).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	})

	It("enforces one shared budget while copying entry contents", func() {
		remaining := int64(5)
		var first, second bytes.Buffer
		Expect(copyPackageEntry(bytes.NewBufferString("abc"), &first, "1.in", &remaining)).To(Succeed())
		Expect(remaining).To(Equal(int64(2)))

		err := copyPackageEntry(bytes.NewBufferString("def"), &second, "1.out", &remaining)
		Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
		Expect(second.String()).To(Equal("de"))
	})

	It("refuses to remove an artifact path that escapes the testdata root", func() {
		Expect(publisher.Remove("../outside")).NotTo(Succeed())
		Expect(publisher.Remove("")).NotTo(Succeed())
	})

	It("removes an installed artifact directory", func() {
		upload, err := publisher.Publish("problem-1", makePackage(map[string]string{"1.in": "1\n", "1.out": "1\n"}))
		Expect(err).NotTo(HaveOccurred())
		Expect(publisher.Remove(upload.StoragePath)).To(Succeed())
		_, err = os.Stat(filepath.Join(root, filepath.FromSlash(upload.StoragePath)))
		Expect(os.IsNotExist(err)).To(BeTrue())
	})
})

func TestArtifactStorage(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Authoring Artifact Storage")
}
