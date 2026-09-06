package storage

import "webtyp.com/model"

// Compiler converts agnostic Query values into engine-specific Plans. Each backend dialect
// (postgres, sqlite) implements this to render its own SQL.
type Compiler interface {
	Compile(q Query, m model.Model) (Plan, error)
}
