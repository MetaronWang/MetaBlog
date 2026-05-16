# MetaBlog

MetaBlog 是一个用 Go 编写的个人博客脚手架。它的核心目标是：用 LaTeX 编写文章和关于页面，用一个 `metablog` 二进制命令把它们构建成可直接部署的静态网站。

这个项目面向的场景不是通用 CMS，而是个人学术博客、论文笔记、技术文章和包含公式、图表、参考文献的长文档。MetaBlog 尽量把常见 LaTeX 写作习惯保留下来，同时避免完整 TeX 引擎的复杂度：简单结构由 Go 内置解析器处理，复杂表格和算法块交给 LaTeXML 处理。

## 特性

- 单个二进制 CLI：网站初始化、整站构建、单篇构建、文章元数据维护和缓存清理都通过 `metablog` 完成。
- 从空白目录初始化网站：`metablog site init` 不依赖源码仓库资源，默认模板、logo、icon 和字体 CSS 都编译进二进制。
- LaTeX 到 HTML：支持章节、公式、引用、图片、列表、表格、算法、摘要、关键词、参考文献等常见论文结构。
- 静态站点输出：生成首页、所有文章页、标签页、分类页、关于页和文章页。
- 增量构建：文档源目录未变化时跳过重新编译；资源复制和 PDF 转 SVG 也有逐文件 fresh 检测。
- LaTeXML 缓存：复杂块转换结果缓存在 `.metablog-cache/latexml/`，缓存命中时不重复调用 LaTeXML。
- 字体支持：默认字体可下载到网站目录，也支持构建时按生成 HTML 内容做字体子集化。
- 文件监听预览：`metablog site serve -watch` 启动本地预览服务器时自动监听源文件变化，增量编译变更的文档，并在当前预览页面更新后自动刷新浏览器。

## 快速开始

先准备一个 `metablog` 二进制文件，然后在任意空目录初始化网站：

```bash
metablog site init -root my-blog -title "My Blog"
cd my-blog
metablog site build -root . -out out
```

生成的网站位于 `out/`，可以部署到任意静态文件服务器。

`site init` 默认会下载字体文件，并在最后检测 Python、LaTeXML 和 PDF 转换器环境。它只检测并提示安装方式，不会自动安装或修改系统环境。

如果只想先生成目录和配置，可以跳过字体下载和环境检测：

```bash
metablog site init -root my-blog -title "My Blog" -skip-fonts -skip-env-check
```

## 外部依赖

基础构建只需要 `metablog` 二进制。根据文档内容和构建选项，可能需要以下外部工具：

| 依赖 | 用途 | 是否必须 |
| --- | --- | --- |
| `latexmlc` | 转换 `tabular`、`tabularx`、`algorithm` 等复杂块。 | 含复杂块时需要。 |
| `pdftocairo` / `mutool` / `inkscape` | 将 PDF 图片转为 SVG。 | 含 PDF 图片时需要任意一个。 |
| `pyftsubset` | 生成字体子集。 | 使用 `-subset-fonts` 时需要。 |
| Python `fontTools` / `brotli` | 支持字体子集化。 | 使用 `-subset-fonts` 时需要。 |

`metablog site init` 会检测这些环境，并在不满足时给出安装建议，但不会自动安装。

### Windows 依赖安装

LaTeXML 推荐通过 Strawberry Perl 安装：

```powershell
# 1. 先安装 Strawberry Perl，并确保 perl/cpan 在 PATH 中
# 2. 然后安装 LaTeXML
cpan LaTeXML
```

如果安装了 `cpanm`，也可以使用：

```powershell
cpanm LaTeXML
```

PDF 转 SVG 推荐安装 Poppler for Windows，并把 Poppler 的 `bin` 目录加入 PATH。也可以安装 MuPDF 或 Inkscape，MetaBlog 会按 `pdftocairo`、`mutool`、`inkscape` 的顺序查找。

字体子集化需要 Python 和 fonttools/brotli：

```powershell
python -m pip install fonttools brotli
```

如果使用 Conda，请先激活对应环境：

