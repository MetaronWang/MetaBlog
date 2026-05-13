# MetaBlog 架构设计文档

本文档说明 MetaBlog 的整体架构、构建流程、LaTeX 解析流程、网站生成方式、缓存策略和 Go 代码组织。目标是让后续开发者先理解系统分层和数据流，再进入具体文件和函数。

更偏用户使用的内容见：

- [README.md](../README.md)
- [CLI 使用文档](cli-usage.md)
- [网站结构定义](网站结构定义.md)
- [LaTeX 命令支持范围](latex-command-support.md)

## 1. 项目目标

MetaBlog 是一个用 Go 编写的静态博客脚手架。它希望让作者继续使用 LaTeX 编写学术风格文章，同时生成适合个人博客发布的 HTML 静态站点。

核心目标：

1. 用一个 `metablog` 二进制完成网站初始化、整站构建、单篇构建、文章管理和缓存清理。
2. 以 LaTeX 作为文章和关于页面的源格式。
3. 支持首页、所有文章、标签、分类、关于页面和文章页面。
4. 通过 `data/config.toml` 管理网站配置，通过 `data/articles.toml` 管理文章元数据。
5. 使用轻量 Go 解析器处理可控 LaTeX 结构。
6. 将 `tabular`、`tabularx`、`algorithm` 等复杂块托管给 LaTeXML。
7. 支持增量构建、资源复用和 LaTeXML 缓存，降低重复构建成本。

## 2. 设计边界

MetaBlog 不是完整 TeX 引擎，不执行完整宏展开，也不尝试复刻 TeX 的排版模型。

当前策略是：

| 层级 | 职责 |
| --- | --- |
| Go 解析器 | 处理文档结构、段落、章节、列表、图片、表格外壳、引用、基础 inline 样式。 |
| LaTeXML | 处理结构复杂且依赖 LaTeX 包语义的块，例如 `tabular`、`tabularx`、`algorithm`。 |
| KaTeX | 在浏览器端渲染数学公式。 |
| Chroma | 在 Go 渲染阶段为 `lstlisting` / `minted` 代码块生成语法高亮 HTML 和主题 CSS。 |
| HTML/CSS 渲染器 | 负责最终页面结构、编号、目录、交叉引用和样式。 |

该边界带来两个原则：

1. 能稳定手写解析的语法放在 Go parser 中，保证 HTML 结构可控。
2. 手写解析成本高、LaTeX 包语义重的结构交给 LaTeXML，并通过缓存减少开销。

## 3. 总体分层

项目按职责分为以下层：

```text
cmd/metablog
  -> internal/app
    -> internal/blog
    -> internal/latex/source
    -> internal/latex/blocks
    -> internal/latexml
    -> internal/latex/parser
    -> internal/assets
    -> internal/bib
    -> internal/highlight
    -> internal/render
    -> internal/site
```

核心数据流：

```text
LaTeX source files
  -> source.Load
  -> blocks.Lift
  -> latexml.Runner.Convert
  -> parser.Parse
  -> assets.Converter.Process
  -> bib.Load
  -> render.RenderWithOptions
  -> HTML files + static assets
```

整站构建在单篇构建外再包一层站点生成逻辑：

```text
data/config.toml + data/articles.toml
  -> blog.Load
  -> write index/tag/category/article-list pages
  -> build about page and article pages in parallel
  -> write static files and optional font subsets
```

## 4. 目录结构

源码仓库主要结构：

```text
cmd/metablog/                 CLI 入口
internal/app/                 应用编排层
internal/blog/                网站配置、文章元数据和站点页面生成
internal/latex/ast/           LaTeX 文档 AST
internal/latex/lexer/         LaTeX 词法切分
internal/latex/source/        源码加载、注释处理、input/include 展开
internal/latex/blocks/        复杂块抽离
internal/latex/parser/        block 和 inline parser
internal/latexml/             LaTeXML 调用、缓存和 HTML fragment 清洗
internal/render/              AST 到文章 HTML
internal/assets/              图片/附件复制，PDF 转 SVG
internal/bib/                 BibTeX 解析
internal/highlight/           代码语法高亮
internal/site/                静态资源、默认 CSS、字体子集化
docs/                         文档
```

网站目录主要结构：

```text
my-blog/
  articles/                   文章源文件目录
  asset/                      站点级资源
  data/config.toml            网站配置
  data/articles.toml          文章元数据
  data/about_page/main.tex    关于页面 LaTeX 主文件
  web/static/                 字体和静态资源
  .metablog-cache/            本地缓存
  out/                        默认构建输出
```

## 5. CLI 设计

MetaBlog 只提供一个 Go 二进制入口：`metablog`。

推荐命令结构：

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

```text
metablog -site -root .
metablog -input path/to/main.tex -out out
```

### 5.1 `site init`

`site init` 用于在只有一个 `metablog` 二进制、没有源码仓库资源的情况下初始化网站目录。

它会创建：

```text
articles/
asset/figs/
data/about_page/
web/static/fonts/
```

并写入：

```text
data/config.toml
data/articles.toml
data/about_page/main.tex
asset/figs/circle_example.svg
web/static/fonts.css
web/static/chroma-theme.css
.gitignore
```

默认模板、SVG、`fonts.css` 和 Chroma 主题 CSS 都编译进二进制。字体文件从网络下载到 `web/static/fonts/`，其中包含正文使用的中文字体和代码框使用的 Source Code Pro。Python、LaTeXML 和 PDF 转换器只做检测，失败时输出 `WARN` 和安装建议，不自动安装。

