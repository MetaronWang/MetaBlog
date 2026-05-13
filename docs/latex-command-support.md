# LaTeX 命令支持范围

本文档说明 MetaBlog 当前支持的 LaTeX 命令、环境、处理策略和已知边界。它面向写作者和维护者：写作者可以用它判断哪些写法可用；维护者可以用它理解某类命令应由 Go parser、LaTeXML 还是渲染层处理。

MetaBlog 不是完整 TeX 引擎。它只解析 `\begin{document}` 和 `\end{document}` 之间的正文内容；导言区不会作为正文渲染。

## 1. 支持策略概览

MetaBlog 对 LaTeX 的支持分为三类：

| 类型 | 说明 | 代表内容 |
| --- | --- | --- |
| Go 手写解析 | 由词法层、block parser 和 inline parser 转为 AST，再由 HTML renderer 渲染。 | 章节、段落、列表、图片外壳、table 外壳、引用、样式命令。 |
| LaTeXML 托管 | 复杂块先抽离为占位符，交给 LaTeXML 转成 HTML fragment，再嵌入页面。 | `tabular`、`tabularx`、`algorithm`、`algorithm*`。 |
| 浏览器端渲染 | MetaBlog 只识别边界和编号，最终显示交给前端运行时。 | KaTeX 数学公式。 |

总体原则：

1. 结构清晰、语义稳定的 LaTeX 写法尽量手写解析。
2. 表格主体和算法这类包语义复杂的块交给 LaTeXML。
3. 未知命令和未知环境尽量降级保留内容，同时记录 warning。

## 2. 快速支持矩阵

| 类别 | 支持内容 | 状态 |
| --- | --- | --- |
| 源码组织 | `\input{...}`、`\include{...}` | 支持，递归展开。 |
| 注释 | `%`、`\%` | 支持；原样环境中不处理注释。 |
| 元信息 | `\title`、`\author`、`\defInstitution` | 支持项目自定义格式。 |
| 摘要关键词 | `abstract`、`keywords`、`IEEEkeywords` | 支持，原位置渲染。 |
| 章节 | `\section`、`\subsection`、`\subsubsection`、`\appendices` | 支持。 |
| 交叉引用 | `\label`、`\ref` | 支持。 |
| 文献引用 | `\cite`、`\bibliography`、`\bibliographystyle` | 支持简化 IEEE 风格。 |
| 段落列表 | 普通段落、`itemize`、`enumerate`、`description` | 支持嵌套。 |
| 图片 | `figure`、`figure*`、`\includegraphics`、`\subfloat` | 支持。 |
| 表格 | `table`、`table*`、`tabular`、`tabularx` | table 外壳手写解析，tabular 主体 LaTeXML。 |
| 算法 | `algorithm`、`algorithm*` | LaTeXML 托管。 |
| 公式 | `$...$`、`\(...\)`、`\[...\]`、`equation`、`align` | 支持边界、编号和 KaTeX 渲染。 |
| 文本样式 | 粗体、斜体、颜色、字号、对齐声明等 | 支持常见命令。 |
| 链接 | `\url`、`\href` | 支持，危险协议降级。 |
| 原样文本 | `verbatim`、`lstlisting`、`minted`、`\verb` | 支持为代码文本框。 |
| 自定义框 | `tcb` | 支持可折叠标题文本框。 |
| 条件编译 | `\iffalse...\fi` | 暂不支持。 |
| 宏展开 | `\newcommand`、`\def` 等 | 暂不支持完整宏展开。 |

## 3. 源码加载和预处理

### 3.1 文档范围

MetaBlog 只处理：

```latex
\begin{document}
...
\end{document}
```

导言区内容不会作为正文解析。LaTeXML 托管复杂块时会使用项目内置 wrapper 加载必要包，而不是直接复用原文导言区。

### 3.2 `\input{...}` 和 `\include{...}`

支持递归展开：

```latex
\input{section/introduction}
\include{section/experiments}
```

规则：

