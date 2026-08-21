package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"task141-firehydraulics/internal/model"
)

// --- Project ---

// CreateProject inserts a project row.
func (s *Store) CreateProject(ctx context.Context, tx *sql.Tx, p *model.Project) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO projects(id,name,hazard_class,design_date_epoch,created_at) VALUES(?,?,?,?,?)`,
		p.ID, p.Name, string(p.HazardClass), p.DesignDate, p.CreatedAt)
	if err != nil {
		return fmt.Errorf("create project: %w", err)
	}
	return nil
}

// ListProjects returns all projects.
func (s *Store) ListProjects(ctx context.Context) ([]model.Project, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,name,hazard_class,design_date_epoch,created_at FROM projects ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list projects: %w", err)
	}
	defer rows.Close()
	var out []model.Project
	for rows.Next() {
		var p model.Project
		var hc string
		if err := rows.Scan(&p.ID, &p.Name, &hc, &p.DesignDate, &p.CreatedAt); err != nil {
			return nil, err
		}
		p.HazardClass = model.HazardClass(hc)
		out = append(out, p)
	}
	return out, rows.Err()
}

// GetProject returns one project.
func (s *Store) GetProject(ctx context.Context, id string) (*model.Project, error) {
	var p model.Project
	var hc string
	err := s.db.QueryRowContext(ctx,
		`SELECT id,name,hazard_class,design_date_epoch,created_at FROM projects WHERE id=?`, id).
		Scan(&p.ID, &p.Name, &hc, &p.DesignDate, &p.CreatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.HazardClass = model.HazardClass(hc)
	return &p, nil
}
