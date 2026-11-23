package integrationtools

import (
	"strings"
	"unicode/utf8"

	xhtml "golang.org/x/net/html"
)

// readableContent is the result of the main-content extractor.
type readableContent struct {
	text   string // visible text, without the title
	title  string
	html   string // cleaned subtree, used by the markdown converter
	source string // "main" when a content root was found, "text" for a body fallback
	ok     bool
}

// extractReadableContent selects the main content root, removes chrome and
// hidden nodes, and renders readable text. It falls back (ok=false) when the
// result is suspiciously small, so the caller can use the legacy extractor.
func extractReadableContent(htmlStr string) readableContent {
	doc, err := xhtml.Parse(strings.NewReader(htmlStr))
	if err != nil {
		return readableContent{}
	}

	root, selected := selectContentRoot(doc)
	if root == nil {
		return readableContent{}
	}

	title := readableTitle(doc, root)
	cleanContentTree(root)
	text := renderReadableText(root)
	if strings.TrimSpace(text) == "" {
		return readableContent{}
	}

	mainLen := utf8.RuneCountInString(text)
	legacyLen := utf8.RuneCountInString(legacyExtractText(htmlStr))
	if mainLen < 500 {
		return readableContent{}
	}
	if selected && legacyLen > 0 && mainLen*5 < legacyLen {
		return readableContent{}
	}

	source := "text"
	if selected {
		source = "main"
	}
	return readableContent{
		text:   text,
		title:  title,
		html:   renderReadableHTML(root),
		source: source,
		ok:     true,
	}
}

func selectContentRoot(doc *xhtml.Node) (*xhtml.Node, bool) {
	// MediaWiki exposes the article body under this id; it is more precise than
	// the surrounding main/content wrappers.
	if n := findElementByID(doc, "mw-content-text"); n != nil {
		return n, true
	}
	// Prefer the semantic article when present: main often wraps the article
	// plus related cards, while article is just the body.
	if n := largestElementByTag(doc, "article"); n != nil {
		return n, true
	}
	if n := largestElementByTag(doc, "main"); n != nil {
		return n, true
	}
	if n := findElementByRole(doc, "main"); n != nil {
		return n, true
	}
	if n := findElementByID(doc, "content"); n != nil {
		return n, true
	}
	if n := findFirstElement(doc, "body"); n != nil {
		return n, false
	}
	return doc, false
}

