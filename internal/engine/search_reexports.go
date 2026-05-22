package engine

import "github.com/GrayCodeAI/hawk/internal/engine/search"

type URLScraper = search.URLScraper
type ScrapeResult = search.ScrapeResult
type Issue = search.Issue
type SimilarIssue = search.SimilarIssue
type IssueIndex = search.IssueIndex
type ResearchAgent = search.ResearchAgent
type ResearchQuery = search.ResearchQuery
type ResearchResult = search.ResearchResult
type ResearchFinding = search.ResearchFinding

var NewURLScraper = search.NewURLScraper
var NewIssueIndex = search.NewIssueIndex
var NewResearchAgent = search.NewResearchAgent
var ExtractHTML = search.ExtractHTML
var ExtractJSON = search.ExtractJSON
var ExtractMarkdown = search.ExtractMarkdown
var ExtractCode = search.ExtractCode
var SuggestResolution = search.SuggestResolution
var FormatIssueResults = search.FormatIssueResults
var BuildSearchContext = search.BuildSearchContext
