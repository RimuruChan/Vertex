package problem

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Testdata snapshots", func() {
	It("keeps the previous content-addressed version readable", func() {
		root := GinkgoT().TempDir()
		firstCount, firstHash, firstPath, err := materializeTestdata(root, "problem-1", testdataZip("1\n", "1\n"))
		Expect(err).NotTo(HaveOccurred())
		Expect(firstCount).To(Equal(1))
		secondCount, secondHash, secondPath, err := materializeTestdata(root, "problem-1", testdataZip("2\n", "2\n"))
		Expect(err).NotTo(HaveOccurred())
		Expect(secondCount).To(Equal(1))
		Expect(secondHash).NotTo(Equal(firstHash))
		Expect(secondPath).NotTo(Equal(firstPath))

		oldInput, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(firstPath), "1.in"))
		Expect(err).NotTo(HaveOccurred())
		Expect(string(oldInput)).To(Equal("1\n"))
	})

	It("reuses the same directory for identical content", func() {
		root := GinkgoT().TempDir()
		archive := testdataZip("1 2\n", "3\n")
		_, firstHash, firstPath, err := materializeTestdata(root, "problem-1", archive)
		Expect(err).NotTo(HaveOccurred())
		_, secondHash, secondPath, err := materializeTestdata(root, "problem-1", archive)
		Expect(err).NotTo(HaveOccurred())
		Expect(secondHash).To(Equal(firstHash))
		Expect(secondPath).To(Equal(firstPath))
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
