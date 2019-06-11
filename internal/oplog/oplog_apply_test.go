package oplog

import (
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/globalsign/mgo"
	"github.com/globalsign/mgo/bson"
	"github.com/percona/percona-backup-mongodb/bsonfile"
	"github.com/percona/percona-backup-mongodb/internal/testutils"
	"github.com/pkg/errors"
)

type mockBSONReader struct {
	index int
	docs  []bson.M
}

func (m *mockBSONReader) ReadNext() ([]byte, error) {
	m.index++
	if m.index >= len(m.docs) {
		return nil, io.EOF
	}

	return bson.Marshal(m.docs[m.index])
}

func (m *mockBSONReader) UnmarshalNext(dest interface{}) error {
	m.index++
	if m.index >= len(m.docs) {
		return io.EOF
	}

	buff, err := bson.Marshal(m.docs[m.index])
	if err != nil {
		return errors.Wrap(err, "cannot marshal to unmarshal")
	}
	if err := bson.Unmarshal(buff, dest); err != nil {
		return err
	}

	return nil
}

func newMockBSONReader(docs []bson.M) *mockBSONReader {
	return &mockBSONReader{
		index: -1,
		docs:  docs,
	}
}

func TestApplyCreateCollection(t *testing.T) {
	docs := []bson.M{
		{
			"v":  int(2),
			"op": "c",
			"ns": "test.$cmd",
			"ts": bson.MongoTimestamp(6688736102603292686),
			"t":  int64(1),
			"h":  int64(2385982676716983621),
			"ui": bson.Binary{
				Kind: 0x4,
				Data: []byte{0xa6, 0xcb, 0xa6, 0xd5, 0xb0, 0x3, 0x4d, 0xab, 0x8b, 0x3, 0x96, 0xe9, 0x81, 0xc2, 0x9b, 0x48},
			},
			"wall": time.Now(),
			"o": bson.M{
				"create":          "testcol_00001",
				"autoIndexId":     bool(true),
				"validationLevel": "strict",
				"idIndex": bson.M{
					"ns": "test.testcol_00001",
					"v":  int(2),
					"key": bson.M{
						"_id": int(1),
					},
					"name": "_id_",
				},
			},
		},
		{
			"ts": bson.MongoTimestamp(6688736102603292690),
			"ns": "test.$cmd",
			"o": bson.M{
				"validationLevel": "strict",
				"idIndex": bson.M{
					"ns": "test.testcol_00002",
					"v":  int(2),
					"key": bson.M{
						"_id": int(1),
					},
					"name": "_id_",
				},
				"create":      "testcol_00002",
				"autoIndexId": bool(true),
			},
			"wall": time.Now(),
			"t":    int64(1),
			"h":    int64(4227177456625961085),
			"v":    int(2),
			"op":   "c",
			"ui": bson.Binary{
				Kind: 0x4,
				Data: []byte{0xf, 0x67, 0x4, 0x2f, 0x99, 0x17, 0x41, 0x8, 0xa0, 0x68, 0xe9, 0x51, 0x75, 0xc, 0xac, 0x3c},
			},
		},
	}

	bsonReader := newMockBSONReader(docs)

	session, err := mgo.DialWithInfo(testutils.PrimaryDialInfo(t, testutils.MongoDBShard1ReplsetName))
	if err != nil {
		t.Fatalf("Cannot connect to primary: %s", err)
	}

	dbname := "test"
	testColsPrefix := "testcol_"

	db := session.DB(dbname)

	if err := cleanup(db, testColsPrefix); err != nil {
		t.Fatalf("Cannot clean up %s database before starting: %s", dbname, err)
	}

	oa, err := NewOplogApply(session, bsonReader)
	if err != nil {
		t.Errorf("Cannot instantiate the oplog applier: %s", err)
	}

	// If you need to debug and see all failed operations output
	oa.ignoreErrors = true

	if err := oa.Run(); err != nil {
		t.Errorf("Error while running the oplog applier: %s", err)
	}
}

