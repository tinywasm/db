package tests

import (
	"testing"

	"webtyp.com/model"
	"webtyp.com/storage"
	"webtyp.com/storage/conformance"
	"webtyp.com/storage/mem"
)

func TestMemConformance(t *testing.T) {
	conformance.Run(t, conformance.Factory{
		Name: "mem",
		New: func(t *testing.T, models ...model.Model) storage.Conn {
			return mem.New()
		},
	})
}
