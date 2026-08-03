package problem

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func makeTestdataZip(files map[string]string) []byte {
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range files {
		entry, err := writer.Create(name)
		Expect(err).NotTo(HaveOccurred())
		_, err = entry.Write([]byte(content))
		Expect(err).NotTo(HaveOccurred())
	}
	Expect(writer.Close()).To(Succeed())
	return buffer.Bytes()
}

var _ = Describe("Testdata archive extraction", func() {
	var destination string

	BeforeEach(func() {
		var err error
		destination, err = os.MkdirTemp("", "vertex-testdata-*")
		Expect(err).NotTo(HaveOccurred())
		DeferCleanup(os.RemoveAll, destination)
	})

	It("extracts complete numbered input/output pairs", func() {
		archive := makeTestdataZip(map[string]string{
			"1.in": "1\n", "1.out": "1\n",
			"2.in": "2\n", "2.out": "2\n",
		})
		count, err := extractTestdataZip(archive, destination)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(2))
		Expect(os.ReadFile(filepath.Join(destination, "2.in"))).To(Equal([]byte("2\n")))
	})

	It("accepts pairs stored in an archive subdirectory", func() {
		archive := makeTestdataZip(map[string]string{
			"data/1.in": "a\n", "data/1.out": "a\n",
		})
		count, err := extractTestdataZip(archive, destination)
		Expect(err).NotTo(HaveOccurred())
		Expect(count).To(Equal(1))
	})

	It("rejects a missing output pair", func() {
		archive := makeTestdataZip(map[string]string{
			"1.in": "x\n", "1.out": "y\n", "2.in": "z\n",
		})
		_, err := extractTestdataZip(archive, destination)
		Expect(err).To(HaveOccurred())
	})

	It("rejects empty and malformed archives", func() {
		_, err := extractTestdataZip(makeTestdataZip(nil), destination)
		Expect(err).To(HaveOccurred())
		_, err = extractTestdataZip([]byte("not a zip"), destination)
		Expect(err).To(HaveOccurred())
	})
})
