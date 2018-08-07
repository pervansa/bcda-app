// Package models contains the types for schema 'public'.
package models

// Code generated by xo. DO NOT EDIT.

import (
	"errors"
	"time"

	"github.com/pborman/uuid"
)

// Job represents a row from 'public.jobs'.
type Job struct {
	ID        int       `json:"id"`         // id
	AcoID     uuid.UUID `json:"aco_id"`     // aco_id
	UserID    uuid.UUID `json:"user_id"`    // user_id
	Location  string    `json:"location"`   // location
	Status    string    `json:"status"`     // status
	CreatedAt time.Time `json:"created_at"` // created_at

	// xo fields
	_exists, _deleted bool
}

// Exists determines if the Job exists in the database.
func (j *Job) Exists() bool {
	return j._exists
}

// Deleted provides information if the Job has been deleted from the database.
func (j *Job) Deleted() bool {
	return j._deleted
}

// Insert inserts the Job to the database.
func (j *Job) Insert(db XODB) error {
	var err error

	// if already exist, bail
	if j._exists {
		return errors.New("insert failed: already exists")
	}

	// sql insert query, primary key provided by sequence
	const sqlstr = `INSERT INTO public.jobs (` +
		`aco_id, user_id, location, status, created_at` +
		`) VALUES (` +
		`$1, $2, $3, $4, $5` +
		`) RETURNING id`

	// run query
	XOLog(sqlstr, j.AcoID, j.UserID, j.Location, j.Status, j.CreatedAt)
	err = db.QueryRow(sqlstr, j.AcoID, j.UserID, j.Location, j.Status, j.CreatedAt).Scan(&j.ID)
	if err != nil {
		return err
	}

	// set existence
	j._exists = true

	return nil
}

// Update updates the Job in the database.
func (j *Job) Update(db XODB) error {
	var err error

	// if doesn't exist, bail
	if !j._exists {
		return errors.New("update failed: does not exist")
	}

	// if deleted, bail
	if j._deleted {
		return errors.New("update failed: marked for deletion")
	}

	// sql query
	const sqlstr = `UPDATE public.jobs SET (` +
		`aco_id, user_id, location, status, created_at` +
		`) = ( ` +
		`$1, $2, $3, $4, $5` +
		`) WHERE id = $6`

	// run query
	XOLog(sqlstr, j.AcoID, j.UserID, j.Location, j.Status, j.CreatedAt, j.ID)
	_, err = db.Exec(sqlstr, j.AcoID, j.UserID, j.Location, j.Status, j.CreatedAt, j.ID)
	return err
}

// Save saves the Job to the database.
func (j *Job) Save(db XODB) error {
	if j.Exists() {
		return j.Update(db)
	}

	return j.Insert(db)
}

// Upsert performs an upsert for Job.
//
// NOTE: PostgreSQL 9.5+ only
func (j *Job) Upsert(db XODB) error {
	var err error

	// if already exist, bail
	if j._exists {
		return errors.New("insert failed: already exists")
	}

	// sql query
	const sqlstr = `INSERT INTO public.jobs (` +
		`id, aco_id, user_id, location, status, created_at` +
		`) VALUES (` +
		`$1, $2, $3, $4, $5, $6` +
		`) ON CONFLICT (id) DO UPDATE SET (` +
		`id, aco_id, user_id, location, status, created_at` +
		`) = (` +
		`EXCLUDED.id, EXCLUDED.aco_id, EXCLUDED.user_id, EXCLUDED.location, EXCLUDED.status, EXCLUDED.created_at` +
		`)`

	// run query
	XOLog(sqlstr, j.ID, j.AcoID, j.UserID, j.Location, j.Status, j.CreatedAt)
	_, err = db.Exec(sqlstr, j.ID, j.AcoID, j.UserID, j.Location, j.Status, j.CreatedAt)
	if err != nil {
		return err
	}

	// set existence
	j._exists = true

	return nil
}

// Delete deletes the Job from the database.
func (j *Job) Delete(db XODB) error {
	var err error

	// if doesn't exist, bail
	if !j._exists {
		return nil
	}

	// if deleted, bail
	if j._deleted {
		return nil
	}

	// sql query
	const sqlstr = `DELETE FROM public.jobs WHERE id = $1`

	// run query
	XOLog(sqlstr, j.ID)
	_, err = db.Exec(sqlstr, j.ID)
	if err != nil {
		return err
	}

	// set deleted
	j._deleted = true

	return nil
}

// Aco returns the Aco associated with the Job's AcoID (aco_id).
//
// Generated from foreign key 'jobs_aco_id_fkey'.
func (j *Job) Aco(db XODB) (*Aco, error) {
	return AcoByUUID(db, j.AcoID)
}

// User returns the User associated with the Job's UserID (user_id).
//
// Generated from foreign key 'jobs_user_id_fkey'.
func (j *Job) User(db XODB) (*User, error) {
	return UserByUUID(db, j.UserID)
}

// JobByID retrieves a row from 'public.jobs' as a Job.
//
// Generated from index 'jobs_pkey'.
func JobByID(db XODB, id int) (*Job, error) {
	var err error

	// sql query
	const sqlstr = `SELECT ` +
		`id, aco_id, user_id, location, status, created_at ` +
		`FROM public.jobs ` +
		`WHERE id = $1`

	// run query
	XOLog(sqlstr, id)
	j := Job{
		_exists: true,
	}

	err = db.QueryRow(sqlstr, id).Scan(&j.ID, &j.AcoID, &j.UserID, &j.Location, &j.Status, &j.CreatedAt)
	if err != nil {
		return nil, err
	}

	return &j, nil
}
