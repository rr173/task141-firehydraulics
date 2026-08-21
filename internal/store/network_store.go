package store

import (
	"context"
	"database/sql"
	"fmt"

	"task141-firehydraulics/internal/model"
)

// --- Node ---

// CreateNode inserts a node.
func (s *Store) CreateNode(ctx context.Context, tx *sql.Tx, n *model.Node) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO nodes(id,system_id,type,label,elevation_mm,k_factor,design_min_pressure_mbar,seq)
		 VALUES(?,?,?,?,?,?,?,?)`,
		n.ID, n.SystemID, string(n.Type), n.Label, n.ElevationMM, n.KFactor, n.DesignMinPressure, n.Seq)
	if err != nil {
		return fmt.Errorf("create node: %w", err)
	}
	return nil
}

// ListNodesBySystem returns all nodes of a system ordered by seq.
func (s *Store) ListNodesBySystem(ctx context.Context, systemID string) ([]model.Node, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,system_id,type,label,elevation_mm,k_factor,design_min_pressure_mbar,seq
		 FROM nodes WHERE system_id=? ORDER BY seq`, systemID)
	if err != nil {
		return nil, fmt.Errorf("list nodes: %w", err)
	}
	defer rows.Close()
	var out []model.Node
	for rows.Next() {
		var n model.Node
		var nt string
		if err := rows.Scan(&n.ID, &n.SystemID, &nt, &n.Label, &n.ElevationMM, &n.KFactor, &n.DesignMinPressure, &n.Seq); err != nil {
			return nil, err
		}
		n.Type = model.NodeType(nt)
		out = append(out, n)
	}
	return out, rows.Err()
}

// --- Pipe ---

// CreatePipe inserts a pipe segment.
func (s *Store) CreatePipe(ctx context.Context, tx *sql.Tx, p *model.PipeSegment) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO pipe_segments(id,system_id,upstream_node_id,downstream_node_id,nominal_dia_mm,inner_dia_mm,length_mm,c_factor,fitting_equiv_mm,seq)
		 VALUES(?,?,?,?,?,?,?,?,?,?)`,
		p.ID, p.SystemID, p.UpstreamNodeID, p.DownstreamNodeID, p.NominalDiaMM, p.InnerDiaMM,
		p.LengthMM, p.CFactor, p.FittingEquivMM, p.Seq)
	if err != nil {
		return fmt.Errorf("create pipe: %w", err)
	}
	return nil
}

// ListPipesBySystem returns all pipes of a system ordered by seq.
func (s *Store) ListPipesBySystem(ctx context.Context, systemID string) ([]model.PipeSegment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,system_id,upstream_node_id,downstream_node_id,nominal_dia_mm,inner_dia_mm,length_mm,c_factor,fitting_equiv_mm,seq
		 FROM pipe_segments WHERE system_id=? ORDER BY seq`, systemID)
	if err != nil {
		return nil, fmt.Errorf("list pipes: %w", err)
	}
	defer rows.Close()
	var out []model.PipeSegment
	for rows.Next() {
		var p model.PipeSegment
		if err := rows.Scan(&p.ID, &p.SystemID, &p.UpstreamNodeID, &p.DownstreamNodeID, &p.NominalDiaMM,
			&p.InnerDiaMM, &p.LengthMM, &p.CFactor, &p.FittingEquivMM, &p.Seq); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
