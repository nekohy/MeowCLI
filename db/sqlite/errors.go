package sqlite

import (
	"database/sql"
	"errors"
	"strings"

	db "github.com/nekohy/MeowCLI/internal/store"
)

func wrapError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, sql.ErrNoRows) {
		return db.ErrNotFound
	}
	if strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return db.ErrConflict
	}
	return err
}
