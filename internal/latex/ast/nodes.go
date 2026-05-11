package ast

type Document struct {
	Title             []Inline                  `json:"title,omitempty"`
	TitleAlign        string                    `json:"titleAlign,omitempty"`
	Authors           []Author                  `json:"authors,omitempty"`
	Institutions      []Institution             `json:"institutions,omitempty"`
	Abstract          []Block                   `json:"abstract,omitempty"`
	Keywords          []Inline                  `json:"keywords,omitempty"`
	Children          []Block                   `json:"children,omitempty"`
	BibliographyFiles []string                  `json:"bibliographyFiles,omitempty"`
	References        map[string]ReferenceEntry `json:"references,omitempty"`
	Warnings          []string                  `json:"warnings,omitempty"`
	InputFile         string                    `json:"inputFile,omitempty"`
	SourceRoot        string                    `json:"sourceRoot,omitempty"`
}

type Author struct {
	Attributes       []string `json:"attributes,omitempty"`
	Name             []Inline `json:"name,omitempty"`
	InstitutionCodes []string `json:"institutionCodes,omitempty"`
	Email            string   `json:"email,omitempty"`
}

type Institution struct {
	Code    string   `json:"code"`
	Aliases []string `json:"aliases,omitempty"`
	Number  int      `json:"number"`
	Info    []Inline `json:"info,omitempty"`
}

type Block interface {
	BlockKind() string
}

type Inline interface {
	InlineKind() string
}

type Section struct {
	Level      int      `json:"level"`
	Title      []Inline `json:"title"`
	TitleAlign string   `json:"titleAlign,omitempty"`
	Label      string   `json:"label,omitempty"`
	Number     string   `json:"number,omitempty"`
	AnchorID   string   `json:"anchorId,omitempty"`
	Appendix   bool     `json:"appendix,omitempty"`
	Children   []Block  `json:"children,omitempty"`
}

func (*Section) BlockKind() string { return "section" }

type Paragraph struct {
	Inlines []Inline `json:"inlines"`
	Align   string   `json:"align,omitempty"`
}

func (*Paragraph) BlockKind() string { return "paragraph" }

type StyledBlock struct {
	Color      string  `json:"color,omitempty"`
	Background string  `json:"background,omitempty"`
	Align      string  `json:"align,omitempty"`
	Underline  bool    `json:"underline,omitempty"`
	Bold       bool    `json:"bold,omitempty"`
	Italic     bool    `json:"italic,omitempty"`
	Mono       bool    `json:"mono,omitempty"`
	FontSize   string  `json:"fontSize,omitempty"`
	Children   []Block `json:"children,omitempty"`
}

func (*StyledBlock) BlockKind() string { return "styledBlock" }

type AbstractBlock struct {
	Children []Block `json:"children,omitempty"`
}

func (*AbstractBlock) BlockKind() string { return "abstract" }

type KeywordsBlock struct {
	Inlines []Inline `json:"inlines,omitempty"`
}

func (*KeywordsBlock) BlockKind() string { return "keywords" }

type EnvironmentBlock struct {
	EnvName  string  `json:"envName,omitempty"`
	Children []Block `json:"children,omitempty"`
}

func (*EnvironmentBlock) BlockKind() string { return "environment" }

type DisplayMath struct {
	TeX      string `json:"tex"`
	Label    string `json:"label,omitempty"`
	Number   string `json:"number,omitempty"`
	AnchorID string `json:"anchorId,omitempty"`
}

func (*DisplayMath) BlockKind() string { return "displayMath" }

type List struct {
	Ordered bool        `json:"ordered"`
	Kind    string      `json:"kind,omitempty"`
	Items   []*ListItem `json:"items"`
}

func (*List) BlockKind() string { return "list" }

type ListItem struct {
	Label  []Inline `json:"label,omitempty"`
	Blocks []Block  `json:"blocks"`
}

type Figure struct {
	Starred      bool              `json:"starred"`
	Image        *Image            `json:"image,omitempty"`
	Images       []*Image          `json:"images,omitempty"`
	Subfigures   []*Subfigure      `json:"subfigures,omitempty"`
	Caption      []Inline          `json:"caption,omitempty"`
	CaptionAlign string            `json:"captionAlign,omitempty"`
	Label        string            `json:"label,omitempty"`
	Number       string            `json:"number,omitempty"`
	AnchorID     string            `json:"anchorId,omitempty"`
	Options      map[string]string `json:"options,omitempty"`
}