### 5.2 `site build`

整站构建读取：

```text
data/config.toml
data/articles.toml
data/about_page/main.tex
articles/<article-folder>/<main-file>
```

输出完整静态站点到 `out/`。

关键参数：

| 参数 | 说明 |
| --- | --- |
| `-root` | 网站根目录。 |
| `-out` | 输出目录。 |
| `-config` | 网站配置文件。 |
| `-articles` | 文章元数据文件。 |
| `-force` | 强制重新编译 about 和所有文章。 |
| `-no-assets` | 跳过资源处理。 |
| `-no-latexml-cache` | 忽略 LaTeXML 缓存。 |
| `-subset-fonts` | 根据生成 HTML 内容生成字体子集。 |
| `-article-workers` | 文章并发构建数。 |
| `-latexml-workers` | LaTeXML 复杂块并发转换数。 |

### 5.3 `site serve`

`site serve` 用于预览已经生成的静态网站目录：

```bash
metablog site serve -out out
```

它使用 Go 标准库 `net/http` 提供静态文件服务，不自动触发构建。默认监听 `127.0.0.1`，默认端口为 `0`，表示由系统分配随机空闲端口。启动后会输出实际访问 URL，并阻塞运行直到用户停止进程。

参数：

| 参数 | 说明 |
| --- | --- |
| `-out` | 要预览的静态网站输出目录，默认 `out`。 |
| `-host` | 监听地址，默认 `127.0.0.1`。 |
| `-port` | 监听端口，默认 `0`，表示随机空闲端口。 |
| `-watch` | 启用文件监听和热重编译。 |
| `-root` | 项目根目录，watch 模式需要此参数。 |
| `-config` | 站点配置文件，watch 模式使用。 |
| `-articles` | 文章元数据文件，watch 模式使用。 |
| `-latexml-bin` | `latexmlc` 路径，watch 模式使用。 |
| `-article-workers` | watch 模式下并行文章编译数。 |
| `-latexml-workers` | watch 模式下并行 LaTeXML 转换数。 |
| `-no-assets` | watch 模式下跳过资源处理。 |

启用 `-watch` 后，serve 在启动 HTTP 服务器之外还会启动文件监听器。监听器以每秒一次的频率轮询所有已注册文章源目录、关于页面目录和站点配置文件。当检测到文件修改时，使用 300ms 去抖动后仅重编译受影响的部分（启用 LaTeXML 缓存）：

1. **文章源文件修改**：仅重编译该文章的 HTML，输出到原位置替换原文件。
2. **关于页面修改**：重编译关于页面 HTML。
3. **站点配置修改**：重新加载配置，更新站点资源，重新生成所有索引页面，并强制重编译关于页面和所有文章页，避免 header、logo、icon 等站点级信息在旧页面中滞留。
4. **文章元数据修改**：重新加载文章列表，重新生成所有索引页面，并检查所有已注册文章页；缺失或源文件更新过的文章会被增量编译。

watch 模式下的增量编译经过 `source.Load` → `blocks.Lift` → LaTeXML 转换 → `parser.Parse` → `render.RenderWithOptions` 完整流程，与整站构建共用同一套构建函数和 LaTeXML 缓存。`LaTeXMLIdentity` 在 watch 启动时预热一次，所有后续重编译共享同一个 identity。

watch 模式还会启用浏览器自动刷新。HTTP handler 会对 HTML 响应注入一个本地脚本，该脚本轮询 `__metablog_live_reload` 内部端点并携带当前 `location.pathname`。watch 重建成功后只标记实际更新的 HTML 路径版本：索引页重建时标记对应索引页，about 重建时标记 `about/index.html`，单篇文章重建时标记该文章页，站点配置强制重建时标记所有受影响页面。浏览器只有在当前路径版本变化时才刷新，因此其他文章更新不会打断当前页面预览。

### 5.4 `article build`

单篇构建只编译一个 LaTeX 主文件，常用于调试解析器和文章渲染。

```bash
metablog article build -input articles/example/main.tex -out out/example
```

输出：

```text
out/example/index.html
out/example/static/
out/example/assets/
```

### 5.5 文章和缓存管理

文章管理命令读写 `data/articles.toml`：

```text
metablog article init
metablog article edit
metablog article delete
```

缓存清理命令删除项目根目录下的 `.metablog-cache/`：

```text
metablog cache clean -root .
```

## 6. 网站数据模型

### 6.1 `data/config.toml`

网站级配置：

```toml
title = "My Blog"
logo = "figs/circle_example.svg"
icon = "figs/circle_example.svg"
home_page_size = 10
article_list_page_size = 20
```

字段：

| 字段 | 说明 |
| --- | --- |
| `title` | 网站标题。 |
| `logo` | 相对于 `asset/` 的 logo 路径。 |
| `icon` | 相对于 `asset/` 的浏览器 icon 路径。 |
| `home_page_size` | 首页每页文章数量。 |
| `article_list_page_size` | 所有文章、分类、标签列表每页文章数量。 |

### 6.2 `data/articles.toml`

文章元数据：

```toml
[[articles]]
title = "My First Article"
description = "文章简介。"
author = "Author Name"
date = "2026-05-11"
category = ["Notes", "LaTeX"]
tags = ["LaTeX", "Blog"]
folder = "articles/my-first-article"
main_fig = "fig/main.pdf"
main_file = "main.tex"
deleted = false
```

文章输入文件由 `folder` 和 `main_file` 共同决定：

```text
<root>/<folder>/<main_file>
```

