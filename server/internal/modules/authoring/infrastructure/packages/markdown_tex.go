package packages

import (
	"bytes"
	"fmt"
	"html"
	"net/url"
	"path"
	"regexp"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/RimuruChan/Vertex/server/internal/modules/authoring/domain"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	mdtext "github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var texEscapes = strings.NewReplacer(
	`\`, `\textbackslash{}`, `{`, `\{`, `}`, `\}`, `$`, `\$`, `&`, `\&`,
	`#`, `\#`, `%`, `\%`, `_`, `\_`, `~`, `\textasciitilde{}`, `^`, `\textasciicircum{}`, `<`, `\textless{}`, `>`, `\textgreater{}`, `|`, `\textbar{}`,
)
var sampleHeading = regexp.MustCompile(`(?i)^(sample(?:s| input| output)?|examples?|样例(?:输入|输出)?|输入样例|输出样例)(?:\s*\d+)?$`)

type texMath struct {
	ast.BaseInline
	value   string
	display bool
}

var texMathKind = ast.NewNodeKind("AuthoringMath")

func (n *texMath) Kind() ast.NodeKind            { return texMathKind }
func (n *texMath) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

type texMathParser struct{}

type texSample struct {
	ast.BaseInline
	command string
}

var texSampleKind = ast.NewNodeKind("AuthoringSample")

func (n *texSample) Kind() ast.NodeKind            { return texSampleKind }
func (n *texSample) Dump(source []byte, level int) { ast.DumpHelper(n, source, level, nil, nil) }

type texSampleParser struct{}

func (*texSampleParser) Trigger() []byte { return []byte{'{'} }
func (*texSampleParser) Parse(_ ast.Node, reader mdtext.Reader, _ parser.Context) ast.Node {
	line, _ := reader.PeekLine()
	for _, command := range []string{"nextsample", "remainingsamples"} {
		marker := "{{" + command + "}}"
		if bytes.HasPrefix(line, []byte(marker)) {
			reader.Advance(len(marker))
			return &texSample{command: command}
		}
	}
	return nil
}

func (*texMathParser) Trigger() []byte { return []byte{'$'} }
func (*texMathParser) Parse(_ ast.Node, reader mdtext.Reader, _ parser.Context) ast.Node {
	line, _ := reader.PeekLine()
	width := 1
	if bytes.HasPrefix(line, []byte("$$")) {
		width = 2
	}
	if len(line) <= width || line[width] == ' ' || line[width] == '\n' {
		return nil
	}
	for index := width; index < len(line); index++ {
		if line[index] == '\\' {
			index++
			continue
		}
		if line[index] != '$' || width == 2 && (index+1 >= len(line) || line[index+1] != '$') {
			continue
		}
		if index == width || line[index-1] == ' ' || width == 1 && index+1 < len(line) && line[index+1] >= '0' && line[index+1] <= '9' {
			continue
		}
		reader.Advance(index + width)
		return &texMath{value: string(line[width:index]), display: width == 2}
	}
	return nil
}

type texConversion struct {
	data    []byte
	images  map[string]domain.BlobRef
	unicode bool
	links   bool
}
type texRenderer struct {
	source                                  []byte
	title                                   string
	statementPath                           string
	entries                                 map[string]domain.TreeEntry
	out                                     strings.Builder
	images                                  map[string]domain.BlobRef
	nodes, listDepth, enumDepth, restricted int
	links                                   bool
}

// markdownToTeX emits a problemtools-compatible fragment. It never passes raw
// HTML or arbitrary math commands through to a TeX engine and never executes it.
func markdownToTeX(source []byte, title, statementPath string, entries map[string]domain.TreeEntry) (*texConversion, error) {
	if !utf8.Valid(source) || !utf8.ValidString(title) || strings.ContainsAny(title, "\r\n") || strings.Contains(title, "^^") {
		return nil, invalid("题面必须为 UTF-8，标题必须为单行")
	}
	for _, r := range string(source) + title {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return nil, invalid("题面含不支持的控制字符")
		}
	}
	source = bytes.ReplaceAll(source, []byte("\r\n"), []byte("\n"))
	markdown := goldmark.New(goldmark.WithExtensions(extension.GFM), goldmark.WithParserOptions(parser.WithInlineParsers(util.Prioritized(&texMathParser{}, 150), util.Prioritized(&texSampleParser{}, 150))))
	document := markdown.Parser().Parse(mdtext.NewReader(source))
	render := &texRenderer{source: source, title: title, statementPath: statementPath, entries: entries, images: map[string]domain.BlobRef{}}
	fmt.Fprintf(&render.out, "%%%% plainproblemname: %s\n\\problemname{%s}\n\n", title, texEscapes.Replace(title))
	if err := render.node(document, 0); err != nil {
		return nil, err
	}
	if render.out.Len() > 8<<20 {
		return nil, domain.ErrPackageTooBig
	}
	nonASCII := false
	for _, r := range string(source) + title {
		if r > 127 {
			nonASCII = true
			break
		}
	}
	return &texConversion{data: []byte(render.out.String()), images: render.images, unicode: nonASCII, links: render.links}, nil
}
func (r *texRenderer) children(node ast.Node, depth int) error {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if err := r.node(child, depth+1); err != nil {
			return err
		}
	}
	return nil
}
func (r *texRenderer) wrap(node ast.Node, depth int, before, after string) error {
	r.out.WriteString(before)
	if err := r.children(node, depth); err != nil {
		return err
	}
	r.out.WriteString(after)
	return nil
}
func markdownText(value []byte) string {
	return html.UnescapeString(string(util.UnescapePunctuations(value)))
}
func (r *texRenderer) node(node ast.Node, depth int) error {
	r.nodes++
	if depth > 64 || r.nodes > 100000 || r.out.Len() > 8<<20 {
		return domain.ErrPackageTooBig
	}
	switch n := node.(type) {
	case *ast.Document:
		return r.children(n, depth)
	case *ast.Heading:
		name := strings.TrimSpace(markdownText(n.Text(r.source)))
		if n.Level == 1 && n.PreviousSibling() == nil && name == r.title {
			return nil
		}
		if sampleHeading.MatchString(name) {
			return invalid("标准题包题面不包含样例；请把 %q 对应的数据配置为样例测试", name)
		}
		command := "section"
		if n.Level >= 3 {
			command = "subsection"
		}
		r.restricted++
		defer func() { r.restricted-- }()
		return r.wrap(n, depth, "\\"+command+"*{", "}\n\n")
	case *ast.Paragraph:
		raw := strings.TrimSpace(string(n.Lines().Value(r.source)))
		if strings.HasPrefix(raw, "$$") {
			if !strings.HasSuffix(raw, "$$") || len(raw) < 4 {
				return invalid("块公式需要成对的 $$，中间不能包含空段落")
			}
			return r.math(raw[2:len(raw)-2], true)
		}
		return r.wrap(n, depth, "", "\n\n")
	case *ast.TextBlock:
		return r.children(n, depth)
	case *ast.Text:
		r.out.WriteString(texEscapes.Replace(markdownText(n.Segment.Value(r.source))))
		if n.HardLineBreak() {
			r.out.WriteString("\\newline\n")
		} else if n.SoftLineBreak() {
			r.out.WriteByte(' ')
		}
	case *ast.String:
		r.out.WriteString(texEscapes.Replace(markdownText(n.Value)))
	case *ast.Emphasis:
		command := "emph"
		if n.Level == 2 {
			command = "textbf"
		}
		return r.wrap(n, depth, "\\"+command+"{", "}")
	case *ast.CodeSpan:
		r.out.WriteString("\\texttt{")
		r.out.WriteString(texEscapes.Replace(strings.ReplaceAll(string(n.Text(r.source)), "\n", " ")))
		r.out.WriteByte('}')
	case *ast.FencedCodeBlock:
		return r.code(n.Lines().Value(r.source))
	case *ast.CodeBlock:
		return r.code(n.Lines().Value(r.source))
	case *ast.Blockquote:
		return r.wrap(n, depth, "\\begin{quote}\n", "\\end{quote}\n\n")
	case *ast.List:
		r.listDepth++
		defer func() { r.listDepth-- }()
		if r.listDepth > 4 {
			return invalid("列表超过 TeX 支持的四层嵌套")
		}
		environment := "itemize"
		prefix := ""
		if n.IsOrdered() {
			r.enumDepth++
			defer func() { r.enumDepth-- }()
			environment = "enumerate"
			if n.Start != 1 {
				prefix = "\\setcounter{enum" + []string{"i", "ii", "iii", "iv"}[r.enumDepth-1] + "}{" + strconv.Itoa(n.Start-1) + "}\n"
			}
		}
		return r.wrap(n, depth, "\\begin{"+environment+"}\n"+prefix, "\\end{"+environment+"}\n\n")
	case *ast.ListItem:
		return r.wrap(n, depth, "\\item ", "\n")
	case *ast.ThematicBreak:
		r.out.WriteString("\\par\\noindent\\rule{\\linewidth}{0.4pt}\n\n")
	case *ast.Link:
		destination := markdownText(n.Destination)
		if err := safeLink(destination); err != nil {
			return err
		}
		r.links = true
		if err := r.children(n, depth); err != nil {
			return err
		}
		r.out.WriteString(" (\\texttt{" + texEscapes.Replace(destination) + "})")
	case *ast.AutoLink:
		destination := string(n.URL(r.source))
		if err := safeLink(destination); err != nil {
			return err
		}
		r.links = true
		r.out.WriteString("\\texttt{" + texEscapes.Replace(destination) + "}")
	case *ast.Image:
		if r.restricted > 0 {
			return invalid("标题和表格内的图片请提供单独 TeX/PDF 题面")
		}
		target, err := r.image(markdownText(n.Destination))
		if err != nil {
			return err
		}
		r.out.WriteString("\n\\begin{center}\\includegraphics[width=\\linewidth,height=0.6\\textheight,keepaspectratio]{" + target + "}")
		caption := strings.TrimSpace(markdownText(n.Text(r.source)))
		if caption != "" {
			r.out.WriteString("\\par\\small " + texEscapes.Replace(caption))
		}
		r.out.WriteString("\\end{center}\n")
	case *texMath:
		return r.math(n.value, n.display)
	case *texSample:
		return invalid("样例插入指令 %s 需要 Kattis 2025-09；legacy 模板不能可靠保留其位置", n.command)
	case *extast.Table:
		if len(n.Alignments) == 0 || len(n.Alignments) > 8 {
			return invalid("表格列数超过转换范围")
		}
		var columns strings.Builder
		for _, alignment := range n.Alignments {
			letter := "l"
			if alignment == extast.AlignCenter {
				letter = "c"
			}
			if alignment == extast.AlignRight {
				letter = "r"
			}
			columns.WriteString(letter)
		}
		r.restricted++
		defer func() { r.restricted-- }()
		return r.wrap(n, depth, "\\begin{center}\\begin{tabular}{"+columns.String()+"}\\hline\n", "\\hline\\end{tabular}\\end{center}\n\n")
	case *extast.TableHeader, *extast.TableRow:
		for cell := n.FirstChild(); cell != nil; cell = cell.NextSibling() {
			if cell != n.FirstChild() {
				r.out.WriteString(" & ")
			}
			if err := r.node(cell, depth+1); err != nil {
				return err
			}
		}
		r.out.WriteString(" \\\\\n")
		if _, header := n.(*extast.TableHeader); header {
			r.out.WriteString("\\hline\n")
		}
	case *extast.TableCell:
		if _, header := n.Parent().(*extast.TableHeader); header {
			return r.wrap(n, depth, "\\textbf{", "}")
		}
		return r.children(n, depth)
	case *extast.TaskCheckBox:
		if n.IsChecked {
			r.out.WriteString("[x] ")
		} else {
			r.out.WriteString("[ ] ")
		}
	default:
		return invalid("Markdown 元素 %s 尚不能可靠转换；请提供 TeX/PDF 题面或导出原生归档", node.Kind().String())
	}
	return nil
}
func (r *texRenderer) code(value []byte) error {
	r.out.WriteString("\\begin{quote}\\ttfamily\\raggedright\\obeylines\\obeyspaces\n")
	r.out.WriteString(texEscapes.Replace(strings.ReplaceAll(string(value), "\t", "    ")))
	r.out.WriteString("\n\\end{quote}\n\n")
	return nil
}
func safeLink(value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "" && parsed.Scheme != "https" && parsed.Scheme != "http" && parsed.Scheme != "mailto" {
		return invalid("不支持的链接协议")
	}
	return nil
}
func (r *texRenderer) image(destination string) (string, error) {
	parsed, err := url.Parse(destination)
	if err != nil || parsed.Scheme != "" || parsed.Host != "" || parsed.RawQuery != "" || parsed.Fragment != "" || strings.HasPrefix(parsed.Path, "/") {
		return "", invalid("图片必须引用题包内的公开图片附件")
	}
	name := path.Clean(path.Join(path.Dir(r.statementPath), parsed.Path))
	var entry *domain.TreeEntry
	for _, item := range r.entries {
		if item.Path == name {
			copy := item
			entry = &copy
			break
		}
	}
	if entry == nil || entry.Kind != domain.EntryAsset {
		return "", invalid("图片不是已声明的公开附件：%s", destination)
	}
	extension := strings.ToLower(path.Ext(name))
	if extension != ".png" && extension != ".jpg" && extension != ".jpeg" && extension != ".pdf" {
		return "", invalid("legacy 图片只支持 PNG/JPEG/PDF")
	}
	target := "assets/" + entry.Blob.SHA256 + extension
	r.images[target] = entry.Blob
	return target, nil
}
