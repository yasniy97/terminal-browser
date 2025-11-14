// Terminal Browser with pagination (main.go)
// Go 1.23 compatible
//
// Features:
//  - Green terminal text (ANSI).
//  - Omnibar prompt "omnibar>": enter URLs, search queries, "open N", "F" (forward), "B" (back), "exit".
//  - Non-URL inputs perform DuckDuckGo searches; results are parsed and stored.
//  - Search results are paginated: pageSize (10) rows per page, navigate with F/B.
//  - open N opens the Nth result on the CURRENT page.
//  - Uses golang.org/x/net/html for robust HTML parsing.
//
// Usage:
//   go mod init terminal-browser
//   go get golang.org/x/net/html@latest
//   go run main.go

package main

import (
	"bufio"
	"fmt"
	stdhtml "html"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	nhtml "golang.org/x/net/html"
)

// ANSI color codes for green text
const (
	ansiGreen = "\033[32m"
	ansiReset = "\033[0m"
)

// Config constants
const (
	userAgent        = "TerminalBrowser/1.0 (+https://example.local/terminal-browser)"
	searchEngineHTML = "https://duckduckgo.com/html/?q="
	// maximum number of parsed search results to keep in memory (adjustable)
	maxSearchResults = 200
	// rows per page
	pageSize = 10
	httpTimeout = 15 * time.Second
)

// lastResults holds the most recent search results for pagination and `open N`.
var lastResults []searchResult
var currentPage int // 0-based page index

func main() {
	printHeader()

	reader := bufio.NewReader(os.Stdin)

	for {
		fmt.Print(ansiGreen + "omnibar> " + ansiReset)
		line, err := reader.ReadString('\n')
		if err != nil {
			if err == io.EOF {
				fmt.Println("\nbye.")
				return
			}
			fmt.Fprintln(os.Stderr, "read error:", err)
			return
		}

		input := strings.TrimSpace(line)
		if input == "" {
			continue
		}
		lower := strings.ToLower(input)

		// Quit commands
		if lower == "exit" || lower == "quit" {
			fmt.Println("Goodbye.")
			return
		}

		// Navigation commands: F forward, B back (single-letter)
		if strings.EqualFold(input, "F") || strings.EqualFold(input, "f") {
			if len(lastResults) == 0 {
				fmt.Fprintln(os.Stderr, "No search results to navigate. Run a search first.")
				continue
			}
			lastPage := (len(lastResults)-1)/pageSize
			if currentPage >= lastPage {
				fmt.Fprintln(os.Stderr, "Already on the last page. Cannot go forward.")
				continue
			}
			currentPage++
			printCurrentPage()
			continue
		}
		if strings.EqualFold(input, "B") || strings.EqualFold(input, "b") {
			if len(lastResults) == 0 {
				fmt.Fprintln(os.Stderr, "No search results to navigate. Run a search first.")
				continue
			}
			if currentPage == 0 {
				fmt.Fprintln(os.Stderr, "Already on the first page. Cannot go back.")
				continue
			}
			currentPage--
			printCurrentPage()
			continue
		}

		// open N command handling (N is 1..pageSize on the current page)
		if parts := strings.Fields(input); len(parts) == 2 && strings.EqualFold(parts[0], "open") {
			n, err := strconv.Atoi(parts[1])
			if err != nil || n <= 0 {
				fmt.Fprintln(os.Stderr, "Usage: open <N>   (N must be a positive integer)")
				continue
			}
			if len(lastResults) == 0 {
				fmt.Fprintln(os.Stderr, "No previous search results. Run a search first.")
				continue
			}
			// compute global index for the requested item on current page
			globalIdx := currentPage*pageSize + (n - 1)
			if globalIdx < 0 || globalIdx >= len(lastResults) {
				// out of range for current page
				// provide clearer hint
				pageStart := currentPage*pageSize + 1
				pageEnd := pageStart + pageSize - 1
				if pageEnd > len(lastResults) {
					pageEnd = len(lastResults)
				}
				fmt.Fprintf(os.Stderr, "Invalid choice. On this page choose N between 1 and %d (showing results %d..%d of %d).\n",
					pageEnd-pageStart+1, pageStart, pageEnd, len(lastResults))
				continue
			}
			target := lastResults[globalIdx].URL
			if !strings.HasPrefix(strings.ToLower(target), "http://") && !strings.HasPrefix(strings.ToLower(target), "https://") {
				target = "http://" + target
			}
			if err := handleFetch(target); err != nil {
				fmt.Fprintf(os.Stderr, "open error: %v\n", err)
			}
			continue
		}

		// If looks like URL, fetch
		if isURLLike(input) {
			u := input
			if !strings.HasPrefix(strings.ToLower(u), "http://") && !strings.HasPrefix(strings.ToLower(u), "https://") {
				u = "http://" + u
			}
			if err := handleFetch(u); err != nil {
				fmt.Fprintf(os.Stderr, "fetch error: %v\n", err)
			}
			// clear lastResults (we didn't change pagination on fetch)
			continue
		}

		// Otherwise perform search
		if err := handleSearch(input); err != nil {
			fmt.Fprintf(os.Stderr, "search error: %v\n", err)
		}
	}
}