```powershell
conda activate base
python -m pip install fonttools brotli
```

### macOS 依赖安装

使用 Homebrew：

```bash
brew install latexml
brew install poppler
brew install python
python3 -m pip install fonttools brotli
```

如果不使用 Poppler，也可以安装 MuPDF 或 Inkscape：

```bash
brew install mupdf
brew install --cask inkscape
```

### Debian/Ubuntu 依赖安装

```bash
sudo apt update
sudo apt install latexml poppler-utils python3 python3-pip
python3 -m pip install fonttools brotli
```

如果需要备用 PDF 转换器：

```bash
sudo apt install mupdf-tools inkscape
```

### 检查环境

初始化网站时会自动检测：

```bash
metablog site init -root my-blog
```

也可以手动检查这些命令是否可用：

```bash
latexmlc --version
pdftocairo -v
pyftsubset --help
python -c "import fontTools, brotli"
```

如果文档不包含复杂表格/算法、不包含 PDF 图片，也不使用 `-subset-fonts`，这些外部依赖可以暂时不安装。

## CLI 用法

MetaBlog 只提供一个命令入口：`metablog`。

```text
metablog site init
metablog site build
metablog site serve
metablog article build
metablog article init
metablog article edit
metablog article delete
metablog cache clean
```

旧入口仍保留兼容：

```bash
metablog -site -root .
metablog -input articles/example/main.tex -out out/example
```

### 初始化网站

```bash
metablog site init -root my-blog -title "My Blog"
```

常用参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-root` | `.` | 要初始化的网站根目录。 |
| `-title` | `MetaBlog` | 写入 `data/config.toml` 的网站标题。 |
| `-latexml-bin` | 空 | 环境检测时使用的 `latexmlc` 路径。 |
| `-skip-fonts` | `false` | 跳过字体下载。 |
| `-skip-env-check` | `false` | 跳过 Python、LaTeXML 和 PDF 转换器检测。 |

`site init` 不覆盖已有文件，重复运行只会补齐缺失内容。

### 构建网站

```bash
metablog site build -root . -out out
```

常用参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-root` | `.` | 网站根目录。 |
| `-out` | `out` | 静态网站输出目录。 |
| `-config` | `data/config.toml` | 网站配置文件。 |
| `-articles` | `data/articles.toml` | 文章元数据文件。 |
| `-force` | `false` | 强制重新编译 about 和所有文章。 |
| `-no-assets` | `false` | 跳过资源复制和 PDF 转 SVG。 |
| `-no-latexml-cache` | `false` | 忽略 LaTeXML 缓存。 |
| `-subset-fonts` | `false` | 构建后生成字体子集。 |
| `-article-workers` | `0` | 并行构建文章数；0 表示自动选择。 |
| `-latexml-workers` | `0` | 并行转换复杂块数；0 表示自动选择。 |
| `-latexml-bin` | 空 | 指定 `latexmlc` 路径。 |
| `-strict` | `false` | 出现解析 warning 时构建失败。 |

### 本地预览网站

```bash
metablog site serve -out out
```

默认监听 `127.0.0.1`，端口默认为 `0`，表示由系统随机分配空闲端口。启动后 CLI 会输出实际访问 URL。

也可以指定地址和端口：

```bash
metablog site serve -out out --host 0.0.0.0 --port 8080
```

命令会阻塞运行，直到按 `Ctrl+C` 停止。

#### 文件监听和热重编译（Watch 模式）

启用 `-watch` 可在预览时自动监听源文件变化并增量编译：

```bash
metablog site serve -out out -watch -root .
```

