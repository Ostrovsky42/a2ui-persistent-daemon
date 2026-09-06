package bubbletea

import (
	"reflect"
	"testing"
)

func TestTableLayoutInputContainsOnlyStructuralFields(t *testing.T) {
	intType := reflect.TypeOf(int(0))
	boolType := reflect.TypeOf(false)
	stringType := reflect.TypeOf("")
	columnsType := reflect.TypeOf([]tableColumn(nil))
	allowed := map[string]reflect.Type{
		"AvailableWidth":  intType,
		"AvailableHeight": intType,
		"Columns":         columnsType,
		"RowCount":        intType,
		"Selectable":      boolType,
		"SelectedRow":     intType,
		"RowOffset":       intType,
		"Variant":         stringType,
	}

	typeOfInput := reflect.TypeOf(TableLayoutInput{})
	if typeOfInput.NumField() != len(allowed) {
		t.Fatalf("TableLayoutInput structural contract changed: got %d fields, want %d", typeOfInput.NumField(), len(allowed))
	}
	for i := 0; i < typeOfInput.NumField(); i++ {
		field := typeOfInput.Field(i)
		wantType, ok := allowed[field.Name]
		if !ok {
			t.Fatalf("TableLayoutInput exposes non-contract field %q; row/cell values must never cross the planner boundary", field.Name)
		}
		if field.Type != wantType {
			t.Fatalf("TableLayoutInput field %q type changed: got %v, want %v", field.Name, field.Type, wantType)
		}
	}
}
