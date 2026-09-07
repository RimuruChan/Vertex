package authoring

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Artifact copying", func() {
	It("validates the legacy hash of manual imports as well as framed build hashes", func(ctx SpecContext) {
		root := GinkgoT().TempDir()
		publisher := NewTestdataPublisher(root)
		source, err := publisher.Publish("source", makePackage(map[string]string{"1.in": "1\n", "1.out": "2\n"}))
		Expect(err).NotTo(HaveOccurred())
		oldPath := filepath.Join(root, filepath.FromSlash(source.StoragePath))
		digest := sha256.Sum256([]byte("1.in1\n1.out2\n"))
		source.SHA256 = hex.EncodeToString(digest[:])
		source.StoragePath = "source/" + source.SHA256
		Expect(os.Rename(oldPath, filepath.Join(root, filepath.FromSlash(source.StoragePath)))).To(Succeed())
		copy, err := publisher.Clone(ctx, "source", "target", *source)
		Expect(err).NotTo(HaveOccurred())
		Expect(copy.SHA256).To(Equal(source.SHA256))
	})
	It("copies regular data and a checker without sharing file identities", func(ctx SpecContext) {
		root := GinkgoT().TempDir()
		publisher := NewTestdataPublisher(root)
		source, err := publisher.Publish("source", makePackage(map[string]string{"1.in": "1\n", "1.out": "2\n", CheckerFileName: "checker"}))
		Expect(err).NotTo(HaveOccurred())
		copy, err := publisher.Clone(ctx, "source", "target", *source)
		Expect(err).NotTo(HaveOccurred())
		Expect(copy.SHA256).To(Equal(source.SHA256))
		Expect(copy.StoragePath).To(Equal("target/" + source.SHA256))
		a, err := os.Stat(filepath.Join(root, filepath.FromSlash(source.StoragePath), "1.out"))
		Expect(err).NotTo(HaveOccurred())
		b, err := os.Stat(filepath.Join(root, filepath.FromSlash(copy.StoragePath), "1.out"))
		Expect(err).NotTo(HaveOccurred())
		Expect(os.SameFile(a, b)).To(BeFalse())
		Expect(publisher.Remove(source.StoragePath)).To(Succeed())
		Expect(os.ReadFile(filepath.Join(root, filepath.FromSlash(copy.StoragePath), CheckerFileName))).To(Equal([]byte("checker")))
		_, err = publisher.Clone(ctx, "target", "target", *copy)
		Expect(err).To(MatchError(ErrPackageTarget))
		Expect(os.ReadFile(filepath.Join(root, filepath.FromSlash(copy.StoragePath), "1.out"))).To(Equal([]byte("2\n")))
	})
	It("rejects missing, altered and nested source data without leaving a target", func(ctx SpecContext) {
		root := GinkgoT().TempDir()
		publisher := NewTestdataPublisher(root)
		source, err := publisher.Publish("source", makePackage(map[string]string{"1.in": "1\n", "1.out": "2\n"}))
		Expect(err).NotTo(HaveOccurred())
		bad := *source
		bad.StoragePath = "../outside"
		_, err = publisher.Clone(ctx, "source", "target", bad)
		Expect(err).To(MatchError(ErrPackageTarget))
		folder := filepath.Join(root, filepath.FromSlash(source.StoragePath))
		Expect(os.WriteFile(filepath.Join(folder, "1.out"), []byte("tampered"), 0644)).To(Succeed())
		_, err = publisher.Clone(ctx, "source", "target", *source)
		Expect(err).To(MatchError(ErrPackageTarget))
		_, err = os.Stat(filepath.Join(root, "target"))
		Expect(os.IsNotExist(err)).To(BeTrue())
		Expect(os.Truncate(filepath.Join(folder, "1.out"), maxPackageFileBytes+1)).To(Succeed())
		_, err = publisher.Clone(ctx, "source", "target", *source)
		Expect(err).To(MatchError(ErrPackageTooBig))
		Expect(os.WriteFile(filepath.Join(folder, "1.out"), []byte("2\n"), 0644)).To(Succeed())
		Expect(os.Mkdir(filepath.Join(folder, "nested"), 0755)).To(Succeed())
		_, err = publisher.Clone(ctx, "source", "target", *source)
		Expect(err).To(MatchError(ErrPackageTarget))
		Expect(os.Remove(filepath.Join(folder, "nested"))).To(Succeed())
		Expect(os.Remove(filepath.Join(folder, "1.out"))).To(Succeed())
		_, err = publisher.Clone(ctx, "source", "target", *source)
		Expect(err).To(MatchError(ErrPackageTarget))
	})
	It("rejects symbolic links in files and source path components", func(ctx SpecContext) {
		root := GinkgoT().TempDir()
		publisher := NewTestdataPublisher(root)
		source, err := publisher.Publish("source", makePackage(map[string]string{"1.in": "1\n", "1.out": "2\n"}))
		Expect(err).NotTo(HaveOccurred())
		folder := filepath.Join(root, filepath.FromSlash(source.StoragePath))
		Expect(os.Remove(filepath.Join(folder, "1.out"))).To(Succeed())
		if err := os.Symlink("1.in", filepath.Join(folder, "1.out")); err != nil {
			Skip("symlink fixture is unavailable: " + err.Error())
		}
		_, err = publisher.Clone(ctx, "source", "target", *source)
		Expect(err).To(MatchError(ErrPackageTarget))
		Expect(os.Symlink(filepath.Join(root, "source"), filepath.Join(root, "alias"))).To(Succeed())
		source.StoragePath = "alias/" + source.SHA256
		_, err = publisher.Clone(ctx, "alias", "target", *source)
		Expect(err).To(MatchError(ErrPackageTarget))
	})
	It("honors cancellation and never overwrites an existing destination", func(ctx SpecContext) {
		root := GinkgoT().TempDir()
		publisher := NewTestdataPublisher(root)
		source, err := publisher.Publish("source", makePackage(map[string]string{"1.in": "1\n", "1.out": "2\n"}))
		Expect(err).NotTo(HaveOccurred())
		cancelled, cancel := context.WithCancel(ctx)
		cancel()
		_, err = publisher.Clone(cancelled, "source", "target", *source)
		Expect(err).To(MatchError(context.Canceled))
		Expect(os.Mkdir(filepath.Join(root, "target"), 0755)).To(Succeed())
		Expect(os.WriteFile(filepath.Join(root, "target", "keep"), []byte("keep"), 0644)).To(Succeed())
		_, err = publisher.Clone(ctx, "source", "target", *source)
		Expect(os.IsExist(err)).To(BeTrue())
		Expect(os.ReadFile(filepath.Join(root, "target", "keep"))).To(Equal([]byte("keep")))
	})
})

