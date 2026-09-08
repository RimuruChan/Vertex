package filesystem

import (
	"archive/zip"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"

	authoringdomain "github.com/RimuruChan/Vertex/server/internal/authoring/domain"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func makePackage(entries map[string]string) []byte {
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
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, name := range []string{"1.in", "1.out"} {
		file, err := writer.Create(name)
		Expect(err).NotTo(HaveOccurred())
		_, err = io.CopyN(file, zeroReader{}, maxPackageUncompressedBytes/2+1)
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
		Expect(upload.Checker).To(Equal("diff"))
		Expect(upload.StoragePath).To(HavePrefix("problem-1/"))

		installed := filepath.Join(root, filepath.FromSlash(upload.StoragePath))
		Expect(os.ReadFile(filepath.Join(installed, "2.out"))).To(Equal([]byte("7\n")))
	})

	It("marks a package that carries a checker as a testlib package", func() {
		upload, err := publisher.Publish("problem-1", makePackage(map[string]string{
			"1.in": "1\n", "1.out": "1\n", CheckerFileName: "int main(){}",
		}))
		Expect(err).NotTo(HaveOccurred())
		Expect(upload.Checker).To(Equal("testlib"))
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

	It("rejects a gap in the test numbering", func() {
		_, err := publisher.Publish("problem-1", makePackage(map[string]string{
			"1.in": "1\n", "1.out": "1\n", "3.in": "3\n", "3.out": "3\n",
		}))
		Expect(err).To(MatchError(authoringdomain.ErrInvalidInput))
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