1. 当前实现中 `\include` 与 `\input` 等价。
2. 不模拟 TeX 的分页、aux 文件和 `\includeonly`。
3. 路径没有扩展名时自动补 `.tex`。
4. 优先按当前文件目录查找，再按主文件根目录查找。
5. 循环 input/include 会被拦截并记录 warning。
6. 重复 input/include 会记录 warning，但不会自动去重。
7. 只支持 `{...}` 参数格式。
8. `\includegraphics` 不会被误识别为 `\include`。
9. 原样环境和 `\verb` 内部的 input/include 不会展开。

### 3.3 `%` 注释

支持：

```latex
正文 % 注释会被删除
100\%
```

规则：

1. 行首或只有空白前缀的 `%` 视为整行注释，整行删除且不留下空行。
2. 行内 `%` 删除该行 `%` 后内容，但保留换行。
3. `\%` 保留为文本百分号。
4. `verbatim`、`lstlisting`、`minted` 和 `\verb` 内部的 `%` 不作为注释。

暂不处理：

```latex
\begin{comment}...\end{comment}
\iffalse ... \fi
```

## 4. 文档元信息

元信息只在顶层扫描。位于 `{...}` 分组或环境内部的命令不会被当作文档元信息。

### 4.1 `\title{...}`

```latex
\title{My Article}
```

标题内容按文本型参数解析，支持 inline 样式、字号声明、颜色、公式和引用。

如果出现多个 `\title`，后者覆盖前者，并记录 warning。

### 4.2 `\author[...]{...}{...}{...}`

项目自定义作者命令：

```latex
\author[Corresponding author]{Author Name}{inst1, inst2}{mail@example.com}
```

参数：

| 参数 | 说明 |
| --- | --- |
| `[...]` | 作者属性，可选，多个属性用逗号分隔。 |
| 第 1 个 `{}` | 作者名。 |
| 第 2 个 `{}` | 机构 key，多个 key 用逗号分隔。 |
| 第 3 个 `{}` | 邮箱，可为空。 |

作者名按文本型参数解析。渲染时每位作者居中显示，作者名在上，邮箱在下。

### 4.3 `\defInstitution{key}{text}`

项目自定义机构命令：

```latex
\defInstitution{inst1}{Department, University}
```

规则：

1. 机构按首次有效定义顺序编号。
2. 作者名后显示机构编号上标。
3. 重复 key 会记录 warning，后续定义忽略，首个定义生效。
4. 不同 key 但机构文本完全一致时，会合并为同一个机构编号。
5. 同内容不同 key 会作为 alias 指向首个机构，并记录具体 alias warning。
6. 机构文本按文本型参数解析。

## 5. 摘要和关键词

### 5.1 `abstract`

```latex
\begin{abstract}
This is the abstract.
\end{abstract}
```

摘要内部按普通 block 解析。渲染为带加粗 `Abstract` 标题的摘要文本框。

`abstract` 不再从原位置抽离为 metadata；如果正文中多次出现，会在对应位置多次渲染。

### 5.2 `keywords` / `IEEEkeywords`

```latex
\begin{IEEEkeywords}
keyword 1, keyword 2
\end{IEEEkeywords}

\begin{keywords}
keyword 1, keyword 2
\end{keywords}
```

`keywords` 与 `IEEEkeywords` 等价。渲染为灰色关键词行，前缀为加粗 `Keywords:`。

## 6. 章节结构

支持：

```latex
\section{...}
\subsection{...}
\subsubsection{...}
```

能力：

1. 自动生成章节编号。
2. 标题进入目录。
3. 支持标题中的 inline 命令、公式和引用。
4. 支持标题后紧跟 `\label{...}`。
5. 正文标题和目录标题使用悬垂缩进。

### 6.1 `\appendices`

```latex
\appendices
\section{Appendix Title}
```

`\appendices` 后的 `\section` 重新按 `A/B/C...` 编号，页面显示为 `App. A`、`App. B` 等。