文章主图由 `folder` 和 `main_fig` 共同决定：

```text
<root>/<folder>/<main_fig>
```

只有 `deleted = false` 的文章会进入站点构建和列表页面。

### 6.3 关于页面

关于页面固定读取：

```text
data/about_page/main.tex
```

输出到：

```text
out/about/index.html
```

资源输出到：

```text
out/assets/about/
```

## 7. 整站页面生成

整站构建生成：

| 页面 | 输出 |
| --- | --- |
| 首页 | `out/index.html`，分页为 `out/page/<n>/index.html`。 |
| 所有文章 | `out/articles/index.html`，分页为 `out/articles/page/<n>/index.html`。 |
| 标签索引 | `out/tags/index.html`。 |
| 标签文章列表 | `out/tags/<tag>/index.html`，分页为 `out/tags/<tag>/page/<n>/index.html`。 |
| 分类索引 | `out/categories/index.html`。 |
| 分类文章列表 | `out/categories/<path>/index.html`，分页为 `out/categories/<path>/page/<n>/index.html`。 |
| 关于页面 | `out/about/index.html`。 |
| 文章页面 | `out/articles/<slug>/index.html`。 |

所有站点页面共享统一顶栏。顶栏左侧为 logo 和网站标题，右侧为所有文章、标签、分类、关于。

首页按日期从晚到早展示文章。文章块包含主图、标题、日期、分类、description 和 tags。所有文章、标签文章列表、分类文章列表按年份分组并分页。

## 8. 单篇文档构建流程

单篇文档构建由 `internal/app.buildArticle` 负责。

流程：

```text
source.Load
  -> blocks.Lift
  -> convertComplexBlocks
  -> parser.Parse
  -> assets.Converter.Process
  -> bib.Load
  -> render.RenderWithOptions
```

各阶段职责：

| 阶段 | 输入 | 输出 | 职责 |
| --- | --- | --- | --- |
| source.Load | 主 LaTeX 文件 | 展开后的 document 正文 | 展开 input/include，处理注释，保护原样区域，提取 document。 |
| blocks.Lift | document 正文 | 带复杂块占位符的正文 | 抽离 `tabular`、`tabularx`、`algorithm`。 |
| convertComplexBlocks | 复杂块 map | 复杂块 HTML | 并发调用 LaTeXML，读写缓存，清洗 fragment。 |
| parser.Parse | 正文和复杂块 | AST | 解析章节、段落、列表、figure、table、inline 等。 |
| assets.Process | AST | 修改后的 AST | 复制图片资源，PDF 转 SVG，回写输出路径。 |
| bib.Load | `.bib` 文件列表 | 引用表 | 解析 BibTeX 条目。 |
| render.RenderWithOptions | AST | HTML 字符串 | 编号、引用解析、目录、正文 HTML。 |

parser 不负责最终编号。编号、交叉引用和目录在 render 阶段统一完成。

## 9. LaTeX 源码加载

源码加载位于 `internal/latex/source`。

加载流程：

1. 读取主文件。
2. 使用 lexer 生成 token。
3. 在 token 层处理 `%` 注释。
4. 在 token 层展开 `\input{...}` 和 `\include{...}`。
5. 保护 `verbatim`、`lstlisting`、`minted` 和 `\verb` 中的内容，不把其中的 `%`、`\input`、`\include` 当作语法。
6. 检测 input/include 循环并记录 warning。
7. 提取 `\begin{document}` 和 `\end{document}` 之间的正文。

`\include` 在当前实现中和 `\input` 等价，不模拟分页、aux 文件或 `\includeonly`。

注释规则：

| 场景 | 行为 |
| --- | --- |
| 整行 `%` 注释 | 删除整行，不留下空行。 |
| 行内 `%` 注释 | 删除 `%` 之后内容，保留换行。 |
| `\%` | 保留为文本百分号。 |
| 原样环境中的 `%` | 不作为注释处理。 |

## 10. 词法层

词法层位于 `internal/latex/lexer`。

主要 token 类型包括：

| Token | 含义 |
| --- | --- |
| Text | 普通文本。 |
| Command | 控制词或控制符。 |
| BeginGroup / EndGroup | `{` 和 `}`。 |
| BeginOptional / EndOptional | `[` 和 `]`。 |
| BeginEnv / EndEnv | `\begin{...}` 和 `\end{...}`。 |
| Math | 行内或块级公式片段。 |
| Comment | `%` 注释。 |
| Raw | 原样环境或 `\verb` 内容。 |

控制词按 TeX 字母边界识别，因此 `\includegraphics` 不会被误判为 `\include`。

lexer 的职责是切分和保护，不负责解释语义。语义判断由 source parser、block parser 和 inline parser 完成。

## 11. 复杂块抽离

复杂块抽离位于 `internal/latex/blocks`。

当前抽离的环境：

1. `tabular`
2. `tabularx`
3. `algorithm`
4. `algorithm*`

抽离后，正文中会留下占位符：

```text
@@METABLOG_COMPLEX_BLOCK_0001@@
```

`table` / `table*` 本身不再抽离给 LaTeXML，而是由 Go parser 解析。这样外层 table 的 `caption`、`label`、`\subfloat`、子表结构可以被 MetaBlog 控制；只有真正复杂的 `tabular` / `tabularx` 内容交给 LaTeXML。

复杂块结构：