func TestIgnoreSystemCollections(t *testing.T) {
	docs := []bson.M{
		{
			"ts": bson.MongoTimestamp(6699345900185059329),
			"h":  int64(1603488249581412698),
			"op": "i",
			"ns": "config.system.sessions",
			"t":  int64(1),
			"v":  int(2),
			"ui": bson.Binary{
				Kind: 0x4,
				Data: []byte{0x1, 0x69, 0xc9, 0x79, 0xb4, 0x6e, 0x42, 0xcf, 0x87, 0x5b, 0x6b, 0xab, 0xfd, 0x89, 0xa4, 0x51},
			},
			"wall": time.Now().Add(-1 * time.Minute),
			"o": bson.M{
				"_id": bson.M{
					"id": bson.Binary{
						Kind: 0x4,
						Data: []byte{0xb7, 0xa1, 0x43, 0xd8, 0xf4, 0x54, 0x43, 0xb3, 0xad, 0xc5, 0x9c, 0xbd, 0x21, 0x41, 0xba, 0x44},
					},
					"uid": []byte{0xe3, 0xb0, 0xc4, 0x42, 0x98, 0xfc, 0x1c, 0x14, 0x9a, 0xfb, 0xf4, 0xc8, 0x99, 0x6f, 0xb9, 0x24,
						0x27, 0xae, 0x41, 0xe4, 0x64, 0x9b, 0x93, 0x4c, 0xa4, 0x95, 0x99, 0x1b, 0x78, 0x52, 0xb8, 0x55},
				},
				"lastUse": time.Now().Add(-1 * time.Minute),
			},
		},
		{
			"t":  1,
			"ns": "config.cache.collections",
			"o2": bson.M{
				"_id": "ycsb_test2.usertable",
			},
			"wall": time.Now(),
			"ts":   bson.MongoTimestamp(time.Now().Unix()),
			"h":    int64(-2273244757254710743),
			"v":    2,
			"op":   "u",
			"ui": bson.Binary{
				Kind: 0x4,
				Data: []byte{0x1, 0x69, 0xc9, 0x79, 0xb4, 0x6e, 0x42, 0xcf, 0x87, 0x5b, 0x6b, 0xab, 0xfd, 0x89, 0xa4, 0x52},
			},
			"o": bson.M{
				"$v":   1,
				"$set": bson.M{"refreshing": true},
			},
		},
	}

	bsonReader := newMockBSONReader(docs)

	session, err := mgo.DialWithInfo(testutils.PrimaryDialInfo(t, testutils.MongoDBShard1ReplsetName))
	if err != nil {
		t.Fatalf("Cannot connect to primary: %s", err)
	}

	oa, err := NewOplogApply(session, bsonReader)
	if err != nil {
		t.Errorf("Cannot instantiate the oplog applier: %s", err)
	}

	// If you need to debug and see all failed operations output
	oa.ignoreErrors = false

	if err := oa.Run(); err != nil {
		t.Errorf("Error while running the oplog applier: %s", err)
	}
}

