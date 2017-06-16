package beater

import (
	"database/sql"
	"fmt"

	"github.com/elastic/beats/libbeat/logp"
	_ "github.com/mattn/go-sqlite3"
)

const databaseName = "streams"

type SqliteRegistry struct {
	db *sql.DB
}

type SqliteRegistryItem struct {
	NextToken string
	Buffer    string
}

func NewSqliteRegistry(dbPath string) Registry {
	db, err := sql.Open("sqlite3", dbPath)
	Fatal(err)
	registry := &SqliteRegistry{db: db}
	registry.createDbIfNecessary()
	return registry
}

func (registry *SqliteRegistry) ReadStreamInfo(stream *Stream) error {
	var err error

	key := generateKey(stream)
	defer func() {
		if err != nil {
			logp.Warn(fmt.Sprintf("sqlite: failed to write key=%s [message=%s]", key, err.Error()))
		}
	}()

	logp.Info("Fetching registry info for %s", key)
	var row SqliteRegistryItem
	err = registry.db.QueryRow("SELECT NextToken, Buffer FROM ? WHERE Key=?",
		databaseName, key).Scan(&row)
	if err == sql.ErrNoRows {
		err = nil
		return nil
	}

	// update stream
	stream.Params.NextToken = &row.NextToken
	stream.Buffer.Reset()
	stream.Buffer.WriteString(row.Buffer)

	return nil
}

func (registry *SqliteRegistry) WriteStreamInfo(stream *Stream) error {
	key := generateKey(stream)
	buf := stream.Buffer.String()
	nextToken := *stream.Params.NextToken

	_, err := registry.db.Exec("INSERT OR REPLACE INTO ? (Key,Buffer,NextToken) VALUES(?,?,?)",
		databaseName, key, buf, nextToken)
	if err != nil {
		logp.Warn(fmt.Sprintf("sqlite: failed to write key=%s [message=%s]", key, err.Error()))
	}
	return err
}

func (registry *SqliteRegistry) createDbIfNecessary() {
	_, err := registry.db.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s
  (Key VARCHAR(1024) NOT NULL,
   Buffer TEXT,
   NextToken TEXT,
   PRIMARY KEY(Key))
`, databaseName))
	Fatal(err)
}
