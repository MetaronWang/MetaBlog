package app

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"MetaBlog/internal/blog"
)

type ArticleCLIConfig struct {
	RootDir      string
	ArticlesFile string
	In           io.Reader
	Out          io.Writer
}

func RunArticleInit(cfg ArticleCLIConfig) error {
	cfg = normalizeArticleCLIConfig(cfg)
	articlesPath := resolveRootPath(cfg.RootDir, cfg.ArticlesFile)
	articles, err := blog.LoadArticles(articlesPath)
	if err != nil {
		return err
	}
	p := prompter{in: bufio.NewReader(cfg.In), out: cfg.Out}
	title, err := p.askRequired("Title")
	if err != nil {
		return err
	}
	slugDefault := blog.Slugify(title)
	folderDefault := filepath.ToSlash(filepath.Join("articles", slugDefault))
	folder, err := p.askDefault("Folder", folderDefault)
	if err != nil {
		return err
	}
	mainFile, err := p.askDefault("Main file", "main.tex")
	if err != nil {
		return err
	}
	author, err := p.askDefault("Author", "")
	if err != nil {
		return err
	}
	date, err := p.askDefault("Date", time.Now().Format("2006-01-02"))
	if err != nil {
		return err
	}
	printArticleInitReferences(cfg.Out, articles)
	category, err := p.askCSV("Category path, comma separated")
	if err != nil {
		return err
	}
	tags, err := p.askCSV("Tags, comma separated")
	if err != nil {
		return err
	}
	mainFig, err := p.askDefault("Main figure", "")
	if err != nil {
		return err
	}
	description, err := p.askMultiline("Description", "")
	if err != nil {
		return err
	}

	folder = strings.Trim(strings.TrimSpace(filepath.ToSlash(folder)), "/")
	mainFile = strings.Trim(strings.TrimSpace(filepath.ToSlash(mainFile)), "/")
	if folder == "" {
		return errors.New("folder cannot be empty")
	}
	if mainFile == "" {
		mainFile = "main.tex"
	}
	articleDir := resolveRootPath(cfg.RootDir, folder)
	mainPath := filepath.Join(articleDir, filepath.FromSlash(mainFile))
	if _, err := os.Stat(mainPath); err == nil {
		return fmt.Errorf("main file already exists: %s", mainPath)
	} else if !os.IsNotExist(err) {
		return err
	}
	if containsArticleFolder(articles, folder) {
		return fmt.Errorf("article folder already exists in %s: %s", cfg.ArticlesFile, folder)
	}
	if err := os.MkdirAll(filepath.Dir(mainPath), 0755); err != nil {
		return err
	}
	if err := os.WriteFile(mainPath, []byte(articleTemplate(title)), 0644); err != nil {
		return err
	}
	articles = append(articles, blog.Article{
		Title:       title,
		Description: description,
		Author:      author,
		Date:        date,
		Category:    category,
		Tags:        tags,
		Folder:      folder,
		MainFig:     mainFig,
		MainFile:    mainFile,
	})
	if err := blog.SaveArticles(articlesPath, articles); err != nil {
		return err
	}
	fmt.Fprintf(cfg.Out, "Created %s\n", filepath.ToSlash(mainPath))
	fmt.Fprintf(cfg.Out, "Updated %s\n", filepath.ToSlash(articlesPath))
	return nil
}

func printArticleInitReferences(out io.Writer, articles []blog.Article) {
	categories := existingCategoryPaths(articles)
	tags := existingTags(articles)
	if len(categories) == 0 && len(tags) == 0 {
		return
	}
	if len(categories) > 0 {
		fmt.Fprintf(out, "Existing categories: %s\n", strings.Join(categories, "; "))
	}
	if len(tags) > 0 {
		fmt.Fprintf(out, "Existing tags: %s\n", strings.Join(tags, ", "))
	}
}

func existingCategoryPaths(articles []blog.Article) []string {
	seen := map[string]bool{}
	for _, article := range articles {
		if article.Deleted || len(article.Category) == 0 {
			continue
		}
		path := strings.Join(article.Category, " / ")
		if path != "" {
			seen[path] = true
		}
	}
	return sortedKeys(seen)
}

