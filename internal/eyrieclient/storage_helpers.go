package eyrieclient

import "github.com/GrayCodeAI/eyrie/storage"

type (
	DAG     = storage.DAG
	DAGNode = storage.DAGNode
)

func NewDAG(dbPath string, sessionID string) (*DAG, error) {
	return storage.NewDAG(dbPath, sessionID)
}