// printHeader prints a small green header describing the program.
func printHeader() {
	fmt.Println(ansiGreen + "Terminal Browser — omnibar (enter URL or search). Type 'exit' to quit." + ansiReset)
	fmt.Println(ansiGreen + "Search results are paginated: 10 rows per page. Use 'F' for forward, 'B' for back." + ansiReset)
	fmt.Println(ansiGreen + "Use 'open N' to open the Nth result on the current page." + ansiReset)
	fmt.Println()
}

// isURLLike returns true if input starts with http or https, or looks like domain (contains dot and no spaces).
func isURLLike(s string) bool {
	ls := strings.ToLower(strings.TrimSpace(s))
	if strings.HasPrefix(ls, "http:") || strings.HasPrefix(ls, "https:") {
		return true
	}
	if strings.Contains(s, " ") {
		return false
	}
	if strings.Contains(s, ".") {
		return true
	}
	return false
}

// handleFetch fetches the URL and prints a parsed text representation.
func handleFetch(rawURL string) error {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	fmt.Println()
	fmt.Println(ansiGreen + "Fetching: " + parsed.String() + ansiReset)

	client := &http.Client{Timeout: httpTimeout}
	req, err := http.NewRequest("GET", parsed.String(), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	fmt.Printf(ansiGreen+"HTTP %s — %d %s\n\n"+ansiReset, parsed.Scheme, resp.StatusCode, resp.Status)

	// Read body (with size guard)
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024)) // limit 5MB
	if err != nil {
		return err
	}
	body := string(bodyBytes)

	// Parse title using html parser (nhtml) and stdhtml for unescape
	title := extractTitleFromHTML(body)
	if title != "" {
		fmt.Println(ansiGreen + "Title: " + ansiReset + title)
		fmt.Println()
	}

	// Convert HTML to text using parser
	text := htmlToText(body)

	// Print a reasonable amount for terminal
	maxChars := 20_000
	if len(text) > maxChars {
		text = text[:maxChars] + "\n\n[output truncated]"
	}

	// print green text
	fmt.Println(ansiGreen + text + ansiReset)
	fmt.Println()
	return nil
}

// handleSearch performs a DuckDuckGo search and stores results, then prints first page.
func handleSearch(query string) error {
	fmt.Println()
	fmt.Println(ansiGreen + "Searching for: " + ansiReset + query)
	searchURL := searchEngineHTML + url.QueryEscape(query)

	client := &http.Client{Timeout: httpTimeout}
	req, err := http.NewRequest("GET", searchURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("search engine returned %d %s", resp.StatusCode, resp.Status)
	}
	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024)) // 2MB limit
	if err != nil {
		return err
	}
	body := string(bodyBytes)

	results := parseSearchResultsWithHTML(body)
	if len(results) == 0 {
		lastResults = nil
		currentPage = 0
		fmt.Println(ansiGreen + "[no results found]" + ansiReset)
		return nil
	}

	// Cap results to maxSearchResults
	if len(results) > maxSearchResults {
		results = results[:maxSearchResults]
	}

	lastResults = results
	currentPage = 0
	printCurrentPage()
	return nil
}

