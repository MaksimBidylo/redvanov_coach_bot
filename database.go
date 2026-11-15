package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func openDatabase(ctx context.Context, databaseURL string) (*pgxpool.Pool, error) {
	if strings.TrimSpace(databaseURL) == "" {
		return nil, errors.New("DATABASE_URL is required")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, errors.New("invalid DATABASE_URL")
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	return pool, nil
}

type group struct {
	ID   int64
	Name string
}

func listGroups(ctx context.Context, pool *pgxpool.Pool) ([]group, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := pool.Query(ctx, "SELECT id, name FROM groups ORDER BY name")
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[group])
}

func getGroupName(ctx context.Context, pool *pgxpool.Pool, id int64) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var name string
	err := pool.QueryRow(ctx, "SELECT name FROM groups WHERE id = $1", id).Scan(&name)
	return name, err
}

func createGroup(ctx context.Context, pool *pgxpool.Pool, name string) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := pool.Exec(ctx, "INSERT INTO groups (name) VALUES ($1) ON CONFLICT ((lower(name))) DO NOTHING", name)
	return result.RowsAffected() == 1, err
}

func createClient(ctx context.Context, pool *pgxpool.Pool, name string, birthDate *time.Time, groupID *int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	_, err := pool.Exec(ctx, "INSERT INTO clients (name, birth_date, group_id) VALUES ($1, $2, $3)", name, birthDate, groupID)
	return err
}

type client struct {
	ID        int64
	Name      string
	BirthDate *time.Time
	GroupID   *int64
	GroupName *string
	Archived  bool
}

func listGroupClients(ctx context.Context, pool *pgxpool.Pool, groupID *int64, includeArchived bool) ([]client, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := pool.Query(ctx, "SELECT c.id, c.name, c.birth_date, c.group_id, g.name, c.archived FROM clients c LEFT JOIN groups g ON g.id = c.group_id WHERE c.group_id IS NOT DISTINCT FROM $1::bigint AND (NOT c.archived OR $2::boolean) ORDER BY c.name, c.id", groupID, includeArchived)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[client])
}

func getClient(ctx context.Context, pool *pgxpool.Pool, id int64) (client, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	var c client
	err := pool.QueryRow(ctx, "SELECT c.id, c.name, c.birth_date, c.group_id, g.name, c.archived FROM clients c LEFT JOIN groups g ON g.id = c.group_id WHERE c.id = $1", id).Scan(&c.ID, &c.Name, &c.BirthDate, &c.GroupID, &c.GroupName, &c.Archived)
	return c, err
}

func searchClients(ctx context.Context, pool *pgxpool.Pool, name string) ([]client, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	rows, err := pool.Query(ctx, "SELECT c.id, c.name, c.birth_date, c.group_id, g.name, c.archived FROM clients c LEFT JOIN groups g ON g.id = c.group_id WHERE strpos(lower(c.name), lower($1)) > 0 ORDER BY c.name, c.id LIMIT 21", name)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[client])
}

func setClientGroup(ctx context.Context, pool *pgxpool.Pool, clientID, groupID int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := pool.Exec(ctx, "UPDATE clients SET group_id = NULLIF($2::bigint, 0) WHERE id = $1", clientID, groupID)
	if err == nil && result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}

func renameGroup(ctx context.Context, pool *pgxpool.Pool, id int64, name string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := pool.Exec(ctx, "UPDATE groups SET name = $2 WHERE id = $1", id, name)
	if err == nil && result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}

func updateClientField(ctx context.Context, pool *pgxpool.Pool, id int64, birthDate bool, value any) error {
	query := "UPDATE clients SET name = $2 WHERE id = $1"
	if birthDate {
		query = "UPDATE clients SET birth_date = $2 WHERE id = $1"
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := pool.Exec(ctx, query, id, value)
	if err == nil && result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}

func deleteGroup(ctx context.Context, pool *pgxpool.Pool, sourceID, targetID int64) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	return pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "UPDATE clients SET group_id = NULLIF($2::bigint, 0) WHERE group_id = $1", sourceID, targetID); err != nil {
			return err
		}
		result, err := tx.Exec(ctx, "DELETE FROM groups WHERE id = $1", sourceID)
		if err == nil && result.RowsAffected() == 0 {
			return pgx.ErrNoRows
		}
		return err
	})
}

func setClientArchived(ctx context.Context, pool *pgxpool.Pool, id int64, archived bool) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	result, err := pool.Exec(ctx, "UPDATE clients SET archived = $2 WHERE id = $1", id, archived)
	if err == nil && result.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return err
}
