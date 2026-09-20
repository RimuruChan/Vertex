package packages

import "strings"

var mathCommands = func() map[string]bool {
	result := map[string]bool{}
	for _, name := range strings.Fields(`frac dfrac tfrac binom sqrt sum prod coprod int iint iiint oint lim min max log ln exp sin cos tan cot sec csc arcsin arccos arctan sinh cosh tanh gcd lcm det Pr
mathrm mathbf mathbb mathcal mathsf mathtt mathit text operatorname overline underline hat widehat bar vec dot ddot tilde widetilde overbrace underbrace overset underset stackrel substack
alpha beta gamma delta epsilon varepsilon zeta eta theta vartheta iota kappa lambda mu nu xi pi varpi rho varrho sigma varsigma tau upsilon phi varphi chi psi omega
Gamma Delta Theta Lambda Xi Pi Sigma Upsilon Phi Psi Omega
le leq ge geq ne neq approx sim simeq equiv cong propto ll gg in notin ni subset subseteq supset supseteq forall exists nexists neg land lor wedge vee oplus otimes cap cup setminus times cdot div pm mp to rightarrow leftarrow leftrightarrow Rightarrow Leftarrow Leftrightarrow iff implies mapsto infty emptyset varnothing ell
langle rangle lceil rceil lfloor rfloor vert Vert backslash ldots cdots vdots ddots dots quad qquad left right middle big Big bigg Bigg bigl bigr Bigl Bigr biggl biggr Biggl Biggr limits nolimits displaystyle textstyle scriptstyle scriptscriptstyle`) {
		result[name] = true
	}
	return result
}()
var mathEnvironments = map[string]bool{"aligned": true, "gathered": true, "matrix": true, "pmatrix": true, "bmatrix": true, "Bmatrix": true, "vmatrix": true, "Vmatrix": true, "cases": true, "smallmatrix": true}
var mathUnicode = strings.NewReplacer("≤", `\leq `, "≥", `\geq `, "≠", `\neq `, "∈", `\in `, "∉", `\notin `, "∞", `\infty `, "×", `\times `, "·", `\cdot `, "→", `\to `, "∅", `\emptyset `)

func (r *texRenderer) math(value string, display bool) error {
	value = mathUnicode.Replace(strings.TrimSpace(value))
	if err := validateMath(value); err != nil {
		return err
	}
	if display {
		r.out.WriteString("\n\\[\n" + value + "\n\\]\n\n")
	} else {
		r.out.WriteString("\\(" + value + "\\)")
	}
	return nil
}

// Export supports a bounded math vocabulary, not arbitrary TeX programming.
// In particular ^^ character rewriting, macro definitions, I/O and package
// loading must not be smuggled through a Markdown equation.
func validateMath(value string) error {
	if value == "" || len(value) > 16<<10 || strings.Contains(value, "^^") {
		return invalid("公式为空、过大或包含不支持的 TeX 记法")
	}
	braces, left := 0, 0
	environments := []string{}
	for index := 0; index < len(value); index++ {
		switch value[index] {
		case '%', '#', '$':
			return invalid("公式中的特殊字符需要转义")
		case '&':
			if len(environments) == 0 {
				return invalid("公式对齐符需要位于受支持的矩阵环境")
			}
		case '{':
			braces++
			if braces > 64 {
				return invalid("公式嵌套过深")
			}
		case '}':
			braces--
			if braces < 0 {
				return invalid("公式花括号不匹配")
			}
		case '\\':
			index++
			if index >= len(value) {
				return invalid("公式末尾存在孤立反斜杠")
			}
			if strings.ContainsRune(`{}_%#$,;:! |\`, rune(value[index])) {
				continue
			}
			start := index
			for index < len(value) && (value[index] >= 'a' && value[index] <= 'z' || value[index] >= 'A' && value[index] <= 'Z') {
				index++
			}
			name := value[start:index]
			if name == "begin" || name == "end" {
				if index >= len(value) || value[index] != '{' {
					return invalid("公式环境名称无效")
				}
				stop := strings.IndexByte(value[index+1:], '}')
				if stop < 0 {
					return invalid("公式环境未闭合")
				}
				stop += index + 1
				environment := value[index+1 : stop]
				if !mathEnvironments[environment] {
					return invalid("公式环境 %q 暂不支持", environment)
				}
				if name == "begin" {
					environments = append(environments, environment)
					if len(environments) > 16 {
						return invalid("公式环境嵌套过深")
					}
				} else {
					if len(environments) == 0 || environments[len(environments)-1] != environment {
						return invalid("公式环境不匹配")
					}
					environments = environments[:len(environments)-1]
				}
				index = stop
				continue
			}
			if !mathCommands[name] {
				return invalid("公式命令 \\%s 暂不支持安全转换；请提供 TeX/PDF 题面", name)
			}
			if name == "left" {
				left++
			}
			if name == "right" {
				left--
				if left < 0 {
					return invalid("公式 left/right 不匹配")
				}
			}
			if name == "middle" && left == 0 {
				return invalid("公式 middle 缺少 left/right")
			}
			index--
		}
	}
	if braces != 0 || left != 0 || len(environments) != 0 {
		return invalid("公式分组、定界符或环境未闭合")
	}
	return nil
}
