export const exampleTitle = '出题工作台 · 区间求和'
export const statementZH = `# 区间求和

## 题目描述

给定一个长度为 $n$ 的整数序列 $a_1,a_2,\\ldots,a_n$。你需要回答 $q$ 次询问：对于每个闭区间 $[l,r]$，输出区间内所有元素的和。

$$
S(l,r)=\\sum_{i=l}^{r}a_i
$$

序列在所有询问之间保持不变。下标从 **1** 开始，区间包含左右两个端点。

## 输入格式

第一行包含两个整数 $n,q$，分别表示序列长度和询问数量。

第二行包含 $n$ 个整数 $a_i$。

接下来 $q$ 行，每行包含两个整数 $l,r$，表示一次询问。

## 输出格式

对于每次询问输出一行，包含一个整数，表示对应区间的和。

{{remainingsamples}}

## 样例说明

第一组样例中，序列为 $[1,2,3,4,5]$。

| 询问 | 区间元素 | 结果 |
| --- | --- | --- |
| $[1,3]$ | $1,2,3$ | $6$ |
| $[2,5]$ | $2,3,4,5$ | $14$ |

第二组样例说明序列可以包含负数，答案也可能为负。

## 数据范围与提示

- $1\\le n,q\\le 2\\times10^5$。
- $-10^9\\le a_i\\le10^9$。
- 每次询问满足 $1\\le l\\le r\\le n$。
- 请使用足够宽的整数类型保存答案。

> 序列不会修改。你可以先进行一次预处理，再回答所有询问。
`
export const statementEN = `# Range Sum

## Description

Given a fixed integer array $a$ of length $n$, answer $q$ inclusive range-sum queries. Indices are **1-based**.

## Input

The first line contains $n$ and $q$. The second line contains $n$ integers. Each of the next $q$ lines contains $l$ and $r$.

## Output

Print one integer for each query: $\\sum_{i=l}^{r}a_i$.

{{remainingsamples}}

## Explanation

In the first example, $1+2+3=6$ and $2+3+4+5=14$. Negative values are allowed.

## Constraints

$1\\le n,q\\le200000$, $|a_i|\\le10^9$, and $1\\le l\\le r\\le n$.
`
export const referenceSources: Record<string, string> = {
  'main.cpp':
    '#include "input_reader.hpp"\n#include "range_sum.hpp"\n#include "answer_writer.hpp"\n#include "fast_io.hpp"\n\nint main() {\n    configure_io();\n    auto data = read_input();\n    auto prefix = build_prefix(data.values);\n    for (const auto& query : data.queries) {\n        write_answer(range_sum(prefix, query));\n    }\n}\n',
  'types.hpp':
    '#pragma once\n#include <vector>\nusing Value = long long;\nusing Values = std::vector<Value>;\n',
  'query.hpp': '#pragma once\nstruct Query { int left, right; };\n',
  'problem_input.hpp':
    '#pragma once\n#include "types.hpp"\n#include "query.hpp"\nstruct ProblemInput { Values values; std::vector<Query> queries; };\n',
  'number_reader.hpp':
    '#pragma once\n#include <iostream>\n#include "types.hpp"\ninline Value read_number() { Value value; std::cin >> value; return value; }\n',
  'input_reader.hpp':
    '#pragma once\n#include "problem_input.hpp"\n#include "number_reader.hpp"\ninline ProblemInput read_input() {\n    int n = static_cast<int>(read_number());\n    int q = static_cast<int>(read_number());\n    ProblemInput data;\n    data.values.resize(n);\n    for (auto& value : data.values) value = read_number();\n    for (int i = 0; i < q; ++i) {\n        int left = static_cast<int>(read_number());\n        int right = static_cast<int>(read_number());\n        data.queries.push_back({left, right});\n    }\n    return data;\n}\n',
  'prefix_sum.hpp':
    '#pragma once\n#include "types.hpp"\ninline Values build_prefix(const Values& values) {\n    Values prefix(values.size() + 1, 0);\n    for (std::size_t i = 0; i < values.size(); ++i)\n        prefix[i + 1] = prefix[i] + values[i];\n    return prefix;\n}\n',
  'range_sum.hpp':
    '#pragma once\n#include "prefix_sum.hpp"\n#include "query.hpp"\ninline Value range_sum(const Values& prefix, const Query& query) {\n    return prefix[query.right] - prefix[query.left - 1];\n}\n',
  'answer_writer.hpp':
    '#pragma once\n#include <iostream>\n#include "types.hpp"\ninline void write_answer(Value answer) { std::cout << answer << "\\n"; }\n',
  'fast_io.hpp':
    '#pragma once\n#include <iostream>\ninline void configure_io() { std::ios::sync_with_stdio(false); std::cin.tie(nullptr); }\n',
}
export const generatorCode = `import random
import sys
from patterns import make_values

n, q, seed, bound = map(int, sys.argv[1:])
rng = random.Random(seed)
print(n, q)
print(*make_values(n, bound, rng))
for _ in range(q):
    left = rng.randint(1, n)
    print(left, rng.randint(left, n))
`
export const validatorCode = `import sys

data = list(map(int, sys.stdin.buffer.read().split()))
try:
    n, q = data[:2]
    assert 1 <= n <= 200000 and 1 <= q <= 200000
    assert len(data) == 2 + n + 2 * q
    assert all(abs(x) <= 10**9 for x in data[2:2+n])
    for i in range(q):
        left, right = data[2+n+2*i:4+n+2*i]
        assert 1 <= left <= right <= n
except (AssertionError, ValueError):
    sys.exit(1)
`
