package storage

import "webtyp.com/fmt"

// ErrNoRows is the agnostic sentinel for "query returned no rows". Conn implementations
// (postgres, sqlt) must map their driver-specific no-rows error to this value so callers can
// detect it without importing the std sql package. This is the raw contract sentinel; orm.QB.ReadOne
// translates it to the ergonomic orm.ErrNotFound — db itself never does that translation.
var ErrNoRows = fmt.Err("no", "rows")