func existingTags(articles []blog.Article) []string {
	seen := map[string]bool{}
	for _, article := range articles {
		if article.Deleted {
			continue
		}
		for _, tag := range article.Tags {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				seen[tag] = true
			}
		}
	}
	return sortedKeys(seen)
}

func sortedKeys(seen map[string]bool) []string {
	out := make([]string, 0, len(seen))
	for key := range seen {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

func RunArticleEdit(cfg ArticleCLIConfig) error {
	cfg = normalizeArticleCLIConfig(cfg)
	articlesPath := resolveRootPath(cfg.RootDir, cfg.ArticlesFile)
	articles, err := blog.LoadArticles(articlesPath)
	if err != nil {
		return err
	}
	if len(articles) == 0 {
		return fmt.Errorf("no articles in %s", articlesPath)
	}
	p := prompter{in: bufio.NewReader(cfg.In), out: cfg.Out}
	for i, article := range articles {
		fmt.Fprintf(cfg.Out, "%d. %s (%s)\n", i+1, article.Title, article.Folder)
	}
	idx, err := p.askIndex("Select article", len(articles))
	if err != nil {
		return err
	}
	a := articles[idx]
	if a.Title, err = p.askDefault("Title", a.Title); err != nil {
		return err
	}
	if a.Author, err = p.askDefault("Author", a.Author); err != nil {
		return err
	}
	if a.Date, err = p.askDefault("Date", a.Date); err != nil {
		return err
	}
	if a.Folder, err = p.askDefault("Folder", a.Folder); err != nil {
		return err
	}
	if a.MainFile, err = p.askDefault("Main file", defaultString(a.MainFile, "main.tex")); err != nil {
		return err
	}
	if a.MainFig, err = p.askDefault("Main figure", a.MainFig); err != nil {
		return err
	}
	if a.Category, err = p.askCSVDefault("Category path, comma separated", a.Category); err != nil {
		return err
	}
	if a.Tags, err = p.askCSVDefault("Tags, comma separated", a.Tags); err != nil {
		return err
	}
	if a.Description, err = p.askMultiline("Description", a.Description); err != nil {
		return err
	}
	articles[idx] = a
	if err := blog.SaveArticles(articlesPath, articles); err != nil {
		return err
	}
	fmt.Fprintf(cfg.Out, "Updated %s\n", filepath.ToSlash(articlesPath))
	return nil
}

func RunArticleDelete(cfg ArticleCLIConfig) error {
	cfg = normalizeArticleCLIConfig(cfg)
	articlesPath := resolveRootPath(cfg.RootDir, cfg.ArticlesFile)
	articles, err := blog.LoadArticles(articlesPath)
	if err != nil {
		return err
	}
	candidates := undeletedArticleIndexes(articles)
	if len(candidates) == 0 {
		return fmt.Errorf("no undeleted articles in %s", articlesPath)
	}
	p := prompter{in: bufio.NewReader(cfg.In), out: cfg.Out}
	for i, idx := range candidates {
		article := articles[idx]
		fmt.Fprintf(cfg.Out, "%d. %s (%s)\n", i+1, article.Title, article.Folder)
	}
	selected, err := p.askIndex("Select article to mark deleted", len(candidates))
	if err != nil {
		return err
	}
	idx := candidates[selected]
	articles[idx].Deleted = true
	if err := blog.SaveArticles(articlesPath, articles); err != nil {
		return err
	}
	fmt.Fprintf(cfg.Out, "Marked deleted: %s\n", articles[idx].Title)
	fmt.Fprintf(cfg.Out, "Updated %s\n", filepath.ToSlash(articlesPath))
	return nil
}

func undeletedArticleIndexes(articles []blog.Article) []int {
	indexes := make([]int, 0, len(articles))
	for i, article := range articles {
		if !article.Deleted {
			indexes = append(indexes, i)
		}
	}
	return indexes
}

type prompter struct {
	in  *bufio.Reader
	out io.Writer
}

func (p prompter) askRequired(label string) (string, error) {
	for {
		value, err := p.askDefault(label, "")
		if err != nil {
			return "", err
		}
		if value != "" {
			return value, nil
		}
		fmt.Fprintf(p.out, "%s is required.\n", label)
	}
}

func (p prompter) askDefault(label, def string) (string, error) {
	if def == "" {
		fmt.Fprintf(p.out, "%s: ", label)
	} else {
		fmt.Fprintf(p.out, "%s [%s]: ", label, def)
	}
	line, err := p.in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	value := strings.TrimSpace(line)
	if value == "" {
		return def, nil
	}
	return value, nil
}

func (p prompter) askCSV(label string) ([]string, error) {
	value, err := p.askDefault(label, "")
	if err != nil {
		return nil, err
	}
	return splitCSV(value), nil
}

func (p prompter) askCSVDefault(label string, def []string) ([]string, error) {
	value, err := p.askDefault(label, strings.Join(def, ", "))
	if err != nil {
		return nil, err
	}
	return splitCSV(value), nil
}

func (p prompter) askMultiline(label, def string) (string, error) {
	if def == "" {
		fmt.Fprintf(p.out, "%s (finish with a blank line):\n", label)
	} else {
		fmt.Fprintf(p.out, "%s (finish with a blank line; blank input keeps current value):\n%s\n", label, def)
	}
	var lines []string
	for {
		line, err := p.in.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return "", err
		}
		if err != nil && errors.Is(err, io.EOF) && line == "" {
			if len(lines) == 0 {
				return def, nil
			}
			return normalizeMultilineInput(strings.Join(lines, "")), nil
		}
		if strings.TrimSpace(line) == "" {
			if len(lines) == 0 {
				return def, nil
			}
			return normalizeMultilineInput(strings.Join(lines, "")), nil
		}
		lines = append(lines, line)
		if errors.Is(err, io.EOF) {
			return normalizeMultilineInput(strings.Join(lines, "")), nil
		}
	}
}

func (p prompter) askIndex(label string, max int) (int, error) {
	for {
		value, err := p.askDefault(label, "1")
		if err != nil {
			return 0, err
		}
		n, err := strconv.Atoi(value)
		if err == nil && n >= 1 && n <= max {
			return n - 1, nil
		}
		fmt.Fprintf(p.out, "Enter a number between 1 and %d.\n", max)
	}
}

func normalizeArticleCLIConfig(cfg ArticleCLIConfig) ArticleCLIConfig {
	if cfg.RootDir == "" {
		cfg.RootDir = "."
	}
	if cfg.ArticlesFile == "" {
		cfg.ArticlesFile = "data/articles.toml"
	}
	if cfg.In == nil {
		cfg.In = os.Stdin
	}
	if cfg.Out == nil {
		cfg.Out = os.Stdout
	}
	return cfg
}

func resolveRootPath(root, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(root, filepath.FromSlash(path)))
}