| 字段 | 说明 |
| --- | --- |
| `ID` | 占位符 ID。 |
| `EnvName` | 环境名。 |
| `RawTeX` | 原始 LaTeX。 |
| `HTML` | LaTeXML 转换后的 HTML。 |
| `Caption` | 从复杂块内提取的 caption。 |
| `Label` | 从复杂块内提取的 label。 |

## 12. LaTeXML 转换和缓存

LaTeXML 转换位于 `internal/latexml`。

每个复杂块转换流程：

1. 判断是否可以使用缓存。
2. 计算缓存 key。
3. 尝试读取 `.metablog-cache/latexml/` 中的 JSON 缓存。
4. 命中时校验 hash 和完整 RawTeX。
5. 未命中时创建临时 LaTeX 文件。
6. 调用 `latexmlc --format=html5 --whatsout=fragment`。
7. 读取 HTML。
8. 写入缓存。
9. 清洗 fragment。
10. 写回 `ComplexBlock.HTML`。

缓存 key 包含：

| 信息 | 目的 |
| --- | --- |
| RawTeX SHA-256 | 区分复杂块内容。 |
| 完整 RawTeX | 命中后做逐字节完全匹配。 |
| LaTeXML wrapper SHA-256 | wrapper 改动后缓存失效。 |
| 调用参数 SHA-256 | 参数改动后缓存失效。 |
| `latexmlc` 解析路径 | 不同可执行文件缓存隔离。 |
| `latexmlc` 版本输出 | 版本变化后缓存失效。 |
| cache schema | 缓存结构升级时失效。 |

缓存绕过条件：

1. 启用 `-keep-temp`。
2. RawTeX 包含 `\input`。
3. RawTeX 包含 `\include`。
4. RawTeX 包含 `\includegraphics`。
5. RawTeX 包含 `\bibliography` 或 `\addbibresource`。
6. 启用 `-no-latexml-cache`。

LaTeXML identity 在构建开始阶段预热一次。单篇构建、about 页面和所有文章共享同一个 `CacheIdentity`，避免每个文档重复执行版本探测。

HTML 清洗策略：

1. 提取 body fragment。
2. 删除 style/script。
3. 删除 LaTeXML 自动 id。
4. MathML 根据 `alttext` 还原给 KaTeX。
5. 只保留安全颜色样式。
6. 修复部分 inline `aligned` 公式。
7. 包裹为 `<div class="metablog-latexml-fragment">...</div>`。

转换失败时生成 fallback `<pre><code>`，并记录 warning。

## 13. Parser 主流程

Parser 位于 `internal/latex/parser`，入口为 `Parse`。

主流程：

1. 扫描顶层元信息命令。
2. 解析正文 block。
3. 解析 inline 内容。
4. 生成 `ast.Document`。
5. 收集 parser warning。

顶层元信息包括：

| 命令 | 说明 |
| --- | --- |
| `\title{...}` | 文档标题；多次出现时后者覆盖前者并 warning。 |
| `\author[...]{...}{...}{...}` | 项目自定义作者命令。 |
| `\defInstitution{key}{text}` | 项目自定义机构命令。 |

元信息扫描只看顶层命令。遇到 `{...}` 分组或环境时会跳过作用域，不把环境内部命令误当成文档元信息。

`abstract`、`keywords`、`IEEEkeywords` 不作为 metadata 抽离，而是在原位置生成专用 block。因此文档中多处使用这些环境时，都会按对应样式渲染在原位置。

## 14. Block 解析

Block parser 基于源码位置和 token 边界推进。

识别的主要 block：

| Block | 来源 |
| --- | --- |
| Section | `\section`、`\subsection`、`\subsubsection`。 |
| Paragraph | 普通段落。 |
| MathBlock | `\[...\]`、`equation`、`align` 等。 |
| Figure | `figure` / `figure*`。 |
| Table | `table` / `table*`。 |
| List | `itemize`、`enumerate`、`description`。 |
| TCB | 自定义 `tcb` 环境。 |
| CodeBlock | `verbatim`、`lstlisting`、`minted`。 |
| AbstractBlock | `abstract`。 |
| KeywordsBlock | `keywords` / `IEEEkeywords`。 |
| ComplexBlock | LaTeXML 托管块占位符。 |
| EnvironmentBlock | 未知环境的透明保留块。 |

### 14.1 章节

支持：

```latex
\section{...}
\subsection{...}
\subsubsection{...}
\appendices
```

`\appendices` 后的 section 会按附录编号。章节标题支持 inline 命令和文本型声明式样式。

### 14.2 figure

`figure` parser 解析：

1. `\includegraphics`
2. `\caption`
3. `\label`
4. `\subfloat`
5. 常见 layout wrapper，例如 `resizebox`、`scalebox`、`rotatebox`、`adjustbox`、`makebox`、`fbox`

子图编号为 `(a)`、`(b)`、...、`(aa)`、`(ab)`。交叉引用显示为 `1.a` 这类格式。

### 14.3 table

`table` parser 解析外层 table 结构：

1. table caption 和 label。
2. `\subfloat` 子表。
3. 子 caption 和子 label。
4. layout wrapper。
5. 内部 `tabular` / `tabularx` 占位符。

真正复杂的表格主体由 LaTeXML 转换。

### 14.4 list

支持：

1. `itemize`
2. `enumerate`
3. `description`

list parser 能处理多层嵌套列表，不会把子列表中的 `\item` 误切成父列表项。`description` 的可选 label 支持 inline 样式。

### 14.5 分组

块级 `{...}` 被视为局部作用域，不直接把 `{` / `}` 渲染到 HTML。分组中的声明式样式可以影响组内内容，但不泄漏到组外。

