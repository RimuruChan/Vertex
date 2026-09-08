package filesystem

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Testdata snapshots", func() {
	It("keeps the previous content-addressed version readable", func() {
		root := GinkgoT().TempDir()
		storage := NewTestdataStorage(root)
		first, err := storage.Materialize("problem-1", testdataZip("1\n", "1\n"))
		Expect(err).NotTo(HaveOccurred())
		Expect(first.CaseCount).To(Equal(1))
		second, err := storage.Materialize("problem-1", testdataZip("2\n", "2\n"))
		Expect(err).NotTo(HaveOccurred())
		Expect(second.CaseCount).To(Equal(1))
		Expect(second.SHA256).NotTo(Equal(first.SHA256))
		Expect(second.StoragePath).NotTo(Equal(first.StoragePath))

		oldInput, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(first.StoragePath), "1.in"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(oldInput)).To(Equal("1\n"))
	})

	It("reuses the same directory for identical content", func() {
		root := GinkgoT().TempDir()
		storage := NewTestdataStorage(root)
		archive := testdataZip("1 2\n", "3\n")
		first, err := storage.Materialize("problem-1", archive)
		Expect(err).NotTo(HaveOccurred())
		second, err := storage.Materialize("problem-1", archive)
		Expect(err).NotTo(HaveOccurred())
		Expect(second.SHA256).To(Equal(first.SHA256))
		Expect(second.StoragePath).To(Equal(first.StoragePath))
	})
})

func testdataZip(input, output string) []byte {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range map[string]string{"1.in": input, "1.out": output} {
		file, err := writer.Create(name)
		Expect(err).NotTo(HaveOccurred())
		_, err = file.Write([]byte(content))
		Expect(err).NotTo(HaveOccurred())
	}
	Expect(writer.Close()).To(Succeed())
	return buffer.Bytes()
}

func TestTestdata(t *testing.T) { RegisterFailHandler(Fail); RunSpecs(t, "Problem Testdata") }