## 7. 段落和列表

### 7.1 普通段落

空行分隔段落。段落内部按 inline 规则解析。

块级 `{...}` 会被视为局部作用域，不会把 `{` / `}` 渲染到 HTML 中。分组中的声明式样式只作用于组内。

### 7.2 `itemize`

```latex
\begin{itemize}
  \item First item
  \item Second item
\end{itemize}
```

渲染为无序列表。

### 7.3 `enumerate`

```latex
\begin{enumerate}
  \item First item
  \item Second item
\end{enumerate}
```

渲染为有序列表。

### 7.4 `description`

```latex
\begin{description}
  \item[Term] Description text.
\end{description}
```

渲染为 `<dl>` / `<dt>` / `<dd>`。`\item[...]` 的 label 按 inline 规则解析。

### 7.5 嵌套列表

`itemize`、`enumerate`、`description` 支持多层嵌套。parser 只会把当前列表环境顶层的 `\item` 作为本层列表项，嵌套列表中的 `\item` 保留在子列表内部。

列表项内部可以包含段落、公式、图片、`tcb`、代码块和其他已支持 block。

## 8. 原样文本和代码块

### 8.1 `verbatim`

```latex
\begin{verbatim}
100% remains
\input{not-expanded}
\end{verbatim}
```

内部内容原样保留，不解析 inline 命令，不处理 `%` 注释，不展开 input/include。

### 8.2 `lstlisting`

```latex
\begin{lstlisting}[language=Go]
fmt.Println("hello")
\end{lstlisting}
```

渲染为带语法高亮的代码框。

可选参数使用 `[...]` 传入。当前只读取 `language`，其他参数会被忽略：

```latex
\begin{lstlisting}[frame=single,language=Python,numbers=left]
print("hello")
\end{lstlisting}
```

`language` 大小写不敏感，并会用于标题栏显示和语法高亮。未提供 `language` 时按普通文本代码块处理。

### 8.3 `minted`

```latex
\begin{minted}[]{go}
fmt.Println("hello")
\end{minted}
```

渲染为带语法高亮的代码框。

`minted` 的语言类型来自必选参数 `{...}`。`[...]` 中的所有可选参数都会被忽略：

```latex
\begin{minted}[linenos,breaklines]{python}
print("hello")
\end{minted}
```

语言名称大小写不敏感，并会用于标题栏显示和语法高亮。未能识别语言时，会降级为普通文本高亮。

### 8.4 代码框交互和样式

`verbatim`、`lstlisting` 和 `minted` 都会渲染为统一的代码框。代码框包含：

1. 左侧贯穿标题栏和正文区域的竖线。
2. 标题栏左侧的语言名称，使用黑体显示。
3. 标题栏右侧的自动换行、复制、折叠按钮。
4. 正文区域的行号和代码内容。

代码框使用 Source Code Pro 作为首选等宽字体。构建站点时会输出 Chroma 语法高亮样式文件 `static/chroma-theme.css`；`site init` 会同时生成该样式文件，并下载 Source Code Pro 字体文件到 `web/static/fonts/`。

自动换行默认关闭。关闭时，长行通过横向滚动查看；开启后，同一源码行仍保持一个行号，过长内容在视觉上换到下一行并带有缩进。复制按钮复制的是原始代码文本，而不是页面中带行号或高亮标签的可见文本。

### 8.5 `\verb`

`\verb` 内容作为原样 inline 片段保护，不会参与注释和 input/include 解析。

## 9. 可折叠文本框 `tcb`

项目自定义 `tcb` 环境：

```latex
\begin{tcb}[gray!70][black][gray!20]{测试 TCB 环境}
  tcb_test。测试！！
\end{tcb}
```

渲染为默认展开的可折叠文本框，包含：

1. 标题栏。
2. 标题栏右侧的展开/折叠箭头。
3. 标题栏下方紧贴的正文区域。
4. 左侧贯穿标题栏和正文区域的竖线。

三个可选参数按顺序表示：