var _ = Describe("Copy input rules", func() {
	It("normalizes references and rejects missing release or attribution before persistence", func(ctx SpecContext) {
		packages := &fakePackages{}
		service := newService(packages, &fakeBuilds{})
		_, err := service.Copy(ctx, CopyInput{SourceDomain: " source ", SourceProblem: "001000", SourceVersion: 1, Attribution: " Approved source "})
		Expect(err).NotTo(HaveOccurred())
		Expect(packages.copyInput.SourceDomain).To(Equal("source"))
		Expect(packages.copyInput.SourceProblem).To(Equal("1000"))
		for _, input := range []CopyInput{
			{SourceDomain: "source", SourceProblem: "1000", Attribution: "reason"},
			{SourceDomain: "source", SourceProblem: "1000", SourceVersion: 1},
			{SourceDomain: "../source", SourceProblem: "1000", SourceVersion: 1, Attribution: "reason"},
			{SourceDomain: "source", SourceProblem: "not-a-uuid", SourceVersion: 1, Attribution: "reason"},
			{SourceDomain: "source", SourceProblem: "1000", SourceVersion: 1, Attribution: strings.Repeat("a", 4097)},
		} {
			packages.copyInput = nil
			_, err := service.Copy(ctx, input)
			Expect(err).To(MatchError(ErrInvalidInput))
			Expect(packages.copyInput).To(BeNil())
		}
	})
})
