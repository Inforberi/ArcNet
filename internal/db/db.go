package db

/*
#cgo LDFLAGS: -lsqlite3
#include <sqlite3.h>
#include <stdlib.h>
*/
import "C"

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"unsafe"
)

type DB struct {
	mu   sync.Mutex
	conn *C.sqlite3
	path string
}
type Tx struct{ conn *C.sqlite3 }
type Row map[string]string

func Open(path string) (*DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return nil, err
	}
	cpath := C.CString(path)
	defer C.free(unsafe.Pointer(cpath))
	var h *C.sqlite3
	flags := C.int(C.SQLITE_OPEN_READWRITE | C.SQLITE_OPEN_CREATE | C.SQLITE_OPEN_FULLMUTEX)
	if code := C.sqlite3_open_v2(cpath, &h, flags, nil); code != C.SQLITE_OK {
		var err error = errors.New("sqlite open failed")
		if h != nil {
			err = fmt.Errorf("sqlite open: %s", C.GoString(C.sqlite3_errmsg(h)))
			C.sqlite3_close_v2(h)
		}
		return nil, err
	}
	d := &DB{conn: h, path: path}
	runtime.SetFinalizer(d, nil)
	if err := d.Read(context.Background(), func(tx *Tx) error {
		for _, q := range []string{"PRAGMA journal_mode=WAL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000", "PRAGMA synchronous=FULL"} {
			if err := tx.Exec(q); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		d.Close()
		return nil, err
	}
	return d, nil
}

func (d *DB) Path() string { return d.path }

func (d *DB) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn == nil {
		return nil
	}
	if C.sqlite3_close_v2(d.conn) != C.SQLITE_OK {
		return errors.New("sqlite close failed")
	}
	d.conn = nil
	return nil
}

// WithTx serializes writes, readers and checkpointing on one connection.
// No domain code obtains a raw SQLite handle.
func (d *DB) WithTx(ctx context.Context, fn func(*Tx) error) (err error) {
	if err = ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn == nil {
		return errors.New("database closed")
	}
	tx := &Tx{conn: d.conn}
	if err = tx.Exec("BEGIN IMMEDIATE"); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = tx.Exec("ROLLBACK")
		} else {
			commitErr := tx.Exec("COMMIT")
			if commitErr != nil {
				_ = tx.Exec("ROLLBACK")
				err = commitErr
			}
		}
	}()
	err = fn(tx)
	return err
}
func (d *DB) Read(ctx context.Context, fn func(*Tx) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn == nil {
		return errors.New("database closed")
	}
	return fn(&Tx{conn: d.conn})
}
func (tx *Tx) prepare(query string, args []string) (*C.sqlite3_stmt, error) {
	cq := C.CString(query)
	defer C.free(unsafe.Pointer(cq))
	var stmt *C.sqlite3_stmt
	if C.sqlite3_prepare_v2(tx.conn, cq, -1, &stmt, nil) != C.SQLITE_OK {
		return nil, fmt.Errorf("sqlite prepare: %s", C.GoString(C.sqlite3_errmsg(tx.conn)))
	}
	if int(C.sqlite3_bind_parameter_count(stmt)) != len(args) {
		C.sqlite3_finalize(stmt)
		return nil, errors.New("SQL placeholder count mismatch")
	}
	for i, arg := range args {
		ca := C.CString(arg)
		rc := C.sqlite3_bind_text(stmt, C.int(i+1), ca, C.int(len(arg)), C.SQLITE_TRANSIENT)
		C.free(unsafe.Pointer(ca))
		if rc != C.SQLITE_OK {
			C.sqlite3_finalize(stmt)
			return nil, fmt.Errorf("sqlite bind: %s", C.GoString(C.sqlite3_errmsg(tx.conn)))
		}
	}
	return stmt, nil
}
func (tx *Tx) Exec(query string, args ...string) error {
	stmt, err := tx.prepare(query, args)
	if err != nil {
		return err
	}
	defer C.sqlite3_finalize(stmt)
	for {
		rc := C.sqlite3_step(stmt)
		if rc == C.SQLITE_DONE {
			return nil
		}
		if rc == C.SQLITE_ROW {
			continue
		}
		return fmt.Errorf("sqlite exec: %s", C.GoString(C.sqlite3_errmsg(tx.conn)))
	}
}
func (tx *Tx) Query(query string, args ...string) ([]Row, error) {
	stmt, err := tx.prepare(query, args)
	if err != nil {
		return nil, err
	}
	defer C.sqlite3_finalize(stmt)
	cols := int(C.sqlite3_column_count(stmt))
	out := make([]Row, 0)
	for {
		rc := C.sqlite3_step(stmt)
		if rc == C.SQLITE_DONE {
			return out, nil
		}
		if rc != C.SQLITE_ROW {
			return nil, fmt.Errorf("sqlite query: %s", C.GoString(C.sqlite3_errmsg(tx.conn)))
		}
		row := make(Row, cols)
		for i := 0; i < cols; i++ {
			key := C.GoString(C.sqlite3_column_name(stmt, C.int(i)))
			if C.sqlite3_column_type(stmt, C.int(i)) == C.SQLITE_NULL {
				row[key] = ""
			} else {
				row[key] = C.GoString((*C.char)(unsafe.Pointer(C.sqlite3_column_text(stmt, C.int(i)))))
			}
		}
		out = append(out, row)
	}
}
