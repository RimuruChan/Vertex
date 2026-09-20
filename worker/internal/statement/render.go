// Package statement renders untrusted TeX in a separate, resource-bounded native
// sandbox. Its inputs must contain public statement materials only.
package statement

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/RimuruChan/Vertex/worker/internal/run"
	"github.com/RimuruChan/Vertex/worker/internal/verdict"
)

const MaxPDFBytes = 32 << 20
const toolchainManifest = "/usr/share/vertex/statement-toolchain.txt"
const wrapperName = "__vertex_statement.tex"

//go:embed polygon.tex
var polygonLayout string

// The wrapper is deliberately a fixed program, not a shell command assembled
// from package paths. chdir preserves standard TeX relative image/input paths.
const launcher = `import os, subprocess, sys
root = os.getcwd()
os.chdir(sys.argv[1])
env = dict(os.environ)
env.update(HOME=root, TEXMFVAR=root+'/.tex-cache', TEXMFCACHE=root+'/.tex-cache',
           TEXMFSYSVAR='/usr/share/vertex/texmf-var',
           openout_any='p', shell_escape='f')
os.makedirs(root+'/.tex-cache', exist_ok=True)
# Native Landlock/seccomp/credentials are the security boundary. XeTeX avoids
# executing embedded Lua and supports Unicode/OpenType without a Lua font cache.
result = subprocess.run(['/usr/bin/xelatex', '--no-shell-escape',
    '--interaction=nonstopmode', '--halt-on-error', '--jobname=vertex-statement',
    '--output-directory='+root, '__vertex_statement.tex'], env=env)
sys.exit(result.returncode)
`

const preamble = `\documentclass[11pt,a4paper]{article}
\usepackage[margin=24mm]{geometry}
\usepackage{fontspec,amsmath,amssymb,graphicx,wrapfig,tabularx,booktabs,verbatim,listings,multirow,epigraph,fvextra}
\usepackage[normalem]{ulem}
\usepackage[hidelinks]{hyperref}
\setmainfont{Noto Serif CJK SC}
\setsansfont{Noto Sans CJK SC}
\setmonofont{Latin Modern Mono}
\NewDocumentCommand{\problemname}{o g}{\IfNoValueTF{#2}{\section*{\vertextitle}}{\section*{#2}}}
\newcommand{\illustration}[3]{\begin{wrapfigure}{r}{#1\textwidth}\centering\includegraphics[width=\linewidth]{#2}\caption{#3}\end{wrapfigure}}
\newenvironment{Input}{\section*{Input}}{\par}
\newenvironment{Output}{\section*{Output}}{\par}
\newenvironment{Interaction}{\section*{Interaction}}{\par}
\newcount\vertexsampleindex
\newcommand{\nextsample}{\ifnum\vertexsampleindex<\vertexsamplecount\global\advance\vertexsampleindex by 1\relax\csname vertexsample\the\vertexsampleindex\endcsname\else\PackageError{vertex}{No remaining sample test cases}{}\fi}
\newcommand{\remainingsamples}{\loop\ifnum\vertexsampleindex<\vertexsamplecount\nextsample\repeat}
\pagestyle{plain}
\begin{document}
`

// Fingerprint includes the renderer policy, wrapper, engine and installed TeX
// package/font versions. A node without the configured renderer fails clearly.
func Fingerprint(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	version, err := exec.CommandContext(ctx, "xelatex", "--version").Output()
	if err != nil {
		return "", fmt.Errorf("TeX renderer unavailable: %w", err)
	}
	manifest, err := os.ReadFile(toolchainManifest)
	if err != nil || len(manifest) == 0 || len(manifest) > 1<<20 {
		return "", fmt.Errorf("TeX renderer toolchain manifest unavailable")
	}
	sum := sha256.Sum256([]byte("vertex-statement-3\x00" + launcher + preamble + polygonLayout + string(version) + string(manifest)))
	return hex.EncodeToString(sum[:]), nil
}