### 14.6 代码块

`verbatim`、`lstlisting` 和 `minted` 都解析为 `ast.CodeBlock`，但语言来源不同：

1. `verbatim` 不提供语言，渲染时默认按 `text` 处理。
2. `lstlisting` 从 `[...]` 的 `language` 参数读取语言，其他参数忽略。参数解析支持 `language=C`、`language={C++}`、`frame,language=Go,numbers=left` 和 begin 后带空白的写法。
3. `minted` 忽略 `[...]` 中的所有可选参数，从 `{...}` 必选参数读取语言。

代码块正文按原样文本处理，不解析 inline 命令，不处理 `%` 注释，也不展开 input/include。

渲染阶段由 `internal/render/highlight_code.go` 调用 `internal/highlight.Highlight`。`internal/highlight` 使用 Chroma 根据语言生成带 class 的 HTML 片段；未识别语言时使用 fallback lexer，保证代码内容仍可显示。Chroma 默认输出的 `<pre><code>` 外壳会被剥离，最终由 MetaBlog 自己生成统一代码框：

1. 根节点带 `code-block chchroma`，其中 `chchroma` 用于匹配 Chroma 主题 CSS 选择器。
2. 标题栏左侧显示语言，右侧显示自动换行、复制和折叠按钮。
3. 正文使用表格组织行号和代码内容，行号不参与复制。
4. 额外写入隐藏的 raw source `<textarea>`，复制按钮优先复制该原始文本，避免从高亮 HTML 或可见行文本反推源码造成空行、缩进丢失。

`internal/render/code_block_script.go` 注入浏览器端脚本，负责切换自动换行、复制原始代码和折叠代码框。自动换行关闭时保留 `white-space: pre` 并允许横向滚动；开启后改为 `pre-wrap`，视觉折行但源码行号不变。

## 15. Inline 解析

Inline parser 负责把文本片段解析为 inline AST。

支持的主要 inline 类型：

| 类型 | 示例 |
| --- | --- |
| 文本 | 普通文本。 |
| 样式 | `\textbf{...}`、`\textit{...}`、`\emph{...}`、`\underline{...}`。 |
| 声明式字号 | `\tiny`、`\small`、`\large`、`\Huge` 等。 |
| 对齐声明 | `\centering` 等。 |
| 颜色 | `\color{blue}`、`\textcolor{red}{...}`。 |
| 链接 | `\url{...}`、`\href{url}{text}`。 |
| 引用 | `\ref{...}`、`\cite{...}`。 |
| 公式 | `$...$`、`\(...\)`。 |

文本型参数使用统一入口解析，例如：

1. `\title{...}`
2. 章节标题
3. figure/table caption
4. tcb 标题
5. `\href` 显示文本

因此这些位置都支持声明式字号、居中、颜色等样式，且 `\tiny\centering` 和 `\centering\tiny` 这类顺序不会影响最终样式合并。

非文本型参数不走文本入口，例如：

1. `\label{...}`
2. `\ref{...}`
3. `\cite{...}`
4. `\bibliography{...}`

这些参数保持原始语义。

未知 inline 命令会记录 warning，并尽可能保留可降级文本。

## 16. AST 设计

AST 位于 `internal/latex/ast`。

顶层结构：

```text
Document
  Metadata
  Children []Block
  Labels
  Citations
  References
  Warnings
```

Block 和 Inline 分离：

| 类别 | 代表节点 |
| --- | --- |
| Block | Section、Paragraph、Figure、Table、List、TCB、CodeBlock、MathBlock。 |
| Inline | Text、Styled、Link、Reference、Citation、Math。 |

Parser 负责语义结构，Render 负责编号和最终 HTML。

这种分工的原因：

1. 同一 label 可能在解析完成后才知道最终编号。
2. 目录需要遍历完整 section 树。
3. 引用和参考文献需要全局信息。
4. render 阶段可以统一处理 HTML escape、链接和样式。

## 17. 编号和交叉引用

编号在 `internal/render` 中完成。

编号对象：

| 对象 | 格式 |
| --- | --- |
| Section | `1`、`1.1`、`1.1.1`；附录为 `App. A`。 |
| Figure | `1`、`2`。 |
| Subfigure | `(a)` 显示，引用为 `1.a`。 |
| Table | `1`、`2`。 |
| Subtable | `(a)` 显示，引用为 `1.a`。 |
| Equation | `(1)`、`(2)`。 |
| Citation | IEEE 风格 `[1]`。 |

引用命令：

```latex
\label{key}
\ref{key}
\cite{bibkey}
```

找不到 label 或 bib key 时记录 warning。引用范围会按 IEEE 风格压缩。

## 18. 数学公式

MetaBlog 负责识别公式边界、生成 AST、编号和交叉引用；公式内容的渲染交给 KaTeX。

支持：

1. `$...$`
2. `\(...\)`
3. `\[...\]`
4. `equation` / `equation*`
5. `align` / `align*`

当前不尝试手写解析数学内部语法。

## 19. 参考文献

参考文献流程：

1. parser 识别 `\bibliography{...}`。
2. `bib.Load` 加载 `.bib` 文件。
3. inline parser 识别 `\cite{...}`。
4. render 阶段按引用出现顺序编号。
5. 在 References block 位置或文末输出参考文献。

`\bibliographystyle{...}` 会识别并忽略。最终渲染为项目内置的简化 IEEE 风格。

## 20. 资源处理

资源处理位于 `internal/assets`。

资源来源：

