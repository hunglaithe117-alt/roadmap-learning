// Package txtx provides shared database transaction management for repositories.
package txtx

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

// ErrTxMismatch is returned when a transaction handle was not created by txtx.
var ErrTxMismatch = errors.New("tx handle mismatch")

type handle struct{ tx *gorm.DB }

// Do executes fn within a database transaction, automatically rolling back on error.
func Do(ctx context.Context, db *gorm.DB, fn func(tx any) error) error {
	return db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return fn(handle{tx: tx})
	})
}

// Of extracts the *gorm.DB from tx, returning db if tx is nil, or ErrTxMismatch if invalid.
func Of(db *gorm.DB, tx any) (*gorm.DB, error) {
	if tx == nil {
		return db, nil
	}
	h, ok := tx.(handle)
	if !ok || h.tx == nil {
		return nil, ErrTxMismatch
	}
	return h.tx, nil
}

