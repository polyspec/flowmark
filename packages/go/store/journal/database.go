package journal

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"regexp"
)

// DatabaseDir is the declared place of the SQL databases of the openings of the corpus (spec 40 SQL statements): one
// database for each opening, `<token>.db` with the files SQLite keeps beside it, and `<token>.holder`, the holder record
// of the process that opened it, which the recovery judges as it judges a pending lock record.
const DatabaseDir = ".flowmark/sql"

// databaseSuffixes are the files of a database besides its holder record: the database, its write-ahead log, its
// shared memory and its rollback journal.
var databaseSuffixes = []string{".db", ".db-wal", ".db-shm", ".db-journal"}

var holderName = regexp.MustCompile(`^[0-9a-f]{32}\.holder$`)

// Database is the database of one opening of the corpus: Path is its file, which SQLite opens; the files beside it are
// SQLite's own.
type Database struct {
	c     *Corpus
	token string
	Path  string
}

// NewDatabase makes the place of a new database of this process: the folder DatabaseDir, made with access for its owner
// alone and checked to be so, and the holder record of this process under a new token, written and synced before the
// database exists, so a database is never without the record that the recovery judges. The database's file and the
// files SQLite keeps beside it are checked absent, so a planted link is refused; one planted between the check and the
// open needs the owner's own access to the folder.
func (c *Corpus) NewDatabase() (*Database, error) {
	self, err := c.self("sql")
	if err != nil {
		return nil, err
	}
	if err := c.ensureDir(".flowmark"); err != nil {
		return nil, fmt.Errorf("journal: the folder of the databases: %w", err)
	}
	dir := c.abs(DatabaseDir)
	if err := mkdirPrivate(dir); err != nil && !errors.Is(err, fs.ErrExist) {
		return nil, fmt.Errorf("journal: the folder of the databases: %w", err)
	}
	if err := c.checkDatabaseDir(); err != nil {
		return nil, err
	}
	if err := checkPrivate(dir); err != nil {
		return nil, fmt.Errorf("journal: %w", err)
	}
	d := &Database{c: c, token: self.Acquisition, Path: c.abs(DatabaseDir + "/" + self.Acquisition + ".db")}
	if err := c.checkAbsent(d.token); err != nil {
		return nil, err
	}
	if err := writeSynced(c.abs(DatabaseDir+"/"+d.token+".holder"), self.record(), os.O_EXCL); err != nil {
		return nil, fmt.Errorf("journal: the holder record of a database: %w", err)
	}
	return d, nil
}

// checkAbsent refuses the files of the database of token when one exists, a link included: no opening has made them.
func (c *Corpus) checkAbsent(token string) error {
	for _, suffix := range append([]string{".holder"}, databaseSuffixes...) {
		info, err := c.lstat(DatabaseDir + "/" + token + suffix)
		if err != nil {
			return fmt.Errorf("journal: %w", err)
		}
		if info != nil {
			return fmt.Errorf("journal: %s exists before its database is made; only an opening writes it, so check what wrote it", c.abs(DatabaseDir+"/"+token+suffix))
		}
	}
	return nil
}

// Remove removes the database and the files beside it, then its holder record: the end of the opening that made it.
func (d *Database) Remove() error {
	return d.c.removeDatabase(d.token)
}

// removeDatabase removes the files of the database of token, its holder record last, so a database is never left
// without its record.
func (c *Corpus) removeDatabase(token string) error {
	for _, suffix := range databaseSuffixes {
		if err := removeAbsent(c.abs(DatabaseDir + "/" + token + suffix)); err != nil {
			return fmt.Errorf("journal: %w", err)
		}
	}
	if err := removeAbsent(c.abs(DatabaseDir + "/" + token + ".holder")); err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	return nil
}

// checkDatabaseDir refuses a folder of the databases that is a symbolic link, a reparse point or no folder.
func (c *Corpus) checkDatabaseDir() error {
	info, err := c.lstat(DatabaseDir)
	if err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	if info != nil && (!info.IsDir() || info.Mode()&fs.ModeIrregular != 0) {
		return fmt.Errorf("journal: %s is not a folder", c.abs(DatabaseDir))
	}
	return nil
}

// removeEndedDatabases removes the databases of the processes of this host that have ended, on opening, as their
// pending lock records are removed: a running holder's database, one of another host, whose processes cannot be judged
// from here, and one whose holder record does not parse, since its writer may still be writing it, stay; a name that is
// no holder record is never touched.
func (c *Corpus) removeEndedDatabases() error {
	if err := c.checkDatabaseDir(); err != nil {
		return err
	}
	return c.removeEnded(DatabaseDir, holderName, func(name, _ string) error { return c.removeDatabase(name[:32]) })
}

// removeEnded calls remove with the name and the path of each record in the folder dir of the corpus whose name name
// matches and whose holder has ended, on opening: a record of a running holder, of another host, or one that does not
// parse, since its writer may still be writing it, stays, and a name that does not match is never touched. A symlink
// among the records is refused.
func (c *Corpus) removeEnded(dir string, name *regexp.Regexp, remove func(name, path string) error) error {
	entries, err := os.ReadDir(c.abs(dir))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("journal: %w", err)
	}
	for _, e := range entries {
		if !name.MatchString(e.Name()) {
			continue
		}
		p := c.abs(dir + "/" + e.Name())
		if e.Type()&fs.ModeSymlink != 0 {
			return fmt.Errorf("journal: %w", symlinkError(p))
		}
		data, _, err := readShared(p)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("journal: %w", err)
		}
		h, err := parseRecord(data)
		if err != nil {
			continue
		}
		state, err := c.judge(h)
		if err != nil {
			return err
		}
		if state == HolderEnded {
			if err := remove(e.Name(), p); err != nil {
				return err
			}
		}
	}
	return nil
}
