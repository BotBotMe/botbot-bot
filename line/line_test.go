package line

import (
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func testLine() (l Line) {
	l = Line{
		ChatBotId: 1,
		Raw:       "Raw",
		Received:  "Received",
		User:      "bob",
		Host:      "example.com",
		Command:   "PRIVMSG",
		Args:      []string{"one", "two"},
		Content:   "The content",
		Channel:   "lincolnloop"}
	return
}

// Create a Line object, and call String method.
func TestCreateLine(t *testing.T) {
	l := testLine()
	if !strings.Contains(l.String(), "bob") {
		t.Error("Line did not render as string correctly")
	}
}

func TestLineString(t *testing.T) {
	srcLine := testLine()
	var trgtLine Line
	err := json.Unmarshal(srcLine.JSON(), &trgtLine)
	if err != nil {
		t.Error("Cannot Unmarshal a Line:", srcLine.String())
	}
	srcValue := reflect.ValueOf(&srcLine).Elem()
	trgtValue := reflect.ValueOf(&trgtLine).Elem()
	for i := 0; i < srcValue.NumField(); i++ {
		if srcValue.Field(i).Kind() != trgtValue.Field(i).Kind() {
			t.Error("Field", i, "does not have the same kind")
		}
		if fmt.Sprintf("%v", srcValue.Field(i)) != fmt.Sprintf("%v", trgtValue.Field(i)) {
			t.Error("Field", i, "does not have the same string representation")
		}
	}
}
