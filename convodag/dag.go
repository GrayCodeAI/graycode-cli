// Package convodag implements a DAG-based conversation store using SQLite.
// Instead of linear chat history, conversations are stored as a directed
// acyclic graph so users can fork from any point and explore alternatives.
package convodag

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "modernc.org/sqlite"
)

// Node is a single message in the conversation DAG.
type Node struct {
	ID        string
	ParentID  string // empty for root nodes
	Role      string // "user", "assistant", "system", "tool"
	Content   string
	Model     string
	CreatedAt time.Time
	Metadata  map[string]string
}

// DAG is a conversation stored as a directed acyclic graph backed by SQLite.
type DAG struct {
	db        *sql.DB
	sessionID string
}

// New opens or creates a conversation DAG for the given session.
func New(dbPath string, sessionID string) (*DAG, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}

	if err := createSchema(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}

	return &DAG{db: db, sessionID: sessionID}, nil
}

// Append adds a new node as a child of the given parent and advances the head pointer.
// Pass an empty parentID to create a root node.
func (d *DAG) Append(parentID string, role string, content string) (*Node, error) {
	if parentID != "" {
		exists, err := d.nodeExists(parentID)
		if err != nil {
			return nil, err
		}
		if !exists {
			return nil, fmt.Errorf("parent node %q not found", parentID)
		}
	}

	node := &Node{
		ID:        generateID(),
		ParentID:  parentID,
		Role:      role,
		Content:   content,
		CreatedAt: time.Now().UTC(),
		Metadata:  make(map[string]string),
	}

	meta, err := json.Marshal(node.Metadata)
	if err != nil {
		return nil, fmt.Errorf("marshal metadata: %w", err)
	}

	tx, err := d.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO nodes (id, parent_id, session_id, role, content, model, created_at, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		node.ID, node.ParentID, d.sessionID, node.Role, node.Content,
		node.Model, node.CreatedAt.Format(time.RFC3339Nano), string(meta),
	)
	if err != nil {
		return nil, fmt.Errorf("insert node: %w", err)
	}

	// Upsert the head pointer to this new node.
	_, err = tx.Exec(
		`INSERT INTO heads (session_id, node_id) VALUES (?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET node_id = excluded.node_id`,
		d.sessionID, node.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("update head: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}
	return node, nil
}

// Fork creates a new branch from the given node. It copies the fork point node
// as a marker and returns it. The returned node can be used as a parent for
// new messages on the alternative branch. Fork also moves the head pointer to
// this new fork point.
func (d *DAG) Fork(nodeID string) (*Node, error) {
	src, err := d.getNode(nodeID)
	if err != nil {
		return nil, fmt.Errorf("get fork point: %w", err)
	}

	fork := &Node{
		ID:        generateID(),
		ParentID:  src.ParentID,
		Role:      src.Role,
		Content:   src.Content,
		Model:     src.Model,
		CreatedAt: time.Now().UTC(),
		Metadata:  map[string]string{"forked_from": nodeID},
	}

	meta, _ := json.Marshal(fork.Metadata)

	tx, err := d.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		`INSERT INTO nodes (id, parent_id, session_id, role, content, model, created_at, metadata)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		fork.ID, fork.ParentID, d.sessionID, fork.Role, fork.Content,
		fork.Model, fork.CreatedAt.Format(time.RFC3339Nano), string(meta),
	)
	if err != nil {
		return nil, fmt.Errorf("insert fork node: %w", err)
	}

	_, err = tx.Exec(
		`INSERT INTO heads (session_id, node_id) VALUES (?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET node_id = excluded.node_id`,
		d.sessionID, fork.ID,
	)
	if err != nil {
		return nil, fmt.Errorf("update head to fork: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return fork, nil
}

// History returns the linear path from root to the given node by walking
// parent pointers. The returned slice is ordered from root to the given node.
func (d *DAG) History(nodeID string) ([]*Node, error) {
	var path []*Node
	current := nodeID
	visited := make(map[string]bool)

	for current != "" {
		if visited[current] {
			return nil, fmt.Errorf("cycle detected at node %q", current)
		}
		visited[current] = true

		node, err := d.getNode(current)
		if err != nil {
			return nil, fmt.Errorf("get node %q: %w", current, err)
		}
		path = append(path, node)
		current = node.ParentID
	}

	// Reverse so root is first.
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	return path, nil
}

// Branches returns all child nodes of the given node (i.e., fork points).
func (d *DAG) Branches(nodeID string) ([]*Node, error) {
	rows, err := d.db.Query(
		`SELECT id, parent_id, role, content, model, created_at, metadata
		 FROM nodes WHERE parent_id = ? AND session_id = ?
		 ORDER BY created_at ASC`,
		nodeID, d.sessionID,
	)
	if err != nil {
		return nil, fmt.Errorf("query branches: %w", err)
	}
	defer rows.Close()

	var nodes []*Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	return nodes, rows.Err()
}

// Head returns the most recent node in the current branch, i.e. the node
// the session's head pointer points to.
func (d *DAG) Head() (*Node, error) {
	var nodeID string
	err := d.db.QueryRow(
		`SELECT node_id FROM heads WHERE session_id = ?`, d.sessionID,
	).Scan(&nodeID)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no head for session %q", d.sessionID)
	}
	if err != nil {
		return nil, fmt.Errorf("query head: %w", err)
	}
	return d.getNode(nodeID)
}

// SetHead moves the current branch pointer to a specific node.
func (d *DAG) SetHead(nodeID string) error {
	exists, err := d.nodeExists(nodeID)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("node %q not found", nodeID)
	}

	_, err = d.db.Exec(
		`INSERT INTO heads (session_id, node_id) VALUES (?, ?)
		 ON CONFLICT(session_id) DO UPDATE SET node_id = excluded.node_id`,
		d.sessionID, nodeID,
	)
	return err
}

// Prune removes a node and all its descendants from the DAG.
// If the head points to a pruned node, the head is moved to the pruned node's parent.
func (d *DAG) Prune(nodeID string) error {
	node, err := d.getNode(nodeID)
	if err != nil {
		return fmt.Errorf("get node for prune: %w", err)
	}

	// Collect all descendant IDs via BFS.
	toDelete := []string{nodeID}
	queue := []string{nodeID}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]

		rows, err := d.db.Query(
			`SELECT id FROM nodes WHERE parent_id = ? AND session_id = ?`,
			current, d.sessionID,
		)
		if err != nil {
			return fmt.Errorf("query children: %w", err)
		}
		for rows.Next() {
			var childID string
			if err := rows.Scan(&childID); err != nil {
				rows.Close()
				return err
			}
			toDelete = append(toDelete, childID)
			queue = append(queue, childID)
		}
		rows.Close()
	}

	tx, err := d.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	// Check if head points to any node being deleted; if so, move to parent.
	var headNodeID string
	err = tx.QueryRow(`SELECT node_id FROM heads WHERE session_id = ?`, d.sessionID).Scan(&headNodeID)
	if err == nil {
		for _, id := range toDelete {
			if id == headNodeID {
				if node.ParentID != "" {
					_, err = tx.Exec(
						`UPDATE heads SET node_id = ? WHERE session_id = ?`,
						node.ParentID, d.sessionID,
					)
				} else {
					_, err = tx.Exec(`DELETE FROM heads WHERE session_id = ?`, d.sessionID)
				}
				if err != nil {
					return fmt.Errorf("move head during prune: %w", err)
				}
				break
			}
		}
	}

	// Delete all collected nodes.
	for _, id := range toDelete {
		if _, err := tx.Exec(`DELETE FROM nodes WHERE id = ?`, id); err != nil {
			return fmt.Errorf("delete node %q: %w", id, err)
		}
	}

	return tx.Commit()
}

// Close closes the underlying database connection.
func (d *DAG) Close() error {
	return d.db.Close()
}

// --- internal helpers ---

func (d *DAG) nodeExists(id string) (bool, error) {
	var count int
	err := d.db.QueryRow(`SELECT COUNT(*) FROM nodes WHERE id = ?`, id).Scan(&count)
	return count > 0, err
}

func (d *DAG) getNode(id string) (*Node, error) {
	row := d.db.QueryRow(
		`SELECT id, parent_id, role, content, model, created_at, metadata
		 FROM nodes WHERE id = ?`, id,
	)
	return scanSingleNode(row)
}

// scanner is satisfied by both *sql.Row and *sql.Rows.
type scanner interface {
	Scan(dest ...interface{}) error
}

func scanSingleNode(row *sql.Row) (*Node, error) {
	var (
		n          Node
		createdStr string
		metaStr    string
	)
	err := row.Scan(&n.ID, &n.ParentID, &n.Role, &n.Content, &n.Model, &createdStr, &metaStr)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("node not found")
	}
	if err != nil {
		return nil, fmt.Errorf("scan node: %w", err)
	}
	n.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	n.Metadata = make(map[string]string)
	json.Unmarshal([]byte(metaStr), &n.Metadata)
	return &n, nil
}

func scanNode(rows *sql.Rows) (*Node, error) {
	var (
		n          Node
		createdStr string
		metaStr    string
	)
	err := rows.Scan(&n.ID, &n.ParentID, &n.Role, &n.Content, &n.Model, &createdStr, &metaStr)
	if err != nil {
		return nil, fmt.Errorf("scan node: %w", err)
	}
	n.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
	n.Metadata = make(map[string]string)
	json.Unmarshal([]byte(metaStr), &n.Metadata)
	return &n, nil
}

// generateID produces a short 8-character hex ID from crypto/rand.
func generateID() string {
	b := make([]byte, 4)
	rand.Read(b)
	return fmt.Sprintf("%x", b)
}