| 参数 | 说明 |
| --- | --- |
| 第 1 个 `[...]` | 标题栏背景色。 |
| 第 2 个 `[...]` | 左侧竖线颜色。 |
| 第 3 个 `[...]` | 正文区域背景色。 |

默认规则：

1. 未提供标题栏背景色时，默认 `gray!70`。
2. 可选参数不足时，按标题栏、竖线、正文区域顺序依次应用。
3. 未提供正文区域背景色时，根据标题栏背景色调浅 80%。
4. 未提供竖线颜色时，根据标题栏背景色提高约 20% 对比度和 20% 饱和度。

标题 `{...}` 是文本型参数，支持字号、居中、颜色、粗体、公式和引用：

```latex
\begin{tcb}[#2BB7B3][#003e42][]{\Huge\centering 测试 TCB 环境}
...
\end{tcb}
```

标题默认使用黑体/无衬线加粗样式，默认字体颜色与竖线颜色一致。箭头按钮无背景、无边框，颜色同样与竖线颜色一致。

正文继续按普通 block 规则解析。

## 10. 图片和 Figure

### 10.1 `figure` / `figure*`

```latex
\begin{figure}
\centering
\includegraphics[width=0.8\textwidth]{fig/example.pdf}
\caption{Example figure}
\label{fig:example}
\end{figure}
```

支持：

1. 自动编号。
2. `\caption{...}`。
3. `\label{...}`。
4. 多个 `\includegraphics`。
5. `\subfloat` 子图。
6. 常见 layout wrapper，例如 `resizebox`、`scalebox`、`rotatebox`、`adjustbox`、`makebox`、`fbox`。

### 10.2 `\includegraphics[...]{...}`

支持读取图片路径并复制到输出目录。

支持 `width` 选项：

```latex
\includegraphics[width=0.8\textwidth]{fig/a.pdf}
\includegraphics[width=95%]{fig/b.png}
```

如果只写 `height`，当前会忽略高度，采用默认响应式宽度。

### 10.3 PDF 图片

`.pdf` 图片会尝试转换为 `.svg`。工具按顺序尝试：

1. `pdftocairo`
2. `mutool`
3. `inkscape`

如果三个工具都不可用，记录 warning。

### 10.4 `\subfloat`

```latex
\subfloat[Caption]{\includegraphics{fig/a.pdf}\label{fig:a}}
```

支持：

1. 子图编号 `(a)`、`(b)`、...、`(aa)`、`(ab)`。
2. 引用编号为父图编号加子图字母，例如 `1.a`。
3. 子 caption 居中显示。
4. 子图内部 `\label` 可被 `\ref` 引用。
5. `\subfloat` 后跟 `\\` 时作为换行处理。

## 11. 表格

### 11.1 `table` / `table*`

`table` 和 `table*` 由 Go parser 解析外壳：

```latex
\begin{table}
\caption{Result}
\label{tab:result}
\begin{tabular}{cc}
...
\end{tabular}
\end{table}
```

Go parser 负责：

1. table 编号。
2. table caption。
3. table label。
4. `\subfloat` 子表。
5. 子 caption 和子 label。
6. 外层 layout wrapper。

### 11.2 `tabular` / `tabularx`

`tabular` 和 `tabularx` 主体由 LaTeXML 托管。

支持范围取决于 LaTeXML 和 wrapper 中加载的包，当前包括：

1. `array`
2. `booktabs`
3. `tabularx`
4. `multirow`
5. `makecell`
6. `xcolor`
7. `amssymb`
8. `amsmath`

### 11.3 子表

table 内的 `\subfloat` 支持子表：

```latex
\subfloat[Sub table A]{%
  \begin{tabular}{cc}
  ...
  \end{tabular}
  \label{tab:a}
}
```

子表编号、caption、label 规则与子图一致。

## 12. 算法

### 12.1 `algorithm` / `algorithm*`

算法环境由 LaTeXML 托管：