func splitCSV(value string) []string {
	fields := strings.Split(value, ",")
	out := make([]string, 0, len(fields))
	for _, field := range fields {
		field = strings.TrimSpace(field)
		if field != "" {
			out = append(out, field)
		}
	}
	return out
}

func normalizeMultilineInput(value string) string {
	value = strings.ReplaceAll(value, "\r\n", "\n")
	value = strings.ReplaceAll(value, "\r", "\n")
	return strings.TrimSpace(value)
}

func containsArticleFolder(articles []blog.Article, folder string) bool {
	folder = strings.Trim(strings.TrimSpace(filepath.ToSlash(folder)), "/")
	for _, article := range articles {
		if strings.Trim(strings.TrimSpace(filepath.ToSlash(article.Folder)), "/") == folder {
			return true
		}
	}
	return false
}

func articleTemplate(title string) string {
	return "\\begin{document}\n\n\\title{" + escapeLatexText(title) + "}\n\n\\section{Introduction}\n\n\n\\end{document}\n"
}

func defaultString(value, def string) string {
	if strings.TrimSpace(value) == "" {
		return def
	}
	return value
}

func escapeLatexText(s string) string {
	replacer := strings.NewReplacer(
		`\`, `\textbackslash{}`,
		`{`, `\{`,
		`}`, `\}`,
		`%`, `\%`,
		`&`, `\&`,
		`_`, `\_`,
		`#`, `\#`,
	)
	return replacer.Replace(s)
}