| 来源 | 输出 |
| --- | --- |
| 文章资源 | `out/assets/articles/<slug>/...` |
| 关于页面资源 | `out/assets/about/...` |
| 站点资源 | `out/assets/site/...` |

处理规则：

1. 普通文件直接复制。
2. PDF 图片转为 SVG。
3. PDF 转换工具按 `pdftocairo`、`mutool`、`inkscape` 顺序尝试。
4. 转换失败记录 warning。
5. 输出资源路径写回 AST 或站点配置。
6. 源文件未变化且输出文件已存在时跳过复制或转换。

## 21. 静态资源和字体

静态资源位于 `web/static/`。

构建时复制到：

```text
out/static/
```

代码高亮需要额外的静态资源：

1. `web/static/fonts/source-code-pro-regular.woff2`
2. `web/static/fonts/source-code-pro-bold.woff2`
3. `web/static/fonts.css` 中的 Source Code Pro `@font-face`
4. `out/static/chroma-theme.css`

`site init` 会下载 Source Code Pro 字体，并写入默认 `web/static/chroma-theme.css`。整站构建时，如果输出目录中不存在 `static/chroma-theme.css`，会由 `internal/highlight.ThemeCSS` 使用当前 Chroma 主题重新生成。文章页面会额外引用该 CSS，使 `lstlisting` / `minted` 代码框中的 token class 得到正确着色。

如果启用：

```bash
metablog site build -subset-fonts
```

则执行字体子集化：

1. 扫描输出 HTML 中出现的字符。
2. 加入默认 ASCII 和常见中文标点字符。
3. 计算字符集和源字体 hash。
4. 如果 manifest 有效，复用已有子集。
5. 否则调用 `pyftsubset` 生成 `.subset.woff2`。
6. 写出引用子集字体的 `fonts.css`。
7. 删除输出目录中的完整字体文件。

`-subset-fonts` 依赖 Python、fonttools、brotli 和 `pyftsubset`。

## 22. 增量构建

### 22.1 文档级 fresh 判断

整站构建会对 about 页面和每篇文章做文档级 fresh 判断。

逻辑：

1. 扫描文档源目录所有文件。
2. 取源目录中文件的最晚修改时间。
3. 读取 HTML 输出文件修改时间。
4. 如果源目录最晚修改时间不晚于 HTML 输出文件，则跳过文档编译。

启用 `-force` 会跳过该判断，强制重新编译。

### 22.2 资源级 fresh 判断

资源处理按单文件判断：

1. 输出资源存在。
2. 输出资源非空。
3. 输出资源修改时间不早于源文件。

满足时跳过复制或转换。

资源输出不参与文档级 fresh 判断；文档级判断只看对应 HTML 输出。

## 23. 并发设计

并发点：

| 并发任务 | 控制参数 |
| --- | --- |
| 整站文章构建 | `-article-workers` |
| about 页面和文章队列 | 固定并行启动。 |
| LaTeXML 复杂块转换 | `-latexml-workers` |

日志策略：

1. 构建开始时输出整体概况。
2. 每个文档使用独立 buffer 收集日志。
3. 文档完成、失败或跳过后一次性输出日志块。
4. LaTeXML cache 和 assets fresh 情况只输出数量摘要。
5. 所有 warning 都列出。

这样可以避免并发构建时多篇文章日志交错。

### 23.1 文件监听（Watch 模式）

`site serve -watch` 启动后会运行一个轻量级文件监听器，实现源码修改时自动增量编译。

**轮询机制**：使用 `time.Ticker` 每秒扫描一次所有被监听目录的最晚文件修改时间。服务启动时立即执行一次基线扫描，记录各目录的当前状态。后续每次轮询将当前最晚修改时间与上次记录的时间比较，发现更新时触发重编译。

**去抖动**：使用 300ms 去抖定时器。在去抖窗口内收到的多个变更请求会被合并为一次批量重编译，避免编辑器连续保存时频繁触发构建。

**监听范围**：
- 每篇文章的源目录（来自 `data/articles.toml` 的 `folder` 字段）
- 关于页面目录 `data/about_page/`
- 站点配置文件 `data/config.toml`
- 文章元数据文件 `data/articles.toml`

**增量编译策略**：
- 文章源文件变更：仅重编译该文章的 HTML
- 关于页面源文件变更：重编译关于页面
- 配置变更：重新加载站点数据，更新站点资源，重新生成所有索引页面，并强制重编译关于页面和所有文章页
- 元数据变更：重新加载站点数据，重新生成所有索引页面，并检查已注册文章页，构建缺失或过期的文章输出

重编译使用与整站构建相同的 `buildArticle` / `buildOneSiteArticle` / `writeSitePages` 函数，共享同一个 `LaTeXMLIdentity` 和缓存目录。`NoAssets` 选项在 watch 模式下可跳过资源复制以加快编译速度。

**与 HTTP 服务器的协调**：监听器在独立的 goroutine 中运行，与 HTTP 文件服务器共享同一个输出目录。重编译完成后，HTTP 服务器自动提供更新后的文件，无需重启。

**浏览器自动刷新**：watch 模式下 HTTP handler 会包装静态文件服务，对 HTML 响应注入自动刷新脚本，并提供 `__metablog_live_reload` 版本查询端点。每次重建成功后，watchState 只递增受影响 HTML 路径的版本号；浏览器端脚本按当前 `location.pathname` 查询版本，只有当前页面版本变化时才调用 `location.reload()`。该机制同时支持普通磁盘输出和 `-only-ram` 内存输出。