```latex
\begin{algorithm}
\caption{Algorithm name}
...
\end{algorithm}
```

当前 wrapper 使用：

```latex
\usepackage[ruled,linesnumbered]{algorithm2e}
```

渲染层支持：

1. ruled 上下横线。
2. 行号栏。
3. 行号栏右侧竖线。
4. 嵌套控制结构竖线。
5. 长行自动换行。
6. input/output 悬垂缩进。
7. algorithm2e 关键字加粗。
8. 空白伪代码行忽略。

## 13. 数学公式

MetaBlog 负责公式边界识别、编号和交叉引用；公式内容由 KaTeX 渲染。

### 13.1 行内公式

```latex
$a+b$
\(a+b\)
```

### 13.2 块级公式

```latex
\[
a+b
\]

\begin{equation}
a+b
\end{equation}

\begin{align}
a &= b + c
\end{align}
```

支持：

1. `equation` / `equation*`
2. `align` / `align*`
3. `\label{...}`。
4. `\ref{...}`。

带星号环境不编号。

### 13.3 文本中的数学字体命令

以下命令如果出现在普通文本中，会被识别为 inline math：

```latex
\boldsymbol{...}
\mathbf{...}
\mathit{...}
\mathrm{...}
\mathbb{...}
\mathcal{...}
\mathsf{...}
\mathtt{...}
\mathfrak{...}
\operatorname{...}
```

支持紧随其后的上下标：

```latex
\boldsymbol{x}_i
\mathbf{A}^{-1}
```

### 13.4 LaTeXML 数学输出

LaTeXML 复杂块中的 MathML 会根据 `alttext` 还原为 KaTeX 公式。对于 LaTeXML 在表格内 inline `aligned` 公式中丢失首行的问题，MetaBlog 会尝试用原始 LaTeX 片段修复。

## 14. 交叉引用和参考文献

### 14.1 `\label{...}`

支持在章节、公式、figure、table、子图、子表和复杂块中建立 label。

### 14.2 `\ref{...}`

```latex
See Fig.~\ref{fig:example}
```

渲染为带链接的编号。找不到 label 时记录 warning。

### 14.3 `\cite{...}`

```latex
\cite{key1,key2}
```

按引用出现顺序生成 IEEE 风格编号，例如 `[1]`、`[1], [2]`。连续引用范围会按 IEEE 风格压缩。

### 14.4 `\bibliographystyle{...}`

识别并忽略。最终输出使用项目内置的简化 IEEE 风格。

### 14.5 `\bibliography{...}`

```latex
\bibliography{refs}
```

规则：

1. 没有扩展名时自动补 `.bib`。
2. 多个 bib 文件可用逗号分隔。
3. References 输出在 `\bibliography` 所在位置。
4. 找不到 bib 文件或 cite key 时记录 warning。

## 15. Inline 文本命令

### 15.1 字体样式

支持：

```latex
\textbf{...}
\textit{...}
\emph{...}
\textrm{...}
\underline{...}
```

### 15.2 声明式样式

支持组内或文本型参数开头的声明式样式：

```latex
{\color{blue} text}
{\bfseries text}
{\itshape text}
\section{\Huge\centering Important Title}
```

当前支持：

```latex
\color{...}
\pagecolor{...}
\backgroundcolor{...}
\bgcolor{...}
\bfseries
\bf
\itshape
\it
\em
\ttfamily
\tt
\normalfont
\upshape
\underline
\ul
```

### 15.3 字号命令

支持：

```latex
\tiny
\scriptsize
\footnotesize
\small
\normalsize
\large
\Large
\LARGE
\huge
\Huge
```

字号声明可用于文本型参数、段落和分组中。

### 15.4 对齐声明

支持：

```latex
\centering
\raggedright
\raggedleft
```

文档标题、章节标题、普通段落、figure/table caption、块级样式组和 `tcb` 标题会消费对齐结果。纯 inline 位置会忽略对齐结果。

### 15.5 颜色和背景色

支持：