func findFirstElement(root *xhtml.Node, tag string) *xhtml.Node {
	var found *xhtml.Node
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if found != nil {
			return
		}
		if n.Type == xhtml.ElementNode && strings.EqualFold(n.Data, tag) {
			found = n
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return found
}

func findElementByID(root *xhtml.Node, id string) *xhtml.Node {
	var found *xhtml.Node
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if found != nil {
			return
		}
		if n.Type == xhtml.ElementNode && strings.EqualFold(attrValue(n, "id"), id) {
			found = n
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return found
}

func findElementByRole(root *xhtml.Node, role string) *xhtml.Node {
	var found *xhtml.Node
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if found != nil {
			return
		}
		if n.Type == xhtml.ElementNode && strings.EqualFold(attrValue(n, "role"), role) {
			found = n
			return
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return found
}

func largestElementByTag(root *xhtml.Node, tag string) *xhtml.Node {
	var best *xhtml.Node
	bestLen := 0
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && strings.EqualFold(n.Data, tag) {
			if size := utf8.RuneCountInString(nodeText(n)); size > bestLen {
				best = n
				bestLen = size
			}
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(root)
	return best
}

func readableTitle(doc, root *xhtml.Node) string {
	if n := findFirstElement(doc, "title"); n != nil {
		if title := collapseWhitespace(nodeText(n)); title != "" {
			return title
		}
	}
	if n := findFirstElement(root, "h1"); n != nil {
		return collapseWhitespace(nodeText(n))
	}
	return ""
}

func cleanContentTree(root *xhtml.Node) {
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		for child := n.FirstChild; child != nil; {
			next := child.NextSibling
			if shouldDropContentNode(child) {
				n.RemoveChild(child)
			} else {
				walk(child)
			}
			child = next
		}
	}
	walk(root)
}

func shouldDropContentNode(n *xhtml.Node) bool {
	if n == nil || n.Type != xhtml.ElementNode {
		return false
	}
	tag := strings.ToLower(n.Data)
	switch tag {
	case "script", "style", "noscript", "nav", "footer", "aside", "form",
		"svg", "iframe", "button", "select", "input", "textarea", "canvas",
		"template", "link", "meta":
		return true
	case "header":
		if strings.EqualFold(attrValue(n, "role"), "banner") {
			return true
		}
		if classOrIDContains(n, "site-header", "masthead", "global-header", "top-bar") {
			return true
		}
		// An article header may hold the title and byline; keep it unless it
		// contains site navigation.
		if hasDescendantTag(n, "nav") {
			return true
		}
	case "sup":
		if hasClassToken(n, "reference") || hasClassToken(n, "mw-ref") {
			return true
		}
	case "table":
		if hasClassToken(n, "infobox") || hasClassToken(n, "navbox") ||
			hasClassToken(n, "vertical-navbox") {
			return true
		}
	}

	if hasAttr(n, "hidden") || strings.EqualFold(attrValue(n, "aria-hidden"), "true") ||
		hasAttr(n, "data-nosnippet") {
		return true
	}
	style := strings.ToLower(attrValue(n, "style"))
	if strings.Contains(style, "display:none") || strings.Contains(style, "display: none") ||
		strings.Contains(style, "visibility:hidden") || strings.Contains(style, "visibility: hidden") {
		return true
	}
	switch strings.ToLower(strings.TrimSpace(attrValue(n, "role"))) {
	case "navigation", "banner", "contentinfo", "complementary", "search", "dialog", "alertdialog":
		return true
	}

	// Exact class/id tokens used by common site chrome.
	for _, token := range []string{
		"toc", "vector-toc", "table-of-contents", "sidebar", "navbox", "infobox",
		"metadata", "noprint", "printfooter", "catlinks", "mw-editsection",
		"mw-jump-link", "bio", "signin", "sign-in", "login", "author-bio",
		"authorbox", "author-box", "about-author", "author-profile", "next",
	} {
		if hasClassToken(n, token) || strings.EqualFold(attrValue(n, "id"), token) {
			return true
		}
	}
	// Prefix/substring matches for banners and widgets.
	if classOrIDContains(n, "cookie", "consent", "newsletter", "subscribe", "advert",
		"ad-slot", "advertisement", "social-share", "share-buttons", "breadcrumb",
		"pagination", "comment-section", "related-articles", "related-posts",
		"author-bio", "author_bio", "authorbio", "author-box", "about-author",
		"author-profile", "author-details", "google-preferred", "preferred-source",
		"google-discover", "follow-google", "add-google", "article-actions",
		"social-actions", "login-prompt", "login-panel", "google-addus",
		"article-cta", "login-button", "user-box", "header-user", "with-excerpt",
		"navbox", "next-article") {
		return true
	}
	return false
}

func hasAttr(n *xhtml.Node, key string) bool {
	for _, attr := range n.Attr {
		if strings.EqualFold(attr.Key, key) {
			return true
		}
	}
	return false
}

func hasDescendantTag(n *xhtml.Node, tag string) bool {
	found := false
	var walk func(*xhtml.Node)
	walk = func(node *xhtml.Node) {
		if found || node == nil {
			return
		}
		if node.Type == xhtml.ElementNode && strings.EqualFold(node.Data, tag) {
			found = true
			return
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(n)
	return found
}

func classOrIDContains(n *xhtml.Node, tokens ...string) bool {
	value := strings.ToLower(attrValue(n, "id") + " " + attrValue(n, "class"))
	if strings.TrimSpace(value) == "" {
		return false
	}
	for _, token := range tokens {
		if strings.Contains(value, strings.ToLower(token)) {
			return true
		}
	}
	return false
}

var readableBlockTags = map[string]bool{
	"address": true, "article": true, "blockquote": true, "dd": true, "div": true,
	"dl": true, "dt": true, "figcaption": true, "figure": true, "h1": true,
	"h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "header": true,
	"hr": true, "li": true, "main": true, "ol": true, "p": true, "pre": true,
	"section": true, "table": true, "tbody": true, "td": true, "tfoot": true,
	"th": true, "thead": true, "tr": true, "ul": true,
}

type readableTextBuilder struct {
	b    strings.Builder
	last byte
}

func (c *readableTextBuilder) write(text string) {
	text = collapseWhitespace(text)
	if text == "" {
		return
	}
	if c.b.Len() > 0 && c.last != 0 && c.last != '\n' && c.last != ' ' {
		c.b.WriteByte(' ')
	}
	c.b.WriteString(text)
	c.last = text[len(text)-1]
}

func (c *readableTextBuilder) newline() {
	if c.b.Len() > 0 && c.last != '\n' {
		c.b.WriteByte('\n')
		c.last = '\n'
	}
}

func (c *readableTextBuilder) writeRaw(text string) {
	text = strings.TrimSpace(text)
	if text == "" {
		return
	}
	c.newline()
	c.b.WriteString(text)
	c.last = '\n'
	c.newline()
}

func renderReadableText(root *xhtml.Node) string {
	var builder readableTextBuilder
	var walk func(*xhtml.Node)
	walk = func(n *xhtml.Node) {
		if n == nil {
			return
		}
		if n.Type == xhtml.TextNode {
			builder.write(n.Data)
			return
		}
		if n.Type != xhtml.ElementNode {
			return
		}
		tag := strings.ToLower(n.Data)
		if tag == "br" {
			builder.newline()
			return
		}
		if tag == "pre" {
			builder.writeRaw(nodeText(n))
			return
		}
		block := readableBlockTags[tag]
		if block {
			builder.newline()
		}
		for child := n.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
		if block {
			builder.newline()
		}
	}
	walk(root)

	lines := strings.Split(builder.b.String(), "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(collapseWhitespace(line))
		if line != "" {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func renderReadableHTML(root *xhtml.Node) string {
	var b strings.Builder
	if err := xhtml.Render(&b, root); err != nil {
		return ""
	}
	return b.String()
}

func prependTitle(title, text string) string {
	title = strings.TrimSpace(title)
	text = strings.TrimSpace(text)
	if title == "" {
		return text
	}
	if text == "" {
		return title
	}
	normalized := strings.TrimLeft(text, "# \n\t")
	if strings.HasPrefix(strings.ToLower(normalized), strings.ToLower(title)) {
		return text
	}
	return title + "\n\n" + text
}

func legacyExtractText(htmlContent string) string {
	result := reScript.ReplaceAllLiteralString(htmlContent, "")
	result = reStyle.ReplaceAllLiteralString(result, "")
	result = reTags.ReplaceAllLiteralString(result, "")

	result = strings.TrimSpace(result)

	result = reWhitespace.ReplaceAllString(result, " ")
	result = reBlankLines.ReplaceAllString(result, "\n\n")

	lines := strings.Split(result, "\n")
	var cleanLines []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			cleanLines = append(cleanLines, line)
		}
	}

	return strings.Join(cleanLines, "\n")
}