修改文章源文件、关于页面或配置文件后，服务器会自动重编译受影响的 HTML。Watch 模式会对 HTML 响应注入本地自动刷新脚本，浏览器会轮询当前页面版本；如果当前正在预览的页面发生更新，会自动刷新，无关页面更新不会打断当前预览。完整参数见 [CLI 使用文档](docs/cli-usage.md#文件监听和热重编译watch-模式)。

#### 纯内存模式（Only-RAM）

启用 `-only-ram` 将预览服务和后续热更新移入内存，避免频繁写入硬盘：

```bash
metablog site serve -out out -watch -root . -only-ram
```

页面和资源更新仅写入内存映射，LaTeXML 缓存仍落盘但自动启用有界内存读缓存。若同时启用 `-initial-build`，启动前的全量构建仍会先写入 `out/` 一次，随后再加载到内存；HTTP 服务和 watch 后续更新仍从内存读写。详见 [CLI 使用文档](docs/cli-usage.md#纯内存模式only-ram)。

### 构建单篇 LaTeX 文档

```bash
metablog article build -input articles/example/main.tex -out out/example
```

这个命令适合调试单篇文章。输出目录中会生成 `index.html`。

常用参数：

| 参数 | 默认值 | 说明 |
| --- | --- | --- |
| `-input` | `sample_latex/DACE-with_supplementary.tex` | 主 LaTeX 文件。 |
| `-out` | `out` | 输出目录。 |
| `-root` | `.` | 项目根目录，用于定位缓存。 |
| `-dump-ast` | `false` | 写出调试 AST 到 `out/debug/ast.json`。 |
| `-no-assets` | `false` | 跳过资源处理。 |
| `-no-latexml-cache` | `false` | 忽略 LaTeXML 缓存。 |

### 维护文章

```bash
metablog article init -root .
metablog article edit -root .
metablog article delete -root .
```

`article init` 会交互式创建文章目录、主 `main.tex` 文件，并写入 `data/articles.toml`。`description` 支持多行输入，连续两个换行结束。

### 清理缓存

```bash
metablog cache clean -root .
```

该命令删除 `.metablog-cache/`。也可以手动删除这个目录。

## 网站目录结构

一个典型网站目录如下：

```text
my-blog/
  articles/
    my-first-article/
      main.tex
      fig/
      table/
      section/
  asset/
    figs/
      circle_example.svg
  data/
    config.toml
    articles.toml
    about_page/
      main.tex
  web/
    static/
      fonts.css
      fonts/
  .metablog-cache/
  out/
```

主要目录含义：

| 路径 | 说明 |
| --- | --- |
| `articles/` | 文章源文件目录，每篇文章一个子目录。 |
| `data/config.toml` | 网站级配置。 |
| `data/articles.toml` | 文章元数据列表。 |
| `data/about_page/main.tex` | 关于页面的 LaTeX 主文件。 |
| `data/custom_components/` | 自定义页面组件，目前包含页尾和文章统计片段。 |
| `asset/` | 网站级资源，例如 logo 和 icon。 |
| `web/static/` | 网站静态资源和字体。 |
| `.metablog-cache/` | 本地构建缓存，不应提交。 |
| `out/` | 构建输出目录，不应提交。 |

## 网站配置

`data/config.toml` 示例：

```toml
title = "My Blog"
logo = "figs/circle_example.svg"
icon = "figs/circle_example.svg"
home_page_size = 10
article_list_page_size = 20
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `title` | 网站标题，显示在顶栏和页面标题中。 |
| `logo` | 相对于 `asset/` 的 logo 路径。 |
| `icon` | 相对于 `asset/` 的浏览器 icon 路径。 |
| `home_page_size` | 首页每页展示文章数量。 |
| `article_list_page_size` | 所有文章、分类、标签列表每页展示文章数量。 |

## 文章配置

`data/articles.toml` 示例：

```toml
[[articles]]
title = "My First Article"
description = "文章简介。首页和文章列表会展示该字段，超过 1000 个字符会省略。"
author = "Author Name"
date = "2026-05-11"
category = ["Notes", "LaTeX"]
tags = ["LaTeX", "Blog"]
folder = "articles/my-first-article"
main_fig = "fig/main.pdf"
main_file = "main.tex"
deleted = false
```

字段说明：

| 字段 | 说明 |
| --- | --- |
| `title` | 文章标题。 |
| `description` | 文章摘要，用于首页和文章列表。 |
| `author` | 文章作者。 |
| `date` | 发表日期，用于排序。 |
| `category` | 多级分类，按数组顺序形成分类路径。 |
| `tags` | 标签列表。 |
| `folder` | 文章目录，相对于网站根目录。 |
| `main_fig` | 文章主图，相对于文章目录。PDF 会映射为 SVG 输出。 |
| `main_file` | 文章 LaTeX 主文件，相对于文章目录。 |
| `deleted` | 软删除标记；为 `true` 时不参与构建和列表展示。 |

## 页面生成

整站构建会生成：

| 页面 | 输出 |
| --- | --- |
| 首页 | `out/index.html`，后续分页为 `out/page/<n>/index.html`。 |
| 所有文章 | `out/articles/index.html`，后续分页为 `out/articles/page/<n>/index.html`。 |
| 标签索引 | `out/tags/index.html`。 |
| 标签文章列表 | `out/tags/<tag>/index.html`。 |
| 分类索引 | `out/categories/index.html`。 |
| 分类文章列表 | `out/categories/<path>/index.html`。 |
| 关于页面 | `out/about/index.html`。 |
| 文章页面 | `out/articles/<slug>/index.html`。 |

关于页面来自 `data/about_page/main.tex`。文章页面来自 `data/articles.toml` 中每篇文章的 `folder` 和 `main_file`。

## LaTeX 支持范围

MetaBlog 不是完整 TeX 引擎。它采用两层策略：

1. 常见结构由 Go 解析器直接解析成 AST，再渲染为 HTML。
2. 复杂块交给 LaTeXML 转换，再嵌入页面。

当前只解析 `\begin{document}` 和 `\end{document}` 之间的内容；导言区不会作为正文渲染。

### 源文件处理

支持：

- `%` 行注释，支持 `\%` 转义。
- 递归展开 `\input{...}` 和 `\include{...}`，当前两者等价。
- 自动为无扩展名 input/include 补 `.tex`。
- 检测循环 input/include 并记录 warning。
- `verbatim`、`lstlisting`、`minted`、`html` 和 `\verb` 中的内容不会被当作注释或 input/include 解析。
- `\importHTML{relative/path.html}` 会读取相对于主文件的 HTML 片段，并作为原始 HTML 嵌入页面。

### 文档元信息

支持：

- `\title{...}`
- `\author[...]{name}{institutions}{email}`
- `\defInstitution{key}{institution}`
- `abstract` 环境
- `keywords` / `IEEEkeywords` 环境

机构会按首次定义编号；重复 key 会 warning；同内容不同 key 会合并为 alias 并 warning。

### 章节、引用和参考文献

支持：

- `\section`
- `\subsection`
- `\subsubsection`
- `\appendices`
- `\label`
- `\ref`
- `\cite`
- `\bibliographystyle`
- `\bibliography`

引用按 IEEE 风格编号。找不到 label 或 cite key 时会记录 warning。

### 公式

支持：

- 行内公式：`$...$`、`\(...\)`
- 块级公式：`\[...\]`、`$$...$$`
- `equation` / `equation*`
- `align` / `align*`
- 常见内部数学环境：`aligned`、`alignedat`、`gathered`、`split`、矩阵环境等

公式渲染交给 KaTeX。MetaBlog 负责公式块识别、编号和交叉引用。`\[...\]`、`$$...$$`、`equation*`、`align*` 和内部数学环境 fallback 不编号；`equation` / `align` 默认编号。

### 图片和表格

支持：

- `figure` / `figure*`
- `\includegraphics`
- `\caption`
- `\label`
- `\subfloat`
- `table` / `table*`
- `tabular` / `tabularx`

`figure` 和 `table` 会由 Go 解析器解析 caption、label、subfloat 等结构。真正复杂的 `tabular` / `tabularx` 内容交给 LaTeXML。子图/子表支持 `(a)`、`(b)`、`(aa)` 编号，引用格式为 `1.a`。

图片资源会输出到 `out/assets/`。PDF 图片会优先使用 `pdftocairo` 转为 SVG，找不到时依次尝试 `mutool` 和 `inkscape`。

### 列表

支持：

- `itemize`
- `enumerate`
- `description`
- 多层嵌套列表
- `description` 的可选 label

### 文本样式和链接

支持常见 inline 命令，包括：

- 字体样式：`\textbf`、`\textit`、`\emph`、`\texttt`、`\textrm`、`\textsf`、`\textsc`、`\textsl`、`\textup`、`\textmd`、`\textnormal`、`\underline` 等。
- 声明式样式：`\tiny`、`\scriptsize`、`\footnotesize`、`\small`、`\normalsize`、`\large`、`\Large`、`\LARGE`、`\huge`、`\Huge`。
- 字体族和字形声明：`\ttfamily`、`\rmfamily`、`\sffamily`、`\scshape`、`\slshape`、`\mdseries`、`\normalfont`、`\upshape` 等。
- 对齐声明：`\centering` 等。
- 颜色：`\color{...}`、`\textcolor{...}{...}`。
- 链接：`\url{...}`、`\href{url}{text}`。

文本型参数会走统一的声明式样式解析入口，因此章节标题、caption、tcb 标题等位置都支持字号、居中、颜色等声明。

### 特殊环境

支持：

- `tcb`：可折叠标题文本框。
- `verbatim`
- `lstlisting`
- `minted`
- `html`：原始 HTML 片段，直接写入最终页面，不做 escape 或安全清洗。
- `\importHTML{...}`：导入相对于主文件的外部 HTML 片段，并按原始 HTML 输出。

`lstlisting` 和 `minted` 会渲染为带标题栏、行号、复制、折叠、自动换行切换和 Chroma 语法高亮的代码框。`html` 环境和 `\importHTML` 只适合可信内容；`\importHTML` 会检查目标文件是否存在，并在内容不像 HTML 时记录 warning。

### 自定义组件

`data/custom_components/` 下的组件会在整站构建时自动加载：

- `page_footing.tex`：注入所有页面底部。默认包含卜算子站点总访问量和访客数统计。
- `article_stat.tex`：注入每篇文章标题区的作者信息下方。默认包含卜算子当前页面阅读量统计。

这两个文件按普通 LaTeX 片段解析，通常使用 `html` 环境写入可信 HTML。卜算子当前常用接口只提供站点 PV、站点 UV 和页面 PV，因此默认文章统计展示的是本文阅读量；站点访客数放在全站页尾中展示。

未知 inline 命令和未知环境会记录 warning。未知环境会作为透明块保留内部可解析内容。

更完整的支持列表见 [docs/latex-command-support.md](docs/latex-command-support.md)。

## 缓存和增量构建

文档级增量构建规则：

- 如果文档源目录中所有文件的最新修改时间不晚于该文档 HTML 输出文件，则跳过该文档编译。
- 使用 `-force` 可以强制重新编译 about 和所有文章。

资源级增量处理规则：

- 资源复制和 PDF 转 SVG 会逐文件比较源文件和输出文件修改时间。
- 输出资源已存在、非空且不旧于源文件时，跳过复制或转换。

LaTeXML 缓存：

- 缓存目录为 `.metablog-cache/latexml/`。
- 缓存 key 包含 RawTeX、LaTeXML wrapper、调用参数、`latexmlc` 路径和版本。
- 缓存文件保存完整 RawTeX，读取时会做完全匹配，正确性优先。
- 使用 `-no-latexml-cache` 可以临时忽略缓存。
- 使用 `metablog cache clean -root .` 可以清理缓存。

## 文档

- [CLI 使用文档](docs/cli-usage.md)
- [LaTeX 命令支持范围](docs/latex-command-support.md)
- [LaTeX 到 HTML 架构设计](docs/latex-to-html-architecture.md)
- [网站结构定义](docs/网站结构定义.md)

## 开发

运行测试：

```bash
go test ./...
```

构建二进制：

```bash
go build -o metablog ./cmd/metablog
```

如果当前 Git 环境无法读取 VCS 状态，可以关闭 VCS stamping：

```bash
go build -buildvcs=false -o metablog ./cmd/metablog
```