```latex
\textcolor{blue}{text}
\colorbox{gray!70}{text}
{\color{blue} text}
```

颜色支持：

1. 常见命名色。
2. 十六进制颜色，例如 `#2BB7B3`。
3. 简单 `xcolor` 百分比混合，例如 `gray!70`。

### 15.6 文本型参数

以下位置按文本型参数解析，因此支持声明式样式：

1. `\title{...}`
2. 作者名和机构文本
3. 章节标题
4. figure/table caption
5. tcb 标题
6. `\href{url}{text}` 的显示文本
7. `description` 的 `\item[...]`
8. `\textbf{...}`、`\textit{...}`、`\emph{...}`、`\textcolor{...}{...}` 等文本参数

文本型参数开头连续出现的声明式命令会合并处理，顺序无关：

```latex
{\tiny\centering 标题}
{\centering\tiny 标题}
```

都会同时得到居中和 `\tiny` 字号。

### 15.7 非文本型参数

以下参数不按文本型参数解析：

1. `\label{...}`
2. `\ref{...}`
3. `\cite{...}`
4. `\href{url}{...}` 的 URL 参数
5. `\includegraphics{...}`
6. `\bibliography{...}`
7. 环境名

这些参数保持 label、引用 key、路径或配置值语义。

### 15.8 脚注

```latex
text\footnote{footnote content}
```

脚注渲染为上标 comment 图标，鼠标悬停显示浮窗。

### 15.9 链接

```latex
\url{https://example.com/a\_b?x=1\&y=2}
\href{https://example.com}{\small Example}
```

规则：

1. `\url{...}` 同时作为链接目标和显示文本。
2. `\href{...}{...}` 第一个参数作为 URL，第二个参数按文本型参数解析。
3. URL 参数会清理 `\_`、`\&`、`\%`、`\#`、`\~` 等常见转义。
4. HTML 渲染允许 `http://`、`https://`、`mailto:`、`ftp://`、锚点、站内绝对路径和相对路径。
5. 其他带 scheme 的链接降级为 `#`，避免危险协议。

### 15.10 特殊符号和空白

支持：

```latex
~
\, \; \:
\%
\&
\_
```

`~` 渲染为不换行空格。

### 15.11 引号、连字符和 `\IEEEPARstart`

支持基础替换：

```latex
``text''
--
```

分别渲染为英文双引号和短横线。

`\IEEEPARstart{T}{his}` 会把两个参数直接拼接为普通文本。

## 16. LaTeXML 缓存

LaTeXML 复杂块缓存位于：

```text
.metablog-cache/latexml/
```

清理缓存：

```bash
metablog cache clean -root .
```

临时忽略缓存：

```bash
metablog site build -no-latexml-cache
```

缓存 key 包含：

1. RawTeX SHA-256。
2. 完整 RawTeX。
3. LaTeXML wrapper SHA-256。
4. 调用参数 SHA-256。
5. `latexmlc` 解析路径。
6. `latexmlc` 版本输出。
7. 缓存 schema。

命中缓存时，不仅校验 hash，也会把缓存中的完整 RawTeX 与当前 RawTeX 做逐字节匹配。

缓存只保存 LaTeXML 原始 HTML。即使命中缓存，MetaBlog 仍会重新执行：

1. body fragment 提取。
2. style/script 删除。
3. MathML 到 KaTeX 还原。
4. 颜色白名单过滤。
5. algorithm2e 行标注。
6. inline `aligned` 公式修复。
7. 最终容器包裹。

缓存绕过条件：

1. 启用 `-keep-temp`。
2. RawTeX 包含 `\input`。
3. RawTeX 包含 `\include`。
4. RawTeX 包含 `\includegraphics`。
5. RawTeX 包含 `\bibliography`。
6. RawTeX 包含 `\addbibresource`。
7. 启用 `-no-latexml-cache`。

## 17. LaTeXML 输出清洗

LaTeXML 输出嵌入最终页面前会清洗：

