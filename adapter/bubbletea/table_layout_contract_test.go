package bubbletea

import (
	"reflect"
	"testing"
)

func TestTableLayoutInputDoesNotExposeRowValues(t *testing.T) {
	typeOfInput := reflect.TypeOf(TableLayoutInput{})
	if _, ok := typeOfInput.FieldByName("Rows"); ok {
		t.Fatal("TableLayoutInput must not expose row values: presentation mode is a pure structural decision over column contract and geometry")
	}
}