### 23.2 纯内存模式（Only-RAM）

`site serve -only-ram` 进一步将所有输出文件操作移入内存。

**MemStore**：`memStore` 是一个 `sync.RWMutex` 保护的内存文件表，以相对路径为键存储文件内容和内存中的修改时间。启动时通过 `loadDir` 递归加载整个 `out/` 目录。MemStore 实现了 `http.Handler` 接口，替代 `http.FileServer` 直接从内存响应用户请求。

**输出路径**：当 `Config.MemStore != nil` 时，`buildSiteAboutPage` 和 `buildOneSiteArticle` 跳过所有磁盘 `WriteFile` 调用，仅将 HTML 内容写入 MemStore。watch 模式下的 `rebuildAboutPage` 和 `rebuildOneArticle` 同理，通过 `watchState.store` 判断使用内存还是磁盘路径。

**LaTeXML 内存缓存**：`latexml.CacheStore` 提供了一个可选的有界内存读缓存层。在 `readCache` 中，先通过 `CacheStore.get(meta, raw)` 查找内存；miss 后才执行磁盘 `os.ReadFile`；磁盘命中后将完整缓存条目写入内存供后续使用。内存命中仍会校验缓存 key、环境信息和完整 RawTeX，语义与磁盘缓存一致。`CacheStore` 由 `loadSiteForWatch` 在 `-only-ram` 启用时创建，通过 `Config.CacheStore` 传递到 `buildArticle` 中的 `latexml.Runner`。

**资源处理**：当 `Config.MemStore != nil` 且未显式 `-no-assets` 时，文档图片、主图、logo 和 icon 等资源会写入 MemStore，而不写入 `out/`。普通文件直接读取源文件进入内存；PDF 转 SVG 需要通过临时文件调用外部转换器，转换产物读取后写入 MemStore，临时目录随后删除。

**内存泄漏预防**：
- MemStore 使用单层文件表，覆盖写入时旧内容由 GC 自动回收。
- 索引页重建时会用新的页面集合替换旧集合，删除已经失效的 tag/category/page URL。
- 文章元数据变化时会删除不再注册的文章页，避免内存中长期保留孤立文章输出。
- watch 去抖和 channel 操作均使用带缓冲的 channel，发送方以 non-blocking select 避免阻塞和 goroutine 泄漏。
- LaTeXML CacheStore 有容量上限，超过上限时淘汰最早写入的缓存条目，避免长时间编辑复杂块导致内存缓存无限增长。

## 24. Warning 和错误策略

MetaBlog 尽量区分错误和 warning：

| 类型 | 行为 |
| --- | --- |
| 输入文件不存在 | 返回错误，构建失败。 |
| 配置文件解析失败 | 返回错误，构建失败。 |
| LaTeXML 转换失败 | 记录 warning，使用 fallback HTML。 |
| 图片资源缺失 | 记录 warning，继续构建。 |
| 未知 inline 命令 | 记录 warning，降级保留文本。 |
| 未知环境 | 记录 warning，作为透明环境解析内部内容。 |
| 找不到 label / citation | 记录 warning。 |

启用 `-strict` 时，只要产生 warning，构建最终失败。

## 25. Go 文件职责

### 25.1 CLI 和应用层

| 文件 | 职责 |
| --- | --- |
| `cmd/metablog/main.go` | CLI 入口，分发 `site`、`article`、`cache` 子命令，并保留旧根命令兼容入口。 |
| `internal/app/app.go` | 单篇构建编排、构建级 LaTeXML identity、文档日志和通用构建函数。 |
| `internal/app/site.go` | 整站构建，加载站点数据，构建 about 和文章，写站点页面和静态资源。 |
| `internal/app/site_init.go` | 网站初始化，创建目录、写入内置模板、下载字体、检测环境。 |
| `internal/app/site_serve.go` | 本地预览服务器，监听指定 host/port 并用静态文件服务器暴露输出目录。 |
| `internal/app/serve_watch.go` | 文件监听和热重编译：轮询源目录修改时间，在文件变更时增量编译对应文档并更新输出。 |
| `internal/app/live_reload.go` | watch 模式下的浏览器自动刷新：注入页面脚本、提供版本查询端点并跟踪已更新页面版本。 |
| `internal/app/memstore.go` | 内存文件存储：提供 HTTP 文件服务、并发安全的读写接口，用于 `-only-ram` 模式。 |
| `internal/app/article_cli.go` | `article init/edit/delete` 的交互式元数据维护。 |
| `internal/app/cache.go` | `.metablog-cache/` 清理。 |

关键函数：

| 函数 | 职责 |
| --- | --- |
| `app.Run` | 根据 `Config.Site` 选择单篇构建或整站构建。 |
| `buildArticle` | 单篇 LaTeX 到 HTML 的完整转换流程。 |
| `RunSite` | 整站构建入口。 |
| `RunSiteInit` | 网站初始化入口。 |
| `RunArticleInit/Edit/Delete` | 文章元数据管理。 |
| `RunCacheClean` | 缓存清理。 |

### 25.2 网站层

| 文件 | 职责 |
| --- | --- |
| `internal/blog/blog.go` | 读取配置和文章元数据，生成首页、文章列表、标签、分类等站点页面 HTML。 |
| `internal/site/builder.go` | 复制静态资源，写默认 CSS、Chroma 主题 CSS、默认字体 CSS 和 warning 文件。 |
| `internal/site/font_subset.go` | 扫描 HTML 字符并生成字体子集。 |

### 25.3 LaTeX 层

