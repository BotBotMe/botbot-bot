package line

import (
	"encoding/json"
	"errors"

	"github.com/golang/glog"
)

var (
	ELSHORT     = errors.New("Line too short")
	ELMALFORMED = errors.New("Malformed line")
	ELIGNORE    = errors.New("Ignore this line")
)

type Line struct {
	ChatBotId int
	Raw       string
	Received  string
	User      string
	Host      string
	Command   string
	Args      []string
	Content   string
	IsCTCP    bool
	Channel   string
}

func (self *Line) String() string {
	return string(self.AsJson())
}

// Current line as json
func (self *Line) AsJson() []byte {
	jsonData, err := json.Marshal(self)
	if err != nil {
		glog.Infoln("Error on json Marshal of "+self.Raw, err)
	}
	// client expects lines to have an ending
	jsonData = append(jsonData, '\n')
	return jsonData
}
