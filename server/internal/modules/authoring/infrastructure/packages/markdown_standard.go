package packages

import (
	"bytes"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	mdtext "github.com/yuin/goldmark/text"
)

// Standard Markdown keeps source formatting, but omits the redundant leading
// problem title. Original authoring bytes remain in the content tree.
func standardMarkdown(source []byte, title, filename string, entries map[string]domain.TreeEntry) ([]byte, error) {
	if !utf8.Valid(source) {
		return nil, invalid("Markdown 题面必须是 UTF-8 文本")
	}
	tree := goldmark.DefaultParser().Parse(mdtext.NewReader(source))
	resolver := texRenderer{statementPath: filename, entries: entries, images: map[string]domain.BlobRef{}}
	err := ast.Walk(tree, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch item := node.(type) {
		case *ast.Image:
			destination := markdownText(item.Destination)
			if strings.ToLower(path.Ext(destination)) == ".svg" {
				return ast.WalkStop, invalid("SVG 题面图片尚未提供受限渲染，请使用 PNG/JPEG 或保留为原生归档")
			}
			if _, err := resolver.image(destination); err != nil {
				return ast.WalkStop, err
			}
		case *ast.HTMLBlock, *ast.RawHTML:
			return ast.WalkStop, invalid("标准 Markdown 中的 HTML 需转换为 Markdown 后导出或发布")
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimLeft(source, " \t\r\n")
	line, rest, ok := bytes.Cut(trimmed, []byte("\n"))
	if ok && strings.TrimSpace(string(line)) == "# "+title {
		return bytes.TrimLeft(rest, "\r\n"), nil
	}
	return source, nil
}