// Render never stages judge data/programs. ExportFile rejects symlinks and
// directories, and the only returned file is a bounded PDF in the caller's dir.
func Render(ctx context.Context, sandbox *run.Client, scratch, entry string, files map[string]string, destination, dialect string, settings ...Options) error {
	if dialect != "" && dialect != "polygon" {
		return fmt.Errorf("unsupported TeX statement dialect")
	}
	if err := validateInputs(entry, files); err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	work, err := os.MkdirTemp(scratch, "statement-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(work)
	options := Options{}
	if len(settings) > 0 {
		options = settings[0]
	}
	sampleCommands, sampleFiles, err := prepareSamples(work, options)
	if err != nil {
		return err
	}
	wrapper := filepath.Join(work, wrapperName)
	contents := preamble + sampleCommands + "\\input{\\detokenize{" + path.Base(entry) + "}}\n\\remainingsamples\n\\end{document}\n"
	if dialect == "polygon" {
		contents = preamble + sampleCommands + polygonLayout + "\n\\input{\\detokenize{" + path.Base(entry) + "}}\n\\end{document}\n"
	}
	if err := os.WriteFile(wrapper, []byte(contents), 0600); err != nil {
		return err
	}
	env, err := sandbox.Create(ctx, run.EnvironmentPolicy{MemoryKB: 1024 * 1024, Processes: 8})
	if err != nil {
		return err
	}
	defer env.Close()
	inputs := map[string]run.InputFile{}
	for name, file := range files {
		inputs["materials/"+name] = run.InputFile{Path: file}
	}
	directory := path.Join("materials", path.Dir(entry))
	for name, file := range sampleFiles {
		inputs[path.Join(directory, name)] = run.InputFile{Path: file}
	}
	inputs[path.Join(directory, wrapperName)] = run.InputFile{Path: wrapper}
	if err := env.PutFiles(ctx, inputs); err != nil {
		return err
	}
	result, err := env.Run(ctx, run.Execution{
		Command:     []string{"/usr/bin/python3", "-c", launcher, directory},
		Environment: []string{"LANG=C.UTF-8"},
		StdoutPath:  filepath.Join(work, "stdout"), StderrPath: filepath.Join(work, "stderr"),
		Limits: run.Limits{CPUTime: 30 * time.Second, WallTime: 40 * time.Second, MemoryKB: 1024 * 1024, Processes: 8, OutputBytes: MaxPDFBytes},
	})
	if err != nil {
		return fmt.Errorf("render TeX: %w", err)
	}
	if result.Meta == nil {
		return fmt.Errorf("TeX renderer returned no execution metadata")
	}
	if verdict.FromSandboxMeta(result.Meta) != "" {
		return fmt.Errorf("TeX rendering failed: %s\n%s", result.Meta.ExitDescription(), boundedLog(result.Stdout+"\n"+result.Stderr))
	}
	if err := env.ExportFile(ctx, "vertex-statement.pdf", destination, MaxPDFBytes); err != nil {
		return fmt.Errorf("export rendered PDF: %w", err)
	}
	file, err := os.Open(destination)
	if err != nil {
		return err
	}
	defer file.Close()
	var header [5]byte
	if _, err := file.Read(header[:]); err != nil || string(header[:]) != "%PDF-" {
		return fmt.Errorf("TeX renderer did not produce a PDF")
	}
	return nil
}

func validateInputs(entry string, files map[string]string) error {
	if _, exists := files[entry]; !exists {
		return fmt.Errorf("TeX statement source is missing")
	}
	if len(files) > 1000 {
		return fmt.Errorf("too many statement files")
	}
	for name := range files {
		if err := run.ValidateInputPath(name); err != nil {
			return err
		}
		// TeX catcode/^^ rewriting must not turn a staged filename into markup.
		if strings.ContainsAny(name, "{}%#^\r\n") || strings.HasPrefix(path.Base(name), "__vertex_") {
			return fmt.Errorf("unsupported TeX material path: %s", name)
		}
	}
	return nil
}

func boundedLog(value string) string {
	value = strings.ReplaceAll(strings.ToValidUTF8(value, "�"), "\x00", "�")
	if len(value) > 8192 {
		value = string([]rune(value[:8192])) + "\n…"
	}
	return value
}