func TestBasicApplyLog(t *testing.T) {
	tmpfile, err := ioutil.TempFile("", "example.bson.")
	if err != nil {
		log.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	if testing.Verbose() {
		fmt.Printf("Dumping the oplog into %q\n", tmpfile.Name())
	}

	session, err := mgo.DialWithInfo(testutils.PrimaryDialInfo(t, testutils.MongoDBShard1ReplsetName))
	if err != nil {
		t.Fatalf("Cannot connect to primary: %s", err)
	}

	dbname := "test"
	testColsPrefix := "testcol_"
	maxDocs := 50

	db := session.DB(dbname)

	if err := cleanup(db, testColsPrefix); err != nil {
		t.Fatalf("Cannot clean up %s database before starting: %s", dbname, err)
	}
	if err := cleanup(db, "drop_col"); err != nil {
		t.Fatalf("Cannot clean up %s database before starting: %s", "drop_col", err)
	}

	ot, err := Open(session)
	if err != nil {
		t.Fatalf("Cannot instantiate the oplog tailer: %s", err)
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)

	// Start tailing the oplog in background while some records are being generated
	go func() {
		if _, err := io.Copy(tmpfile, ot); err != nil {
			t.Errorf("Error while tailing the oplog: %s", err)
		}
		wg.Done()
	}()

	if err := ot.WaitUntilFirstDoc(); err != nil {
		t.Fatalf("Error while waiting for first doc: %s", err)
	}

	testCols := []*mgo.CollectionInfo{
		{
			DisableIdIndex: false,
			ForceIdIndex:   true,
			Capped:         false,
		},
		{
			DisableIdIndex: false,
			ForceIdIndex:   true,
			Capped:         true,
			MaxDocs:        100,
			MaxBytes:       1E6,
		},
	}

	for i, info := range testCols {
		name := fmt.Sprintf("%s%02d", testColsPrefix, i)
		if err := db.C(name).Create(info); err != nil {
			t.Fatalf("Cannot create capped collection %s with info: %+v: %s", name, info, err)
		}
	}

	for i := 0; i < len(testCols); i++ {
		name := fmt.Sprintf("%s%02d", testColsPrefix, i)
		for j := 0; j < maxDocs; j++ {
			if err := db.C(name).Insert(bson.M{"f1": j, "f2": maxDocs - j}); err != nil {
				t.Errorf("Cannot insert document # %d into collection %s: %s", j, name, err)
			}
		}
	}

	if err := db.C("new_index").Insert(bson.M{"f1": 33, "f2": 34}); err != nil {
		t.Errorf("Cannot create the new_index collection and insert a doc: %s", err)
	}
	if err := db.C("new_index").DropCollection(); err != nil {
		t.Errorf("Cannot drop the new_index collection")
	}

	index := mgo.Index{
		Key:        []string{"f1", "-f2"},
		Unique:     true,
		DropDups:   true,
		Background: true, // See notes.
		Sparse:     true,
	}
	colName := fmt.Sprintf("%s%02d", testColsPrefix, 0)
	if err := db.C(colName).EnsureIndex(index); err != nil {
		t.Errorf("Cannot create index for testing")
	}

	if err := db.C(colName).Remove(bson.M{"f1": bson.M{"$gt": 10}}); err != nil {
		t.Errorf("Cannot remove some documents")
	}

	dropColName := "drop_col"

	info := &mgo.CollectionInfo{
		DisableIdIndex: false,
		ForceIdIndex:   true,
		Capped:         false,
	}
	if err := db.C(dropColName).Create(info); err != nil {
		t.Fatalf("Cannot create capped collection %s with info: %+v: %s", dropColName, info, err)
	}

	if err := db.C(dropColName).DropCollection(); err != nil {
		t.Errorf("Cannot drop collection %s: %s", dropColName, err)
	}

	ot.Close()

	wg.Wait()
	tmpfile.Close()

	if err := cleanup(db, testColsPrefix); err != nil {
		t.Fatalf("Cannot clean up %s database before applying the oplog: %s", dbname, err)
	}

	n, _, err := count(db, testColsPrefix)
	if err != nil {
		t.Errorf("Cannot verify collections have been dropped before applying the oplog: %s", err)
	}
	if n != 0 {
		t.Fatalf("Collections %s* already exists. Cannot test oplog apply", testColsPrefix)
	}

	// Open the oplog file we just wrote and use it as a reader for the oplog apply
	reader, err := bsonfile.OpenFile(tmpfile.Name())
	if err != nil {
		t.Errorf("Cannot open oplog dump file %q: %s", tmpfile.Name(), err)
		return
	}

	// Replay the oplog
	oa, err := NewOplogApply(session, reader)
	if err != nil {
		t.Errorf("Cannot instantiate the oplog applier: %s", err)
	}

	oa.ignoreErrors = true

	if err := oa.Run(); err != nil {
		t.Errorf("Error while running the oplog applier: %s", err)
	}

	n, d, err := count(db, testColsPrefix)
	if err != nil {
		t.Errorf("Cannot verify collections have been created by the oplog: %s", err)
	}

	maxCollections := len(testCols)

	if n != maxCollections {
		t.Fatalf("Invalid collections count after aplplying the oplog. Want %d, got %d", maxCollections, n)
	}
	wantDocs := maxCollections*maxDocs - 1 // 1 removed doc
	if d != wantDocs {
		t.Fatalf("Invalid documents count after aplplying the oplog. Want %d, got %d", wantDocs, d)
	}

	if colExists(db, dropColName) {
		t.Errorf("Collection %s was not dropped", dropColName)
	}

	if err := cleanup(db, testColsPrefix); err != nil {
		t.Fatalf("Cannot clean up %s database after testing: %s", dbname, err)
	}
	if err := cleanup(db, "new_index"); err != nil {
		t.Fatalf("Cannot clean up %s database after testing: %s", dbname, err)
	}
}

func TestSkipSystemCollections(t *testing.T) {
	tests := []struct {
		NS   string
		Skip bool
	}{
		{NS: "config.system.sessions", Skip: true},
		{NS: "config.cache.collections", Skip: true},
		{NS: "some-other-col", Skip: false},
	}

	for i, test := range tests {
		name := fmt.Sprintf("Test #%d: %s", i, test.NS)
		ns := test.NS
		want := test.Skip
		t.Run(name, func(t *testing.T) {
			if got := skip(ns); got != want {
				t.Errorf("Invalid result for collection skip %q. Want %v, got %v", ns, want, got)
			}
		})
	}
}

func colExists(db *mgo.Database, name string) bool {
	cols, err := db.CollectionNames()
	if err != nil {
		return false
	}
	for _, col := range cols {
		if col == name {
			return true
		}
	}
	return false
}

func cleanup(db *mgo.Database, testColsPrefix string) error {
	// Clean up before starting
	colNames, err := db.CollectionNames()
	if err != nil {
		return fmt.Errorf("Cannot list collections in %q database", err)
	}
	for _, cname := range colNames {
		if strings.HasPrefix(cname, testColsPrefix) {
			if err := db.C(cname).DropCollection(); err != nil {
				return fmt.Errorf("Cannot drop %s collection: %s", cname, err)
			}
		}
	}
	return nil
}

func count(db *mgo.Database, testColsPrefix string) (int, int, error) {
	var totalCols, totalDocs int
	colNames, err := db.CollectionNames()
	if err != nil {
		return 0, 0, fmt.Errorf("Cannot list collections in %q database", err)
	}
	for _, cname := range colNames {
		if strings.HasPrefix(cname, testColsPrefix) {
			n, err := db.C(cname).Count()
			if err != nil {
				return 0, 0, fmt.Errorf("Cannot count documents in collection %s: %s", cname, err)
			}
			totalCols++
			totalDocs += n
		}
	}
	return totalCols, totalDocs, nil
}