| 文件 | 职责 |
| --- | --- |
| `internal/latex/ast/nodes.go` | AST 节点定义。 |
| `internal/latex/lexer/lexer.go` | 词法切分和原样区域保护。 |
| `internal/latex/source/loader.go` | 主文件加载和递归 input/include。 |
| `internal/latex/source/parser.go` | token 层注释处理、命令识别、分组读取。 |
| `internal/latex/source/comments.go` | 注释处理工具函数。 |
| `internal/latex/blocks/blocks.go` | 复杂块抽离和占位符管理。 |
| `internal/latex/parser/parser.go` | block parser、section、figure、table、list、metadata 等解析。 |
| `internal/latex/parser/inline.go` | inline parser 和文本型参数解析。 |

### 25.4 转换、渲染和资源

| 文件 | 职责 |
| --- | --- |
| `internal/latexml/runner.go` | LaTeXML 调用、缓存、HTML fragment 清洗和 fallback。 |
| `internal/render/html.go` | AST 到文章 HTML，编号、目录、引用、KaTeX 依赖。 |
| `internal/render/highlight_code.go` | 代码块 HTML 渲染，组织标题栏、行号、隐藏原始源码和 Chroma 高亮片段。 |
| `internal/render/code_block_script.go` | 代码框浏览器端交互脚本，包括自动换行、复制和折叠。 |
| `internal/highlight/highlight.go` | Chroma 封装，负责语言归一化、语法高亮 HTML 生成和主题 CSS 生成。 |
| `internal/assets/assets.go` | 资源复制、PDF 转 SVG、资源 fresh 判断。 |
| `internal/bib/bib.go` | BibTeX 文件解析和参考文献字段清洗。 |

## 26. 外部依赖

Go 模块依赖：

| 依赖 | 用途 |
| --- | --- |
| `github.com/alecthomas/chroma/v2` | `lstlisting` / `minted` 代码块语法高亮和主题 CSS 生成。 |
| `github.com/pelletier/go-toml/v2` | TOML 配置读写。 |
| `gopkg.in/yaml.v3` | YAML 配置兼容。 |

运行时外部工具：

| 工具 | 用途 | 是否必须 |
| --- | --- | --- |
| `latexmlc` | 转换 `tabular`、`tabularx`、`algorithm`。 | 文档含复杂块时需要。 |
| `pdftocairo` | PDF 图片转 SVG 首选工具。 | 文档含 PDF 图片时需要任意转换器。 |
| `mutool` | PDF 图片转 SVG 备用工具。 | 可选。 |
| `inkscape` | PDF 图片转 SVG 备用工具。 | 可选。 |
| `pyftsubset` | 字体子集化。 | 使用 `-subset-fonts` 时需要。 |
| Python `fontTools` / `brotli` | 字体子集化。 | 使用 `-subset-fonts` 时需要。 |

前端运行时：

| 依赖 | 来源 |
| --- | --- |
| KaTeX CSS/JS | 页面中通过 CDN 引入。 |

## 27. 测试策略

测试分布：

| 目录 | 覆盖重点 |
| --- | --- |
| `internal/app/*_test.go` | 构建流程、site init、文章 CLI、缓存清理。 |
| `internal/blog/*_test.go` | 站点页面、分页、分类、标签、主图 URL。 |
| `internal/latex/source/*_test.go` | 注释、input/include、原样区域保护。 |
| `internal/latex/lexer/*_test.go` | token 边界、注释 token、raw token。 |
| `internal/latex/blocks/*_test.go` | 复杂块抽离。 |
| `internal/latex/parser/*_test.go` | metadata、section、figure、table、list、inline、未知命令。 |
| `internal/latexml/*_test.go` | HTML 清洗、缓存 identity、公式修复。 |
| `internal/render/*_test.go` | 编号、引用、HTML 输出、代码框渲染和复制原文保真。 |
| `internal/highlight/*_test.go` | Chroma 语言归一化、语法高亮输出和 fallback 行为。 |
| `internal/site/*_test.go` | 静态资源和字体子集化。 |

运行：

```bash
go test ./...
```

## 28. 设计权衡

### 28.1 为什么不完整实现 LaTeX

完整 TeX 需要宏展开、寄存器、条件、盒模型、包系统和排版算法，复杂度远超博客脚手架。MetaBlog 只处理写作中稳定可控的语义结构。

### 28.2 为什么 table 外壳手写解析、tabular 托管给 LaTeXML

`table` 的 caption、label、subfloat 决定网页语义和交叉引用，适合手写解析。`tabular` 内部语法复杂且包语义多，适合交给 LaTeXML。

### 28.3 为什么编号放在 render 阶段

编号依赖全局遍历结果。放在 render 阶段可以统一处理目录、label、引用、子图子表和公式编号。

### 28.4 为什么缓存保存 RawTeX 并完全匹配

SHA-256 碰撞概率极低，但缓存正确性优先。保存 RawTeX 并逐字节匹配可以让缓存命中条件更直观，也方便调试。

### 28.5 为什么资源按文章 slug 隔离

不同文章可能都有 `fig/main.pdf`。输出到 `out/assets/articles/<slug>/...` 可以避免同名冲突。

## 29. 后续可扩展方向

当前可继续改进的方向：

1. 对更多 LaTeX 环境进行手写结构化解析。
2. 增加环境检查命令，例如 `metablog doctor`。
3. 支持更多 bibliography 样式。
4. 支持可配置 KaTeX 资源来源。
5. 为 site init 的字体下载增加可配置镜像源。
