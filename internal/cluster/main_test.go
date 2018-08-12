package cluster

import (
	"io/ioutil"

	"github.com/globalsign/mgo/bson"
	"github.com/percona/mongodb-backup/internal/testutils"
)

var (
	testSecondary2Host = testutils.MongoDBHost + ":" + testutils.MongoDBSecondary2Port
)

func loadBSONFile(file string, out interface{}) error {
	bytes, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	return bson.Unmarshal(bytes, out)
}
