// Package search is the Stage-1 namespace for URL scraping, issue search,
// and research agent types. See ../REFACTOR_PLAN.md.
package search

import "github.com/GrayCodeAI/hawk/internal/engine"

type (
	URLScraper      = engine.URLScraper
	ScrapeResult    = engine.ScrapeResult
	Issue           = engine.Issue
	SimilarIssue    = engine.SimilarIssue
	IssueIndex      = engine.IssueIndex
	ResearchAgent   = engine.ResearchAgent
	ResearchQuery   = engine.ResearchQuery
	ResearchResult  = engine.ResearchResult
	ResearchFinding = engine.ResearchFinding
)

func NewURLScraper() *URLScraper                     { return engine.NewURLScraper() }
func NewIssueIndex() *IssueIndex                     { return engine.NewIssueIndex() }
func NewResearchAgent(maxWorkers int) *ResearchAgent { return engine.NewResearchAgent(maxWorkers) }
func ExtractHTML(body string) (string, string)       { return engine.ExtractHTML(body) }
func ExtractJSON(body string) string                 { return engine.ExtractJSON(body) }
func ExtractMarkdown(body string) string             { return engine.ExtractMarkdown(body) }
func ExtractCode(body, rawURL string) string         { return engine.ExtractCode(body, rawURL) }
func SuggestResolution(s []*SimilarIssue) string     { return engine.SuggestResolution(s) }
func FormatIssueResults(s []*SimilarIssue) string    { return engine.FormatIssueResults(s) }
func BuildSearchContext(s []*SimilarIssue) string    { return engine.BuildSearchContext(s) }