// printCurrentPage prints the current page of lastResults with page indicator and tips.
func printCurrentPage() {
	total := len(lastResults)
	if total == 0 {
		fmt.Println(ansiGreen + "[no results]" + ansiReset)
		return
	}
	lastPage := (total-1)/pageSize
	start := currentPage*pageSize
	end := start + pageSize
	if end > total {
		end = total
	}
	// header
	fmt.Println()
	fmt.Printf(ansiGreen+"Search results — Page %d/%d  (showing results %d..%d of %d)\n"+ansiReset,
		currentPage+1, lastPage+1, start+1, end, total)
	fmt.Println()

	// print rows numbered 1..(end-start)
	for i := start; i < end; i++ {
		rowNum := i - start + 1 // 1-based on current page
		fmt.Printf(ansiGreen+"%d) %s\n   %s\n\n"+ansiReset, rowNum, lastResults[i].Title, lastResults[i].URL)
	}

	// tips
	tips := []string{"Type 'open N' to open the Nth item on this page."}
	if currentPage > 0 {
		tips = append(tips, "B to go Back")
	}
	if currentPage < lastPage {
		tips = append(tips, "F to go Forward")
	}
	tips = append(tips, "Or paste a URL at the omnibar.")
	fmt.Println(ansiGreen + strings.Join(tips, " | ") + ansiReset)
	fmt.Println()
}

// extractTitleFromHTML uses the parser to find the first <title> node text.
func extractTitleFromHTML(htmlBody string) string {
	doc, err := nhtml.Parse(strings.NewReader(htmlBody))
	if err != nil {
		return ""
	}
	var title string
	var f func(*nhtml.Node)
	f = func(n *nhtml.Node) {
		if n.Type == nhtml.ElementNode && strings.EqualFold(n.Data, "title") {
			title = nodeText(n)
			return
		}
		for c := n.FirstChild; c != nil && title == ""; c = c.NextSibling {
			f(c)
		}
	}
	f(doc)
	title = strings.TrimSpace(stdhtml.UnescapeString(title))
	return title
}

// htmlToText converts HTML to readable plain text using the html parser.
// It skips <script>, <style>, <noscript> and comments. It preserves newlines
// for block elements for readability.
func htmlToText(htmlBody string) string {
	doc, err := nhtml.Parse(strings.NewReader(htmlBody))
	if err != nil {
		// fallback: return decoded raw text
		return stdhtml.UnescapeString(stripTagsFallback(htmlBody))
	}

	var b strings.Builder
	var render func(*nhtml.Node)
	render = func(n *nhtml.Node) {
		if n.Type == nhtml.TextNode {
			// write text, trimming only leading/trailing spaces at node level
			text := strings.TrimSpace(n.Data)
			if text != "" {
				// ensure spacing
				if b.Len() > 0 {
					last := b.String()[b.Len()-1]
					if last != '\n' && last != ' ' {
						b.WriteString(" ")
					}
				}
				b.WriteString(stdhtml.UnescapeString(text))
			}
			return
		}
		if n.Type == nhtml.ElementNode {
			// Skip content of these tags entirely
			switch strings.ToLower(n.Data) {
			case "script", "style", "noscript", "iframe", "svg":
				return
			}
			// Before certain block-level tags, add newline to separate blocks
			switch strings.ToLower(n.Data) {
			case "p", "div", "section", "article", "header", "footer", "aside",
				"h1", "h2", "h3", "h4", "h5", "h6", "br", "li", "tr", "table":
				// add a newline if previous char isn't already newline
				if b.Len() > 0 {
					last := b.String()[b.Len()-1]
					if last != '\n' {
						b.WriteString("\n")
					}
				}
			}
			// Traverse children
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				render(c)
			}
			// After block-level tags, ensure a newline
			switch strings.ToLower(n.Data) {
			case "p", "div", "section", "article", "header", "footer", "aside",
				"h1", "h2", "h3", "h4", "h5", "h6", "li", "tr", "table":
				if b.Len() > 0 {
					last := b.String()[b.Len()-1]
					if last != '\n' {
						b.WriteString("\n")
					}
				}
			}
		}
		// ignore comments and other node types
	}

	// Start from <body> if present to avoid titles/meta duplication
	var bodyNode *nhtml.Node
	var findBody func(*nhtml.Node)
	findBody = func(n *nhtml.Node) {
		if n.Type == nhtml.ElementNode && strings.EqualFold(n.Data, "body") {
			bodyNode = n
			return
		}
		for c := n.FirstChild; c != nil && bodyNode == nil; c = c.NextSibling {
			findBody(c)
		}
	}
	findBody(doc)
	if bodyNode != nil {
		render(bodyNode)
	} else {
		render(doc)
	}

	// Post-process: collapse multiple blank lines, trim edges
	out := b.String()
	// Replace \r\n with \n
	out = strings.ReplaceAll(out, "\r\n", "\n")
	// Collapse more than 2 newlines into 2
	for strings.Contains(out, "\n\n\n") {
		out = strings.ReplaceAll(out, "\n\n\n", "\n\n")
	}
	out = strings.TrimSpace(out)
	return out
}