func (*Figure) BlockKind() string { return "figure" }

type Table struct {
	Starred      bool        `json:"starred"`
	Children     []Block     `json:"children,omitempty"`
	Subtables    []*Subtable `json:"subtables,omitempty"`
	Caption      []Inline    `json:"caption,omitempty"`
	CaptionAlign string      `json:"captionAlign,omitempty"`
	Label        string      `json:"label,omitempty"`
	Number       string      `json:"number,omitempty"`
	AnchorID     string      `json:"anchorId,omitempty"`
}

func (*Table) BlockKind() string { return "table" }

type TCB struct {
	Title           []Inline `json:"title,omitempty"`
	TitleAlign      string   `json:"titleAlign,omitempty"`
	TitleBackground string   `json:"titleBackground,omitempty"`
	BorderColor     string   `json:"borderColor,omitempty"`
	BodyBackground  string   `json:"bodyBackground,omitempty"`
	Children        []Block  `json:"children,omitempty"`
}

func (*TCB) BlockKind() string { return "tcb" }

type CodeBlock struct {
	EnvName  string `json:"envName,omitempty"`
	Language string `json:"language,omitempty"`
	Text     string `json:"text"`
}

func (*CodeBlock) BlockKind() string { return "codeBlock" }

type Subfigure struct {
	ImageIndex int      `json:"imageIndex"`
	Label      string   `json:"label,omitempty"`
	Caption    []Inline `json:"caption,omitempty"`
	Number     string   `json:"number,omitempty"`
	AnchorID   string   `json:"anchorId,omitempty"`
	BreakAfter bool     `json:"breakAfter,omitempty"`
}

type Subtable struct {
	Blocks     []Block  `json:"blocks,omitempty"`
	Label      string   `json:"label,omitempty"`
	Caption    []Inline `json:"caption,omitempty"`
	Number     string   `json:"number,omitempty"`
	AnchorID   string   `json:"anchorId,omitempty"`
	BreakAfter bool     `json:"breakAfter,omitempty"`
}

type Image struct {
	SourcePath string            `json:"sourcePath"`
	OutputPath string            `json:"outputPath,omitempty"`
	Options    map[string]string `json:"options,omitempty"`
}

type ComplexHTML struct {
	BlockID  string `json:"blockId"`
	EnvName  string `json:"envName"`
	RawTeX   string `json:"rawTex,omitempty"`
	HTML     string `json:"html,omitempty"`
	Caption  string `json:"caption,omitempty"`
	Label    string `json:"label,omitempty"`
	Number   string `json:"number,omitempty"`
	AnchorID string `json:"anchorId,omitempty"`
}

func (*ComplexHTML) BlockKind() string { return "complexHTML" }

type References struct {
	Files []string `json:"files,omitempty"`
}

func (*References) BlockKind() string { return "references" }

type ReferenceEntry struct {
	Key    string            `json:"key"`
	Type   string            `json:"type,omitempty"`
	Fields map[string]string `json:"fields,omitempty"`
}

type Text struct {
	Value string `json:"value"`
}

func (*Text) InlineKind() string { return "text" }

type Bold struct {
	Children []Inline `json:"children"`
}

func (*Bold) InlineKind() string { return "bold" }

type Italic struct {
	Children []Inline `json:"children"`
}

func (*Italic) InlineKind() string { return "italic" }

type Styled struct {
	Children   []Inline `json:"children"`
	Color      string   `json:"color,omitempty"`
	Background string   `json:"background,omitempty"`
	Underline  bool     `json:"underline,omitempty"`
	Bold       bool     `json:"bold,omitempty"`
	Italic     bool     `json:"italic,omitempty"`
	Mono       bool     `json:"mono,omitempty"`
	FontSize   string   `json:"fontSize,omitempty"`
}

func (*Styled) InlineKind() string { return "styled" }

type InlineMath struct {
	TeX string `json:"tex"`
}

func (*InlineMath) InlineKind() string { return "inlineMath" }

type Link struct {
	URL      string   `json:"url"`
	Children []Inline `json:"children"`
}

func (*Link) InlineKind() string { return "link" }

type Cite struct {
	Keys []string `json:"keys"`
}

func (*Cite) InlineKind() string { return "cite" }

type Ref struct {
	Key string `json:"key"`
}

func (*Ref) InlineKind() string { return "ref" }

type Footnote struct {
	Children []Inline `json:"children"`
}

func (*Footnote) InlineKind() string { return "footnote" }
