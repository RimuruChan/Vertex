package authoring

// Template is a starter source file offered by the authoring workspace.
type Template struct {
	Kind        string `json:"kind"`
	Name        string `json:"name"`
	Language    string `json:"language"`
	Title       string `json:"title"`
	Description string `json:"description"`
	SourceCode  string `json:"sourceCode"`
}

// Templates returns the built-in testlib starters. They are deliberately
// minimal and compile as-is, so a new problem can reach a green build without
// the author writing boilerplate first.
func Templates() []Template {
	return []Template{
		{
			Kind: KindChecker, Name: "check", Language: "cpp",
			Title:       "标准 token 比较 checker",
			Description: "逐 token 比较选手输出与标准答案,适合唯一解题目。",
			SourceCode:  checkerTokenTemplate,
		},
		{
			Kind: KindChecker, Name: "check", Language: "cpp",
			Title:       "浮点数 checker(1e-6)",
			Description: "按相对或绝对误差 1e-6 比较实数序列。",
			SourceCode:  checkerDoubleTemplate,
		},
		{
			Kind: KindChecker, Name: "check", Language: "cpp",
			Title:       "多解 checker 骨架",
			Description: "同时读取输入、选手输出与标准答案,自行判断答案是否合法。",
			SourceCode:  checkerCustomTemplate,
		},
		{
			Kind: KindValidator, Name: "validate", Language: "cpp",
			Title:       "输入校验器",
			Description: "校验输入格式与数据范围,构建时对每个测试点执行。",
			SourceCode:  validatorTemplate,
		},
		{
			Kind: KindGenerator, Name: "gen", Language: "cpp",
			Title:       "随机数据生成器",
			Description: "使用 testlib rnd,命令行参数决定规模;相同参数生成相同数据。",
			SourceCode:  generatorTemplate,
		},
		{
			Kind: KindSolution, Name: "std", Language: "cpp",
			Title:       "标程",
			Description: "标准解法,构建时用它产生每个测试点的答案。",
			SourceCode:  solutionTemplate,
		},
	}
}

const checkerTokenTemplate = `#include "testlib.h"

// 逐 token 比较:空白字符不敏感,内容必须完全一致。
int main(int argc, char* argv[]) {
    registerTestlibCmd(argc, argv);

    int index = 0;
    while (!ans.seekEof() || !ouf.seekEof()) {
        index++;
        std::string expected = ans.readWord();
        std::string actual = ouf.readWord();
        if (expected != actual) {
            quitf(_wa, "第 %d 个 token 不同: 期望 %s, 实际 %s",
                  index, compress(expected).c_str(), compress(actual).c_str());
        }
    }
    quitf(_ok, "共 %d 个 token", index);
}
`

const checkerDoubleTemplate = `#include "testlib.h"

// 实数比较:相对或绝对误差不超过 EPS 视为正确。
static const double EPS = 1e-6;

int main(int argc, char* argv[]) {
    registerTestlibCmd(argc, argv);

    int index = 0;
    while (!ans.seekEof()) {
        index++;
        double expected = ans.readDouble();
        double actual = ouf.readDouble();
        if (!doubleCompare(expected, actual, EPS)) {
            quitf(_wa, "第 %d 个数不同: 期望 %.10f, 实际 %.10f", index, expected, actual);
        }
    }
    quitf(_ok, "共 %d 个数, 最大误差在 %g 内", index, EPS);
}
`

const checkerCustomTemplate = `#include "testlib.h"

// 多解题:inf 是输入,ouf 是选手输出,ans 是标程输出。
// 通常先分别读入两侧的答案,再验证选手答案确实合法且不劣于标程。
int main(int argc, char* argv[]) {
    registerTestlibCmd(argc, argv);

    int n = inf.readInt();
    long long jury = ans.readLong();
    long long participant = ouf.readLong();

    if (participant > jury) {
        quitf(_fail, "选手答案 %lld 优于标程答案 %lld, 请检查标程", participant, jury);
    }
    if (participant < jury) {
        quitf(_wa, "选手答案 %lld 劣于标程答案 %lld", participant, jury);
    }
    quitf(_ok, "n = %d, answer = %lld", n, jury);
}
`

const validatorTemplate = `#include "testlib.h"

// 校验器在构建时对每个测试点运行一次,失败会让整次构建失败。
// 读入必须严格到位:readInt 的上下界即为题目数据范围。
int main(int argc, char* argv[]) {
    registerValidation(argc, argv);

    int n = inf.readInt(1, 100000, "n");
    inf.readEoln();
    for (int i = 0; i < n; i++) {
        inf.readInt(-1000000000, 1000000000, "a[i]");
        if (i + 1 < n) {
            inf.readSpace();
        }
    }
    inf.readEoln();
    inf.readEof();
}
`

const generatorTemplate = `#include "testlib.h"

// 用法示例: gen 100000 1000000000
// 生成命令中的参数会原样传给 argv, 相同参数总是生成相同数据。
int main(int argc, char* argv[]) {
    registerGen(argc, argv, 1);

    int n = atoi(argv[1]);
    int maxValue = argc > 2 ? atoi(argv[2]) : 1000000000;

    println(n);
    for (int i = 0; i < n; i++) {
        std::cout << rnd.next(-maxValue, maxValue) << " \n"[i + 1 == n];
    }
    return 0;
}
`

const solutionTemplate = `#include <bits/stdc++.h>

int main() {
    std::ios::sync_with_stdio(false);
    std::cin.tie(nullptr);

    int n;
    if (!(std::cin >> n)) {
        return 0;
    }
    long long sum = 0;
    for (int i = 0; i < n; i++) {
        long long value;
        std::cin >> value;
        sum += value;
    }
    std::cout << sum << '\n';
    return 0;
}
`
