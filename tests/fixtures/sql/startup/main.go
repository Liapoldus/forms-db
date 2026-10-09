package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Liapoldus/forms-db/tests/fixtures/support"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Liapoldus/forms-db/internal/domain/models"
	"github.com/Liapoldus/forms-db/internal/infrastructure/storage/sqlstore"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const schemaName = "contact"

var schemas = map[string]json.RawMessage{
	schemaName: json.RawMessage(`{"type":"object","properties":{"email":{"type":"string"}}}`),
}

func main() {
	if err := run(); err != nil {
		support.Written(fmt.Fprintln(os.Stderr, err))
		os.Exit(1)
	}
}

func run() error {
	driver := os.Getenv("FORMS_STARTUP_SQL_DRIVER")
	if driver == "" {
		driver = "sqlite"
	}
	dsn := os.Getenv("FORMS_STARTUP_SQL_DSN")
	if os.Getenv("FORMS_STARTUP_CHILD") == "1" {
		if dsn == "" {
			return errors.New("missing child SQL endpoint")
		}
		return runReplica(driver, dsn, os.Getenv("FORMS_STARTUP_PREFIX"), os.Getenv("FORMS_STARTUP_SUBMISSION_ID"))
	}
	if driver == "sqlite" {
		directory, err := os.MkdirTemp("", "forms-startup-")
		if err != nil {
			return errors.New("create temporary startup directory")
		}
		defer func() { support.Check(os.RemoveAll(directory)) }()
		dsn = "file:" + filepath.ToSlash(filepath.Join(directory, "shared.sqlite")) + "?mode=rwc"
	}
	if driver != "sqlite" && (driver != "mysql" && driver != "postgres" || dsn == "") {
		return errors.New("invalid shared SQL startup configuration")
	}
	const rounds = 12
	executable, err := os.Executable()
	if err != nil {
		return errors.New("resolve startup fixture executable")
	}
	for round := range rounds {
		prefix := fmt.Sprintf("startup_%02d_", round)
		if driver != "sqlite" {
			defer func(prefix string) {
				if err := dropTables(driver, dsn, prefix); err != nil {
					support.Written(fmt.Fprintln(os.Stderr, "failed to clean up startup fixture tables"))
				}
			}(prefix)
		}
		if err := startReplicasTogether(executable, driver, dsn, prefix, round); err != nil {
			return err
		}
		repository, err := sqlstore.NewRepository(context.Background(), driver, dsn, prefix, schemas)
		if err != nil {
			return errors.New("open storage after concurrent replica startup")
		}
		page, err := repository.List(context.Background(), "shared", schemaName, nil, nil, 10)
		support.Check(repository.Close())
		seen := map[string]bool{}
		for _, item := range page {
			seen[item.ID] = true
		}
		if err != nil || len(page) != 2 || !seen[fmt.Sprintf("frm_%02d_a", round)] || !seen[fmt.Sprintf("frm_%02d_b", round)] {
			return fmt.Errorf("shared SQL storage did not retain both replica writes after concurrent startup (rows=%d)", len(page))
		}
	}

	if _, err := fmt.Fprintf(os.Stdout, "{\"databaseDriver\":%q,\"simultaneousInitializations\":%d,\"allReplicasOpened\":true,\"crossReplicaReadAfterWrite\":true}\n", driver, rounds); err != nil {
		return errors.New("write startup fixture result")
	}
	return nil
}

func runReplica(driver, dsn, prefix, submissionID string) error {
	if prefix == "" || submissionID == "" {
		return errors.New("missing startup replica inputs")
	}
	repository, err := sqlstore.NewRepository(context.Background(), driver, dsn, prefix, schemas)
	if err != nil {
		return fmt.Errorf("concurrent replica storage initialization failed: %w", err)
	}
	defer support.Close(repository)
	submission := models.Submission{
		ID: submissionID, Site: "shared", Schema: schemaName,
		CreatedAt: "2026-10-06T00:00:00Z", Data: map[string]any{"email": submissionID + "@example.test"},
	}
	if _, err := repository.Submit(context.Background(), submission); err != nil {
		return errors.New("replica could not write shared SQL storage")
	}
	return nil
}

func startReplicasTogether(executable, driver, dsn, prefix string, round int) error {
	commands := make([]*exec.Cmd, 0, 2)
	outputs := make([]*bytes.Buffer, 0, 2)
	for _, suffix := range []string{"a", "b"} {
		command := exec.CommandContext(context.Background(), executable)
		command.Env = append(os.Environ(),
			"FORMS_STARTUP_CHILD=1",
			"FORMS_STARTUP_SQL_DRIVER="+driver,
			"FORMS_STARTUP_SQL_DSN="+dsn,
			"FORMS_STARTUP_PREFIX="+prefix,
			"FORMS_STARTUP_SUBMISSION_ID="+fmt.Sprintf("frm_%02d_%s", round, suffix),
		)
		output := &bytes.Buffer{}
		command.Stdout = output
		command.Stderr = output
		commands = append(commands, command)
		outputs = append(outputs, output)
	}
	started := make([]*exec.Cmd, 0, len(commands))
	for _, command := range commands {
		if err := command.Start(); err != nil {
			for _, process := range started {
				support.Stopped(process.Process.Kill())
				support.Stopped(process.Wait())
			}
			return errors.New("start concurrent forms-db replicas")
		}
		started = append(started, command)
	}
	var failed bool
	for index, command := range commands {
		if err := command.Wait(); err != nil {
			failed = true
			support.Written(fmt.Fprintf(os.Stderr, "replica startup failed: %s\n", bytes.TrimSpace(outputs[index].Bytes())))
		}
	}
	if failed {
		return errors.New("concurrent process startup was not safe")
	}
	return nil
}

func dropTables(driver, dsn, prefix string) error {
	databaseDriver := "pgx"
	quote := `"`
	if driver == "mysql" {
		databaseDriver = "mysql"
		quote = "`"
	}
	database, err := sql.Open(databaseDriver, dsn)
	if err != nil {
		return errors.New("open SQL cleanup connection")
	}
	defer support.Close(database)
	for _, suffix := range []string{"submissions", "schemas"} {
		// #nosec G202 -- The prefix is generated by this fixture; suffix and dialect quote come from closed sets.
		statement := "DROP TABLE IF EXISTS " + quote + prefix + suffix + quote
		if _, err := database.ExecContext(context.Background(), statement); err != nil {
			return errors.New("remove temporary SQL startup tables")
		}
	}
	return nil
}
