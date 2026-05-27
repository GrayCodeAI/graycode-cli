//go:build !cgo

package codegraph

import (
	"database/sql"
	"fmt"
	"sync"
)

type CodeGraph struct {
	db   *sql.DB
	mu   sync.RWMutex
	root string
}

type Node struct {
	ID            string `json:"id"`
	Kind          string `json:"kind"`
	Name          string `json:"name"`
	QualifiedName string `json:"qualified_name"`
	FilePath      string `json:"file_path"`
	Language      string `json:"language"`
	StartLine     int    `json:"start_line"`
	EndLine       int    `json:"end_line"`
	Signature     string `json:"signature"`
	Docstring     string `json:"docstring"`
	Visibility    string `json:"visibility"`
	IsExported    bool   `json:"is_exported"`
}

type Edge struct {
	ID       int    `json:"id"`
	Source   string `json:"source"`
	Target   string `json:"target"`
	Kind     string `json:"kind"`
	Line     int    `json:"line"`
	Metadata string `json:"metadata"`
}

type LanguageExtractor struct{}

type UnresolvedRef struct {
	FromNodeID    string
	ReferenceName string
	ReferenceKind string
	Line          int
	FilePath      string
	Language      string
}

func Open(root string) (*CodeGraph, error) {
	return nil, ErrNoCGO
}

func (cg *CodeGraph) Close() error                                   { return nil }
func (cg *CodeGraph) IndexFile(filePath string) error                { return ErrNoCGO }
func (cg *CodeGraph) IndexDir(dir string) error                      { return ErrNoCGO }
func (cg *CodeGraph) Search(query string, limit int) ([]Node, error) { return nil, ErrNoCGO }

func (cg *CodeGraph) GetCallers(nodeID string, maxDepth int) ([]Node, error) { return nil, ErrNoCGO }

func (cg *CodeGraph) GetCallees(nodeID string, maxDepth int) ([]Node, error) { return nil, ErrNoCGO }

func (cg *CodeGraph) GetImpactRadius(nodeID string, maxDepth int) ([]Node, error) {
	return nil, ErrNoCGO
}

func (cg *CodeGraph) BuildContext(query string, maxNodes int) (string, error) { return "", ErrNoCGO }
func (cg *CodeGraph) ResolveRefs() error                                      { return ErrNoCGO }
func (cg *CodeGraph) Stats() (map[string]interface{}, error)                  { return nil, ErrNoCGO }

func (cg *CodeGraph) GetNode(id string) (Node, error) { return Node{}, ErrNoCGO }

func (cg *CodeGraph) Sync() (*SyncResult, error) { return nil, ErrNoCGO }

func (cg *CodeGraph) Trace(from, to string) ([]Node, error) { return nil, ErrNoCGO }

func (cg *CodeGraph) Explore(query string, maxFiles int) (*ExploreResult, error) {
	return nil, ErrNoCGO
}
func (cg *CodeGraph) Files(dirFilter string) ([]FileEntry, error) { return nil, ErrNoCGO }
func (cg *CodeGraph) Status() (*StatusResult, error)              { return nil, ErrNoCGO }
func (cg *CodeGraph) BetweennessCentrality(topN int) (*BetweennessResult, error) {
	return nil, ErrNoCGO
}

func (cg *CodeGraph) CommunityDetection() (*CommunityDetectionResult, error) { return nil, ErrNoCGO }

func (cg *CodeGraph) ConnectedComponents() ([][]string, error) { return nil, ErrNoCGO }

func (cg *CodeGraph) PageRank(iterations int, damping float64) (map[string]float64, error) {
	return nil, ErrNoCGO
}

func (cg *CodeGraph) ImpactAnalysis(nodeID string, maxDepth int) (*ImpactResult, error) {
	return nil, ErrNoCGO
}

func (cg *CodeGraph) AnalyzeCoupling(topN int) ([]CouplingMetric, error) { return nil, ErrNoCGO }

func (cg *CodeGraph) FindDeadCode() ([]DeadCodeEntry, error) { return nil, ErrNoCGO }

func (cg *CodeGraph) SemanticSearch(query string, limit int) ([]Node, error) { return nil, ErrNoCGO }

func (cg *CodeGraph) HybridSearch(query string, limit int) ([]Node, error) { return nil, ErrNoCGO }

func CrossRepoQuery(repos []string, query string, limit int) (map[string][]Node, error) {
	return nil, ErrNoCGO
}

func CrossRepoImpact(repos []string, symbol string, maxDepth int) (map[string]*ImpactResult, error) {
	return nil, ErrNoCGO
}
func FindCrossRepoCalls(repos []string) ([]CrossRepoCall, error) { return nil, ErrNoCGO }

type SyncResult struct {
	FilesChecked  int
	FilesAdded    int
	FilesModified int
	FilesRemoved  int
	NodesUpdated  int
	DurationMs    int
}

type ExploreResult struct {
	Files       map[string][]Node
	SourceLines map[string]string
}

type FileEntry struct {
	Path      string
	Language  string
	Size      int
	NodeCount int
	IndexedAt int
}

type StatusResult struct {
	ProjectRoot string
	DBPath      string
	DBSizeBytes int64
	Files       int
	Nodes       int
	Edges       int
	Unresolved  int
	NodesByKind map[string]int
	FilesByLang map[string]int
	JournalMode string
	UpToDate    bool
}

type BetweennessResult struct {
	Scores map[string]float64
	Top    []NodeCentrality
}

type NodeCentrality struct {
	NodeID   string
	Name     string
	FilePath string
	Score    float64
	Kind     string
}

type CommunityDetectionResult struct {
	Communities []Community
	Modularity  float64
}

type Community struct {
	ID    int
	Nodes []string
	Score float64
}

type ImpactResult struct {
	Root     string
	Impacted map[string]int
	Nodes    []Node
	MaxDepth int
}

type CouplingMetric struct {
	FileA      string
	FileB      string
	SharedDeps int
	Coupling   float64
}

type DeadCodeEntry struct {
	Node       Node
	Confidence float64
	Reason     string
}

type CrossRepoCall struct {
	FromRepo string
	ToRepo   string
	Symbol   string
	File     string
	Line     int
	Target   Node
}

type GraphDiff struct {
	AddedNodes   []string
	RemovedNodes []string
	AddedEdges   int
	RemovedEdges int
	Affected     []string
}

type Embedding struct {
	NodeID  string
	Vector  []float32
	Model   string
	Symbols []string
}

var ErrNoCGO = fmt.Errorf("codegraph requires CGO (build with CGO_ENABLED=1)")
