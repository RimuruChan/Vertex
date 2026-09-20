package compile

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestBundleIdentityAndArguments(t *testing.T) {
	dir := t.TempDir()
	main := filepath.Join(dir, "main")
	header := filepath.Join(dir, "header")
	if err := os.WriteFile(main, []byte("int main(){}"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(header, []byte("#pragma once"), 0600); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{"src/main.cpp": main, "include/a.h": header}
	first, err := bundleDigest(context.Background(), "src/main.cpp", files)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(header, []byte("#define CHANGED 1"), 0600); err != nil {
		t.Fatal(err)
	}
	second, err := bundleDigest(context.Background(), "src/main.cpp", files)
	if err != nil || first == second {
		t.Fatal("header change reused compile identity")
	}
	files["src/helper.cpp"] = main
	files["-flag.cpp"] = main
	args, err := bundleCommand("cpp", Supported["cpp"], "src/main.cpp", files, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"./src/main.cpp", "./src/helper.cpp", "./-flag.cpp"} {
		if !slices.Contains(args, name) {
			t.Fatalf("missing translation unit %s: %v", name, args)
		}
	}
	if slices.Contains(args, "./include/a.h") {
		t.Fatal("header was compiled as a translation unit")
	}
	files["../escape"] = main
	if _, err := bundleDigest(context.Background(), "src/main.cpp", files); err == nil {
		t.Fatal("escaping program path accepted")
	}
}
