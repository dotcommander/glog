package sqlite

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/dotcommander/glog/internal/constants"
	"github.com/dotcommander/glog/internal/domain/entities"
	"github.com/dotcommander/glog/internal/domain/ports"
)

const inboxColumns = `id, title, body, status, kind, priority, source, repo, session_id,
       COALESCE(tags, '[]'), COALESCE(linked_log_ids, '[]'), COALESCE(linked_fingerprints, '[]'),
       created_by, claimed_by, created_at, updated_at, due_at`

type InboxRepository struct {
	db *Database
}

func NewInboxRepository(db *Database) *InboxRepository {
	return &InboxRepository{db: db}
}

func scanInboxItem(scanner interface{ Scan(...any) error }, item *entities.InboxItem) error {
	return scanner.Scan(
		&item.ID, &item.Title, &item.Body, &item.Status, &item.Kind, &item.Priority,
		&item.Source, &item.Repo, &item.SessionID, &item.Tags, &item.LinkedLogIDs,
		&item.LinkedFingerprints, &item.CreatedBy, &item.ClaimedBy, &item.CreatedAt,
		&item.UpdatedAt, &item.DueAt,
	)
}

func (r *InboxRepository) Create(item *entities.InboxItem) error {
	if err := item.Validate(); err != nil {
		return err
	}

	query := `
		INSERT INTO inbox_items (
			title, body, status, kind, priority, source, repo, session_id,
			tags, linked_log_ids, linked_fingerprints, created_by, claimed_by,
			created_at, updated_at, due_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`
	result, err := r.db.Conn().Exec(
		query, item.Title, item.Body, item.Status, item.Kind, item.Priority,
		item.Source, item.Repo, item.SessionID, item.Tags, item.LinkedLogIDs,
		item.LinkedFingerprints, item.CreatedBy, item.ClaimedBy, item.CreatedAt,
		item.UpdatedAt, item.DueAt,
	)
	if err != nil {
		return fmt.Errorf("failed to insert inbox item: %w", err)
	}
	id, err := result.LastInsertId()
	if err != nil {
		return fmt.Errorf("failed to get last insert ID: %w", err)
	}
	item.ID = id
	return nil
}

func (r *InboxRepository) FindByID(id int64) (*entities.InboxItem, error) {
	var item entities.InboxItem
	query := `SELECT ` + inboxColumns + ` FROM inbox_items WHERE id = ?`
	if err := scanInboxItem(r.db.Conn().QueryRow(query, id), &item); err == sql.ErrNoRows {
		return nil, ports.ErrNotFound
	} else if err != nil {
		return nil, fmt.Errorf("failed to query inbox item: %w", err)
	}
	return &item, nil
}

func (r *InboxRepository) List(filters ports.InboxFilters) ([]*entities.InboxItem, int, error) {
	filters.Limit, filters.Offset = constants.NormalizePagination(filters.Limit, filters.Offset)

	where, args := inboxWhere(filters)
	countQuery := `SELECT COUNT(*) FROM inbox_items` + where
	var total int
	if err := r.db.Conn().QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count inbox items: %w", err)
	}

	query := `SELECT ` + inboxColumns + ` FROM inbox_items` + where + ` ORDER BY updated_at DESC, id DESC LIMIT ? OFFSET ?`
	args = append(args, filters.Limit, filters.Offset)

	rows, err := r.db.Conn().Query(query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to query inbox items: %w", err)
	}
	defer rows.Close()

	var items []*entities.InboxItem
	for rows.Next() {
		var item entities.InboxItem
		if err := scanInboxItem(rows, &item); err != nil {
			return nil, 0, fmt.Errorf("failed to scan inbox item: %w", err)
		}
		items = append(items, &item)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating inbox items: %w", err)
	}
	return items, total, nil
}

func inboxWhere(filters ports.InboxFilters) (string, []any) {
	var clauses []string
	var args []any
	add := func(column, value string) {
		if strings.TrimSpace(value) == "" {
			return
		}
		clauses = append(clauses, column+" = ?")
		args = append(args, value)
	}
	add("status", filters.Status)
	add("kind", filters.Kind)
	if repo := strings.TrimSpace(filters.Repo); repo != "" {
		base := strings.TrimSuffix(repo, "/")
		if i := strings.LastIndex(base, "/"); i >= 0 {
			base = base[i+1:]
		}
		clauses = append(clauses, "(repo = ? OR repo = ? OR repo LIKE ?)")
		args = append(args, repo, base, "%/"+base)
	}
	add("session_id", filters.SessionID)
	add("source", filters.Source)
	if len(clauses) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(clauses, " AND "), args
}

func (r *InboxRepository) Update(item *entities.InboxItem) error {
	if err := item.Validate(); err != nil {
		return err
	}
	query := `
		UPDATE inbox_items SET
			title = ?, body = ?, status = ?, kind = ?, priority = ?, source = ?,
			repo = ?, session_id = ?, tags = ?, linked_log_ids = ?,
			linked_fingerprints = ?, created_by = ?, claimed_by = ?,
			updated_at = ?, due_at = ?
		WHERE id = ?
	`
	result, err := r.db.Conn().Exec(
		query, item.Title, item.Body, item.Status, item.Kind, item.Priority,
		item.Source, item.Repo, item.SessionID, item.Tags, item.LinkedLogIDs,
		item.LinkedFingerprints, item.CreatedBy, item.ClaimedBy, item.UpdatedAt,
		item.DueAt, item.ID,
	)
	if err != nil {
		return fmt.Errorf("failed to update inbox item: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return ports.ErrNotFound
	}
	return nil
}

func (r *InboxRepository) Delete(id int64) error {
	result, err := r.db.Conn().Exec(`DELETE FROM inbox_items WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("failed to delete inbox item: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return ports.ErrNotFound
	}
	return nil
}
