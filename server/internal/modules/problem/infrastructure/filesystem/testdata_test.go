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
	It("rejects paths and empty IDs before creating or removing any directories", func() {
		parent := GinkgoT().TempDir()
		root := filepath.Join(parent, "testdata")
		storage := NewTestdataStorage(root)
		outside := filepath.Join(parent, "keep")
		Expect(os.WriteFile(outside, []byte("keep"), 0o600)).To(Succeed())
		for _, id := range []string{"", ".", "..", "../keep", "nested/problem", `nested\problem`, outside, "C:drive", "a\x00b"} {
			_, err := storage.Materialize(id, testdataZip("1\n", "1\n"))
			Expect(err).To(HaveOccurred(), id)
			Expect(storage.RemoveProblem(id)).NotTo(Succeed(), id)
		}
		_, err := os.Stat(root)
		Expect(os.IsNotExist(err)).To(BeTrue())
		data, err := os.ReadFile(outside)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("keep"))
	})

	It("confines publication even when the resource directory is a symlink", func() {
		root, outside := GinkgoT().TempDir(), GinkgoT().TempDir()
		marker := filepath.Join(outside, "keep")
		Expect(os.WriteFile(marker, []byte("keep"), 0o600)).To(Succeed())
		link := filepath.Join(root, "problem-1")
		if err := os.Symlink(outside, link); err != nil {
			Skip("symlinks are not available: " + err.Error())
		}
		storage := NewTestdataStorage(root)
		_, err := storage.Materialize("problem-1", testdataZip("1\n", "1\n"))
		Expect(err).To(HaveOccurred())
		entries, err := os.ReadDir(outside)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(HaveLen(1))
		Expect(storage.RemoveProblem("problem-1")).To(Succeed())
		data, err := os.ReadFile(marker)
		Expect(err).NotTo(HaveOccurred())
		Expect(string(data)).To(Equal("keep"))
		entries, err = os.ReadDir(root)
		Expect(err).NotTo(HaveOccurred())
		Expect(entries).To(BeEmpty())
	})

	It("removes only the selected resource and treats an absent root as empty", func() {
		root := filepath.Join(GinkgoT().TempDir(), "testdata")
		storage := NewTestdataStorage(root)
		Expect(storage.RemoveProblem("missing")).To(Succeed())
		_, err := storage.Materialize("first", testdataZip("1\n", "1\n"))
		Expect(err).NotTo(HaveOccurred())
		second, err := storage.Materialize("second", testdataZip("2\n", "2\n"))
		Expect(err).NotTo(HaveOccurred())
		Expect(storage.RemoveProblem("first")).To(Succeed())
		_, err = os.Stat(filepath.Join(root, second.StoragePath, "1.in"))
		Expect(err).NotTo(HaveOccurred())
	})

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