1. 删除 LaTeXML 自动生成的 `id` 属性。
2. 删除 `<style>` 和 `<script>` 块。
3. 删除不在白名单内的 inline `style`。
4. 保留安全颜色样式。
5. 清理后包裹为统一 fragment 容器。

允许保留的颜色样式：

1. `color`
2. `background-color`

允许的颜色值：

1. 十六进制颜色。
2. 命名颜色。
3. `rgb(...)` / `rgba(...)`。

布局类样式如 `position`、`left`、`width`、`height`、`margin`、`font-size` 会删除，避免破坏 MetaBlog 的统一响应式布局。

## 18. 输出和资源能力

文档图片资源会复制或转换到 `out/assets/` 下。

整站模式：

| 来源 | 输出 |
| --- | --- |
| 文章资源 | `out/assets/articles/<slug>/...` |
| 关于页面资源 | `out/assets/about/...` |
| 站点 logo/icon | `out/assets/site/...` |

单篇模式：

```text
out/assets/...
```

资源 fresh 判断：

1. 输出文件存在。
2. 输出文件非空。
3. 输出文件修改时间不早于源文件。

满足时跳过复制或 PDF 转 SVG。

## 19. 未完整支持和降级策略

### 19.1 未知 inline 命令

未知 inline 命令会记录 warning。

降级规则：

1. 如果后面有 `{...}`，通常丢弃命令名，保留参数内容。
2. `\LaTeX` 渲染为 `LaTeX`。
3. 其他无法理解的命令可能被丢弃。

示例：

```latex
\unknown{text}
```

通常降级为：

```text
text
```

### 19.2 未知环境

未知环境会记录 warning，并作为透明 block 保留内部可解析内容。也就是说，环境本身不产生特殊 HTML 外壳，但内部段落、章节、列表等仍会继续解析。

### 19.3 条件编译

暂不支持：

```latex
\iffalse ... \fi
```

如果需要忽略内容，建议直接使用 `%` 注释，或在输入前删除对应片段。

### 19.4 宏定义和宏展开

暂不支持完整宏展开：

```latex
\newcommand{...}{...}
\renewcommand{...}{...}
\def\foo{...}
```

原因是完整宏展开涉及 TeX 参数扫描、作用域、条件、计数器和包语义，超出当前轻量解析目标。

### 19.5 复杂 LaTeX 布局

MetaBlog 不追求复刻 PDF 排版。以下内容可能只能部分支持或降级：

1. 复杂浮动体布局。
2. 多栏排版。
3. 自定义计数器。
4. TikZ / PGF 绘图。
5. 需要宏展开才能得到正文结构的自定义命令。

## 20. Warning 输出

可能产生 warning 的情况：

1. input/include 循环或重复。
2. 未知 inline 命令。
3. 未知环境。
4. 重复机构 key。
5. 同机构文本不同 key alias。
6. 找不到 label。
7. 找不到 citation key。
8. 找不到 bib 文件。
9. 资源文件缺失。
10. PDF 转 SVG 失败。
11. LaTeXML 转换失败并使用 fallback。

构建日志会列出 warning 明细。启用 `-strict` 时，只要存在 warning，构建最终失败。

## 21. 推荐写法

推荐：

1. 文章主文件使用 `main.tex`。
2. 分章节使用 `\input{section/name}` 或 `\include{section/name}`。
3. 图片放在文章目录下的 `fig/`。
4. 表格 LaTeX 片段放在文章目录下的 `table/`。
5. 参考文献 `.bib` 放在文章目录或其子目录。
6. 尽量使用标准 `figure` / `table` / `caption` / `label` 结构。
7. 对复杂表格使用 `tabular` 或 `tabularx`，交给 LaTeXML 处理。
8. 避免依赖自定义宏生成核心结构。

一个典型文章结构：

```text
articles/my-paper/
  main.tex
  section/
    introduction.tex
    experiments.tex
  fig/
    overview.pdf
  table/
    result.tex
  refs.bib
```
