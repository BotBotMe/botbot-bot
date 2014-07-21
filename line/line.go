package line

import (
	"encoding/json"
	"errors"

	"github.com/golang/glog"
)

// Custom error
var (
	ErrLineShort     = errors.New("line too short")
	ErrLineMalformed = errors.New("malformed line")
)

// Line represent an IRC line
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
	BotNick   string
}

// NewFromJSON returns a pointer to line
func NewFromJSON(b []byte) (*Line, error) {
	var l Line
	err := json.Unmarshal(b, &l)
	if err != nil {
		glog.Errorln("An error occured while unmarshalling the line")
		return nil, err
	}
	glog.V(2).Infoln("line", l)
	return &l, nil

}

// String returns a JSON string
func (l *Line) String() string {
	return string(l.JSON())
}

// JSON returns a JSON []byte
func (l *Line) JSON() []byte {
	jsonData, err := json.Marshal(l)
	if err != nil {
		glog.Infoln("Error on json Marshal of "+l.Raw, err)
	}
	// client expects lines to have an ending
	jsonData = append(jsonData, '\n')
	return jsonData
}
