// Package support provides checked, secret-safe fixture operations.
package support

import (
	"database/sql"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
)

// Check fails the fixture without exposing request, SQL or credential values.
func Check(err error) {
	if err != nil {
		panic("fixture operation failed")
	}
}

func Written(_ int, err error)         { Check(err) }
func Executed(_ sql.Result, err error) { Check(err) }

func Close(closer io.Closer) {
	err := closer.Close()
	if !errors.Is(err, net.ErrClosed) {
		Check(err)
	}
}

func Served(err error) {
	if !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
		Check(err)
	}
}

// Stopped accepts the exit status produced by an intentional process stop.
func Stopped(err error) {
	var exit *exec.ExitError
	if !errors.Is(err, os.ErrProcessDone) && !errors.As(err, &exit) {
		Check(err)
	}
}