// nodeText returns all descendant text for a node (simple helper).
func nodeText(n *nhtml.Node) string {
	if n == nil {
		return ""
	}
	var b strings.Builder
	var f func(*nhtml.Node)
	f = func(nd *nhtml.Node) {
		if nd.Type == nhtml.TextNode {
			b.WriteString(nd.Data)
		}
		for c := nd.FirstChild; c != nil; c = c.NextSibling {
			f(c)
		}
	}
	f(n)
	return b.String()
}

// stripTagsFallback is used only if the HTML parser fails; very small fallback.
func stripTagsFallback(s string) string {
	// remove <> blocks crudely
	var b strings.Builder
	inTag := false
	for _, r := range s {
		switch r {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				b.WriteRune(r)
			}
		}
	}
	return b.String()
}

// searchResult holds a parsed search result
type searchResult struct {
	Title string
	URL   string
}

// parseSearchResultsWithHTML parses HTML search results using the HTML parser.
// It finds <a> anchors with href and text, filters duplicates, and returns results.
func parseSearchResultsWithHTML(htmlBody string) []searchResult {
	doc, err := nhtml.Parse(strings.NewReader(htmlBody))
	if err != nil {
		return nil
	}
	results := make([]searchResult, 0, maxSearchResults)
	seen := map[string]bool{}

	// traverse anchor nodes in document order
	var visit func(*nhtml.Node)
	visit = func(n *nhtml.Node) {
		if n.Type == nhtml.ElementNode && strings.EqualFold(n.Data, "a") {
			// get href
			var href string
			for _, a := range n.Attr {
				if strings.EqualFold(a.Key, "href") {
					href = strings.TrimSpace(a.Val)
					break
				}
			}
			if href != "" {
				// get visible text
				txt := strings.TrimSpace(nodeText(n))
				txt = strings.Join(strings.Fields(txt), " ") // collapse whitespace
				// sanitize href: handle ddg redirect pattern /l/?uddg=...
				if strings.HasPrefix(href, "/l/?") || strings.Contains(href, "uddg=") {
					// try extract uddg param
					if u, err := url.Parse(href); err == nil {
						if q := u.Query().Get("uddg"); q != "" {
							if decoded, err2 := url.QueryUnescape(q); err2 == nil {
								href = decoded
							}
						}
					}
				}
				// make relative href absolute for duckduckgo domain
				if strings.HasPrefix(href, "/") {
					href = "https://duckduckgo.com" + href
				}
				// skip javascript or anchors
				if strings.HasPrefix(strings.ToLower(href), "javascript:") || strings.HasPrefix(href, "#") {
					// skip
				} else {
					href = sanitizeURL(href)
					if txt == "" {
						txt = href
					}
					// dedupe
					if !seen[href] {
						seen[href] = true
						results = append(results, searchResult{Title: stdhtml.UnescapeString(txt), URL: href})
					}
				}
			}
		}
		// continue traversal
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	visit(doc)

	// Heuristic: prefer non-ddg links first
	sort.SliceStable(results, func(i, j int) bool {
		a := strings.ToLower(results[i].URL)
		b := strings.ToLower(results[j].URL)
		score := func(u string) int {
			if strings.Contains(u, "duckduckgo.com") || strings.Contains(u, "google.com") || strings.Contains(u, "bing.com") {
				return 0
			}
			if strings.HasPrefix(u, "https://") {
				return 2
			}
			if strings.HasPrefix(u, "http://") {
				return 1
			}
			return 0
		}
		return score(a) > score(b)
	})

	return results
}

// sanitizeURL removes a few common tracking query params for display.
func sanitizeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	tracking := []string{"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "gclid", "fbclid"}
	q := u.Query()
	changed := false
	for _, k := range tracking {
		if q.Get(k) != "" {
			q.Del(k)
			changed = true
		}
	}
	if changed {
		u.RawQuery = q.Encode()
	}
	return u.String()
}
