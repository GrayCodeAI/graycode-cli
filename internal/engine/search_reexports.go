package engine

import "github.com/GrayCodeAI/hawk/internal/engine/search"

type (
	URLScraper      = search.URLScraper
	ScrapeResult    = search.ScrapeResult
	Issue           = search.Issue
	SimilarIssue    = search.SimilarIssue
	IssueIndex      = search.IssueIndex
	ResearchAgent   = search.ResearchAgent
	ResearchQuery   = search.ResearchQuery
	ResearchResult  = search.ResearchResult
	ResearchFinding = search.ResearchFinding
)

var (
	NewURLScraper      = search.NewURLScraper
	NewIssueIndex      = search.NewIssueIndex
	NewResearchAgent   = search.NewResearchAgent
	ExtractHTML        = search.ExtractHTML
	ExtractJSON        = search.ExtractJSON
	ExtractMarkdown    = search.ExtractMarkdown
	ExtractCode        = search.ExtractCode
	SuggestResolution  = search.SuggestResolution
	FormatIssueResults = search.FormatIssueResults
	BuildSearchContext = search.BuildSearchContext
)
